package hardening

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ApplyResult struct {
	Namespace string `json:"namespace"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Updated   bool   `json:"updated"`
}

func (s *Service) Apply(namespace string) ([]ApplyResult, error) {
	if err := validateNamespace(namespace); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	deployments, err := s.Client.AppsV1().
		Deployments(namespace).
		List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list deployments: %w", err)
	}

	results := make([]ApplyResult, 0, len(deployments.Items))

	for _, deployment := range deployments.Items {
		// Fetch the latest version before modifying it.
		current, err := s.Client.AppsV1().
			Deployments(namespace).
			Get(ctx, deployment.Name, metav1.GetOptions{})
		if err != nil {
			return results, fmt.Errorf("get deployment %s: %w", deployment.Name, err)
		}

		// Work on a copy, preserving the original object.
		updated := current.DeepCopy()

		changed := applyPodTemplateDefaults(&updated.Spec.Template)

		if changed {
			_, err = s.Client.AppsV1().
				Deployments(namespace).
				Update(ctx, updated, metav1.UpdateOptions{})
			if err != nil {
				return results, fmt.Errorf("update deployment %s: %w", deployment.Name, err)
			}
		}

		results = append(results, ApplyResult{
			Namespace: namespace,
			Kind:      "Deployment",
			Name:      deployment.Name,
			Updated:   changed,
		})
	}
	// Apply defaults to StatefulSets.
	statefulSets, err := s.Client.AppsV1().
		StatefulSets(namespace).
		List(ctx, metav1.ListOptions{})
	if err != nil {
		return results, fmt.Errorf("list statefulsets: %w", err)
	}

	for _, statefulSet := range statefulSets.Items {
		current, err := s.Client.AppsV1().
			StatefulSets(namespace).
			Get(ctx, statefulSet.Name, metav1.GetOptions{})
		if err != nil {
			return results, fmt.Errorf("get statefulset %s: %w", statefulSet.Name, err)
		}

		updated := current.DeepCopy()
		changed := applyPodTemplateDefaults(&updated.Spec.Template)

		if changed {
			_, err = s.Client.AppsV1().
				StatefulSets(namespace).
				Update(ctx, updated, metav1.UpdateOptions{})
			if err != nil {
				return results, fmt.Errorf("update statefulset %s: %w", statefulSet.Name, err)
			}
		}

		results = append(results, ApplyResult{
			Namespace: namespace,
			Kind:      "StatefulSet",
			Name:      statefulSet.Name,
			Updated:   changed,
		})
	}

	// Apply defaults to DaemonSets.
	daemonSets, err := s.Client.AppsV1().
		DaemonSets(namespace).
		List(ctx, metav1.ListOptions{})
	if err != nil {
		return results, fmt.Errorf("list daemonsets: %w", err)
	}

	for _, daemonSet := range daemonSets.Items {
		current, err := s.Client.AppsV1().
			DaemonSets(namespace).
			Get(ctx, daemonSet.Name, metav1.GetOptions{})
		if err != nil {
			return results, fmt.Errorf("get daemonset %s: %w", daemonSet.Name, err)
		}

		updated := current.DeepCopy()
		changed := applyPodTemplateDefaults(&updated.Spec.Template)

		if changed {
			_, err = s.Client.AppsV1().
				DaemonSets(namespace).
				Update(ctx, updated, metav1.UpdateOptions{})
			if err != nil {
				return results, fmt.Errorf("update daemonset %s: %w", daemonSet.Name, err)
			}
		}

		results = append(results, ApplyResult{
			Namespace: namespace,
			Kind:      "DaemonSet",
			Name:      daemonSet.Name,
			Updated:   changed,
		})
	}
	return results, nil
}
