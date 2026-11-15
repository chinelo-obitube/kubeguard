package isolation

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBuildPolicy(t *testing.T) {
	a := Group{
		Namespace: "frontend",
		Selector:  map[string]string{"run": "client"},
	}
	b := Group{
		Namespace: "backend",
		Selector:  map[string]string{"app": "test-server"},
	}

	policy := buildPolicy(a, b)

	if policy.Namespace != "frontend" {
		t.Fatalf("unexpected namespace: %s", policy.Namespace)
	}

	if policy.Name != "kubeguard-isolation" {
		t.Fatalf("unexpected policy name: %s", policy.Name)
	}

	if policy.Spec.PodSelector.MatchLabels["run"] != "client" {
		t.Fatal("policy does not select the frontend workload")
	}

	if len(policy.Spec.Ingress) != 1 || len(policy.Spec.Egress) != 1 {
		t.Fatal("policy must define both ingress and egress rules")
	}

	if len(policy.Spec.PolicyTypes) != 2 {
		t.Fatal("policy must enforce both ingress and egress")
	}

	// Verify that the opposite namespace is excluded.
	foundExclusion := false

	for _, peer := range policy.Spec.Egress[0].To {
		if peer.NamespaceSelector == nil {
			continue
		}

		for _, expression := range peer.NamespaceSelector.MatchExpressions {
			if expression.Key == "kubernetes.io/metadata.name" &&
				expression.Operator == metav1.LabelSelectorOpNotIn &&
				len(expression.Values) == 1 &&
				expression.Values[0] == "backend" {
				foundExclusion = true
			}
		}
	}

	if !foundExclusion {
		t.Fatal("policy does not exclude the opposite namespace")
	}
}