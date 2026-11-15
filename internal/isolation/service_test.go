package isolation

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
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