package hardening

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type PreviewItem struct {
	Finding
	Changes         map[string]any `json:"changes"`
	Recommendations []string       `json:"recommendations,omitempty"`
}

func (s *Service) Preview(namespace string) ([]PreviewItem, error) {
	if err := validateNamespace(namespace); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	previews := []PreviewItem{}

	// Each workload type exposes its containers through a Pod template.
	// We inspect those templates without modifying the Kubernetes objects.

	deployments, err := s.Client.AppsV1().
		Deployments(namespace).
		List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list deployments: %w", err)
	}

	for _, deployment := range deployments.Items {
		previews = append(previews, previewTemplate(
			namespace,
			deployment.Name,
			"Deployment",
			&deployment.Spec.Template,
		)...)
	}

	statefulSets, err := s.Client.AppsV1().
		StatefulSets(namespace).
		List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list statefulsets: %w", err)
	}

	for _, statefulSet := range statefulSets.Items {
		previews = append(previews, previewTemplate(
			namespace,
			statefulSet.Name,
			"StatefulSet",
			&statefulSet.Spec.Template,
		)...)
	}

	daemonSets, err := s.Client.AppsV1().
		DaemonSets(namespace).
		List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list daemonsets: %w", err)
	}

	for _, daemonSet := range daemonSets.Items {
		previews = append(previews, previewTemplate(
			namespace,
			daemonSet.Name,
			"DaemonSet",
			&daemonSet.Spec.Template,
		)...)
	}

	return previews, nil
}

func previewTemplate(
	namespace, name, kind string,
	template *corev1.PodTemplateSpec,
) []PreviewItem {
	items := []PreviewItem{}

	for _, container := range template.Spec.Containers {
		if item := previewContainer(
			container,
			template.Spec.SecurityContext,
			"container",
			namespace,
			name,
			kind,
		); item != nil {
			items = append(items, *item)
		}
	}

	for _, container := range template.Spec.InitContainers {
		if item := previewContainer(
			container,
			template.Spec.SecurityContext,
			"initContainer",
			namespace,
			name,
			kind,
		); item != nil {
			items = append(items, *item)
		}
	}

	return items
}

func previewContainer(
	container corev1.Container,
	podSecurityContext *corev1.PodSecurityContext,
	containerType, namespace, name, kind string,
) *PreviewItem {
	findings := inspectContainer(
		container,
		podSecurityContext,
		containerType,
		namespace,
		name,
		kind,
	)

	changes := planContainerChanges(container, podSecurityContext)

	// No issues and no planned changes: nothing to report.
	if len(findings) == 0 && len(changes) == 0 {
		return nil
	}

	item := &PreviewItem{
		Changes: changes,
	}

	if len(findings) > 0 {
		item.Finding = findings[0]
	} else {
		// A mutation can be planned even when effective Pod-level
		// configuration already satisfies the inspection baseline.
		item.Finding = Finding{
			Namespace:     namespace,
			Name:          name,
			Kind:          kind,
			Container:     container.Name,
			ContainerType: containerType,
			Issues:        []string{},
		}
	}

	// Issues not fixed by the automatic mutation remain recommendations.
	for _, issue := range item.Issues {
		switch issue {
		case "runAsNonRoot is not explicitly true":
			item.Recommendations = append(
				item.Recommendations,
				"Review whether the image supports running as non-root",
			)

		case "allowPrivilegeEscalation is not explicitly false":
			if _, planned := changes["securityContext.allowPrivilegeEscalation"]; !planned {
				item.Recommendations = append(
					item.Recommendations,
					"Existing allowPrivilegeEscalation value is preserved; review manually",
				)
			}

		case "seccomp profile is not RuntimeDefault":
			if _, planned := changes["securityContext.seccompProfile.type"]; !planned {
				item.Recommendations = append(
					item.Recommendations,
					"Existing seccomp profile is preserved; review manually",
				)
			}
		}
	}

	return item
}
