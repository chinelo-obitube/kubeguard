package isolation

import (
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	policyName         = "kubeguard-isolation"
	managedBy          = "kubeguard"
	namespaceNameLabel = "kubernetes.io/metadata.name"
	peerAnnotation     = "kubeguard.io/peer-namespace"
)

func buildPolicy(group, opposite Group) *networkingv1.NetworkPolicy {
	// Allow all Pods in the selected group's own namespace.
	peers := []networkingv1.NetworkPolicyPeer{
		{
			PodSelector: &metav1.LabelSelector{},
		},
	}

	// Allow Pods in every namespace except the opposing namespace.
	peers = append(peers, networkingv1.NetworkPolicyPeer{
		NamespaceSelector: &metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{
					Key:      namespaceNameLabel,
					Operator: metav1.LabelSelectorOpNotIn,
					Values:   []string{opposite.Namespace},
				},
			},
		},
	})

	// Allow Pods in the opposing namespace that do NOT match
	// the opposing workload's complete label selector.
	//
	// For app=gateway, this means app NotIn [gateway].
	//
	// For multiple labels, a Pod is outside the group if
	// at least one label differs, so each condition is a
	// separate peer (OR semantics).
	for key, value := range opposite.Selector {
		peers = append(peers, networkingv1.NetworkPolicyPeer{
			NamespaceSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					namespaceNameLabel: opposite.Namespace,
				},
			},
			PodSelector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      key,
						Operator: metav1.LabelSelectorOpNotIn,
						Values:   []string{value},
					},
				},
			},
		})
	}

	return &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      policyName,
			Namespace: group.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": managedBy,
			},
			Annotations: map[string]string{
				peerAnnotation: opposite.Namespace,
			},
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: group.Selector,
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{From: peers},
			},
			Egress: []networkingv1.NetworkPolicyEgressRule{
				{To: peers},
			},
		},
	}
}
