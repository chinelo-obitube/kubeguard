package isolation

import (
	"context"
	"testing"

	"fmt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	"strings"
)

func TestEnableDisable(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "client",
				Namespace: "frontend",
				Labels:    map[string]string{"run": "client"},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-server",
				Namespace: "backend",
				Labels:    map[string]string{"app": "test-server"},
			},
		},
	)

	service := NewService(client)

	request := Request{
		GroupA: Group{
			Namespace: "frontend",
			Selector:  map[string]string{"run": "client"},
		},
		GroupB: Group{
			Namespace: "backend",
			Selector:  map[string]string{"app": "test-server"},
		},
	}

	if err := service.Enable(request); err != nil {
		t.Fatalf("enable isolation: %v", err)
	}

	for _, namespace := range []string{"frontend", "backend"} {
		policies, err := client.NetworkingV1().
			NetworkPolicies(namespace).
			List(context.Background(), metav1.ListOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if len(policies.Items) != 1 {
			t.Fatalf("expected one policy in %s, got %d",
				namespace, len(policies.Items))
		}
	}

	if err := service.Disable(request); err != nil {
		t.Fatalf("disable isolation: %v", err)
	}

	for _, namespace := range []string{"frontend", "backend"} {
		policies, err := client.NetworkingV1().
			NetworkPolicies(namespace).
			List(context.Background(), metav1.ListOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if len(policies.Items) != 0 {
			t.Fatalf("expected no policies in %s after disable", namespace)
		}
	}
}

func TestDisableRejectsUnmanagedPolicy(t *testing.T) {
	request := Request{
		GroupA: Group{
			Namespace: "frontend",
			Selector:  map[string]string{"run": "client"},
		},
		GroupB: Group{
			Namespace: "backend",
			Selector:  map[string]string{"app": "test-server"},
		},
	}

	// Create two policies matching the request.
	frontendPolicy := buildPolicy(request.GroupA, request.GroupB)
	backendPolicy := buildPolicy(request.GroupB, request.GroupA)

	// Simulate a policy that Kubeguard does not own.
	delete(frontendPolicy.Labels, "app.kubernetes.io/managed-by")

	client := fake.NewSimpleClientset(
		frontendPolicy,
		backendPolicy,
	)

	service := NewService(client)

	// Disabling must fail because the frontend policy is unmanaged.
	if err := service.Disable(request); err == nil {
		t.Fatal("expected disable to reject an unmanaged policy")
	}

	// Neither policy should be deleted.
	for _, namespace := range []string{"frontend", "backend"} {
		_, err := client.NetworkingV1().
			NetworkPolicies(namespace).
			Get(context.Background(), policyName, metav1.GetOptions{})

		if err != nil {
			t.Fatalf("policy in %s was unexpectedly deleted: %v", namespace, err)
		}
	}
}

func TestEnableRollsBackWhenSecondPolicyFails(t *testing.T) {
	request := Request{
		GroupA: Group{
			Namespace: "frontend",
			Selector:  map[string]string{"run": "client"},
		},
		GroupB: Group{
			Namespace: "backend",
			Selector:  map[string]string{"app": "test-server"},
		},
	}

	client := fake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "client",
				Namespace: "frontend",
				Labels:    map[string]string{"run": "client"},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-server",
				Namespace: "backend",
				Labels:    map[string]string{"app": "test-server"},
			},
		},
	)

	// Simulate failure when Kubernetes creates the backend policy.
	client.PrependReactor(
		"create",
		"networkpolicies",
		func(action k8stesting.Action) (bool, runtime.Object, error) {
			if action.GetNamespace() == "backend" {
				return true, nil, fmt.Errorf("simulated backend creation failure")
			}
			return false, nil, nil
		},
	)

	service := NewService(client)

	if err := service.Enable(request); err == nil {
		t.Fatal("expected enable to fail")
	}

	// The frontend policy must be removed by rollback.
	for _, namespace := range []string{"frontend", "backend"} {
		policies, err := client.NetworkingV1().
			NetworkPolicies(namespace).
			List(context.Background(), metav1.ListOptions{})
		if err != nil {
			t.Fatal(err)
		}

		if len(policies.Items) != 0 {
			t.Fatalf(
				"expected no policies in %s after rollback, found %d",
				namespace,
				len(policies.Items),
			)
		}
	}
}

func TestEnableReportsRollbackFailure(t *testing.T) {
	request := Request{
		GroupA: Group{
			Namespace: "frontend",
			Selector:  map[string]string{"run": "client"},
		},
		GroupB: Group{
			Namespace: "backend",
			Selector:  map[string]string{"app": "test-server"},
		},
	}

	client := fake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "client",
				Namespace: "frontend",
				Labels:    map[string]string{"run": "client"},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-server",
				Namespace: "backend",
				Labels:    map[string]string{"app": "test-server"},
			},
		},
	)

	// Simulate Kubernetes rejecting creation of the second policy.
	client.PrependReactor(
		"create",
		"networkpolicies",
		func(action k8stesting.Action) (bool, runtime.Object, error) {
			if action.GetNamespace() == "backend" {
				return true, nil, fmt.Errorf("simulated backend creation failure")
			}
			return false, nil, nil
		},
	)

	// Simulate Kubernetes rejecting rollback deletion.
	client.PrependReactor(
		"delete",
		"networkpolicies",
		func(action k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, fmt.Errorf("simulated rollback deletion failure")
		},
	)

	service := NewService(client)

	err := service.Enable(request)
	if err == nil {
		t.Fatal("expected enable to fail")
	}

	if !strings.Contains(err.Error(), "rollback failed") {
		t.Fatalf("expected rollback failure in error, got: %v", err)
	}

	// The first policy should remain because deletion failed.
	_, err = client.NetworkingV1().
		NetworkPolicies("frontend").
		Get(context.Background(), policyName, metav1.GetOptions{})

	if err != nil {
		t.Fatalf("expected frontend policy to remain: %v", err)
	}
}
