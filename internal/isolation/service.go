package isolation

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"time"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/kubernetes"
)

type Group struct {
	Namespace string            `json:"namespace"`
	Selector  map[string]string `json:"selector"`
}

type Request struct {
	GroupA Group `json:"groupA"`
	GroupB Group `json:"groupB"`
}

type Service struct {
	Client kubernetes.Interface
	mu     sync.Mutex
}

func NewService(client kubernetes.Interface) *Service {
	return &Service{Client: client}
}

func (r Request) Validate() error {
	a, b := r.GroupA, r.GroupB

	if a.Namespace == b.Namespace {
		return fmt.Errorf("groups must use different namespaces")
	}

	for _, group := range []Group{a, b} {
		if errs := validation.IsDNS1123Label(group.Namespace); len(errs) > 0 {
			return fmt.Errorf("invalid namespace %q: %v", group.Namespace, errs)
		}
		if len(group.Selector) == 0 {
			return fmt.Errorf("namespace %q requires a non-empty selector", group.Namespace)
		}
		if _, err := metav1.LabelSelectorAsSelector(
			&metav1.LabelSelector{MatchLabels: group.Selector},
		); err != nil {
			return fmt.Errorf("invalid selector: %w", err)
		}
	}

	return nil
}

func (s *Service) Enable(r Request) error {
	if err := r.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Constrained MVP: reject any existing NetworkPolicy.
	// Other policies could allow A <-> B despite our rules.
	existing, err := s.Client.NetworkingV1().
		NetworkPolicies("").
		List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list network policies: %w", err)
	}
	if len(existing.Items) != 0 {
		return fmt.Errorf(
			"isolation requires a cluster without existing NetworkPolicies; found %d",
			len(existing.Items),
		)
	}

	// Require both selected workloads to exist.
	for _, group := range []Group{r.GroupA, r.GroupB} {
		selector, _ := metav1.LabelSelectorAsSelector(
			&metav1.LabelSelector{MatchLabels: group.Selector},
		)

		pods, err := s.Client.CoreV1().
			Pods(group.Namespace).
			List(ctx, metav1.ListOptions{
				LabelSelector: selector.String(),
			})
		if err != nil {
			return fmt.Errorf("list pods in %s: %w", group.Namespace, err)
		}
		if len(pods.Items) == 0 {
			return fmt.Errorf(
				"no Pods match selector in namespace %q",
				group.Namespace,
			)
		}
	}

	first := buildPolicy(r.GroupA, r.GroupB)
	second := buildPolicy(r.GroupB, r.GroupA)

	created, err := s.Client.NetworkingV1().
		NetworkPolicies(first.Namespace).
		Create(ctx, first, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("create first isolation policy: %w", err)
	}

	_, err = s.Client.NetworkingV1().
		NetworkPolicies(second.Namespace).
		Create(ctx, second, metav1.CreateOptions{})
	if err != nil {
		// Best-effort rollback of the first policy.
		rollbackErr := s.Client.NetworkingV1().
			NetworkPolicies(first.Namespace).
			Delete(ctx, created.Name, metav1.DeleteOptions{
				Preconditions: &metav1.Preconditions{
					UID: &created.UID,
				},
			})
		if rollbackErr != nil {
			return fmt.Errorf(
				"create second policy: %w; rollback failed: %v",
				err, rollbackErr,
			)
		}
		return fmt.Errorf("create second isolation policy: %w", err)
	}

	return nil
}

func (s *Service) Disable(r Request) error {
	if err := r.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pairs := []struct {
		group    Group
		opposite Group
	}{
		{r.GroupA, r.GroupB},
		{r.GroupB, r.GroupA},
	}

	// Verify both policies before deleting either.
	policies := make([]*networkingv1.NetworkPolicy, 0, 2)

	for _, pair := range pairs {
		policy, err := s.Client.NetworkingV1().
			NetworkPolicies(pair.group.Namespace).
			Get(ctx, policyName, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf(
				"get isolation policy in %s: %w",
				pair.group.Namespace, err,
			)
		}

		if policy.Labels["app.kubernetes.io/managed-by"] != managedBy ||
			policy.Annotations[peerAnnotation] != pair.opposite.Namespace ||
			!reflect.DeepEqual(policy.Spec.PodSelector.MatchLabels, pair.group.Selector) {
			return fmt.Errorf(
				"policy in %s does not match this isolation request",
				pair.group.Namespace,
			)
		}

		policies = append(policies, policy)
	}

	for _, policy := range policies {
		err := s.Client.NetworkingV1().
			NetworkPolicies(policy.Namespace).
			Delete(ctx, policy.Name, metav1.DeleteOptions{
				Preconditions: &metav1.Preconditions{
					UID: &policy.UID,
				},
			})
		if err != nil {
			return fmt.Errorf(
				"delete policy in %s: %w",
				policy.Namespace, err,
			)
		}
	}

	return nil
}
