package hardening

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Finding struct {
	Namespace     string
	Name          string
	Kind          string
	Container     string
	ContainerType string
	Issues        []string
}

func (s *Service) Inspect(namespace string) ([]Finding, error) {

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	deployments, err := s.Client.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	statefulSets, err := s.Client.AppsV1().StatefulSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	daemonSets, err := s.Client.AppsV1().DaemonSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	var findings []Finding
	for _, deployment := range deployments.Items {
		for _, container := range deployment.Spec.Template.Spec.Containers {

			findings = append(findings,
				inspectContainer(
					container,
					deployment.Spec.Template.Spec.SecurityContext,
					"Container",
					deployment.Namespace,
					deployment.Name,
					"Deployment",
				)...,
			)

		}
		for _, container := range deployment.Spec.Template.Spec.InitContainers {
			findings = append(findings,
				inspectContainer(
					container,
					deployment.Spec.Template.Spec.SecurityContext,
					"InitContainer",
					deployment.Namespace,
					deployment.Name,
					"Deployment",
				)...)
		}
	}

	for _, statefulSet := range statefulSets.Items {
		for _, container := range statefulSet.Spec.Template.Spec.Containers {
			findings = append(findings,
				inspectContainer(
					container,
					statefulSet.Spec.Template.Spec.SecurityContext,
					"Container",
					statefulSet.Namespace,
					statefulSet.Name,
					"StatefulSet",
				)...,
			)
		}
		for _, container := range statefulSet.Spec.Template.Spec.InitContainers {
			findings = append(findings,
				inspectContainer(
					container,
					statefulSet.Spec.Template.Spec.SecurityContext,
					"InitContainer",
					statefulSet.Namespace,
					statefulSet.Name,
					"StatefulSet",
				)...,
			)
		}
	}

	for _, daemonSet := range daemonSets.Items {
		for _, container := range daemonSet.Spec.Template.Spec.Containers {
			findings = append(findings,
				inspectContainer(
					container,
					daemonSet.Spec.Template.Spec.SecurityContext,
					"Container",
					daemonSet.Namespace,
					daemonSet.Name,
					"DaemonSet",
				)...,
			)
		}
		for _, container := range daemonSet.Spec.Template.Spec.InitContainers {
			findings = append(findings,
				inspectContainer(
					container,
					daemonSet.Spec.Template.Spec.SecurityContext,
					"InitContainer",
					daemonSet.Namespace,
					daemonSet.Name,
					"DaemonSet",
				)...,
			)
		}
	}
	return findings, nil
}

func inspectContainer(
	container corev1.Container,
	podSecurityContext *corev1.PodSecurityContext,
	containerType string,
	namespace, name, kind string,
) []Finding {

	var issues []string

	_, cpuRequestExists := container.Resources.Requests[corev1.ResourceCPU]
	if !cpuRequestExists {
		issues = append(issues, "missing CPU request")
	}

	_, cpuLimitExists := container.Resources.Limits[corev1.ResourceCPU]

	if !cpuLimitExists {
		issues = append(issues, "missing CPU limit")
	}

	_, memoryRequestExists := container.Resources.Requests[corev1.ResourceMemory]

	if !memoryRequestExists {
		issues = append(issues, "missing memory request")
	}
	_, memoryLimitExists := container.Resources.Limits[corev1.ResourceMemory]

	if !memoryLimitExists {
		issues = append(issues, "missing memory limit")
	}

	if container.SecurityContext == nil ||
		container.SecurityContext.AllowPrivilegeEscalation == nil ||
		*container.SecurityContext.AllowPrivilegeEscalation {
		issues = append(issues, "allowPrivilegeEscalation is not explicitly false")
	}
	capabilitiesHardened := false

	if container.SecurityContext != nil &&
		container.SecurityContext.Capabilities != nil {
		for _, capability := range container.SecurityContext.Capabilities.Drop {
			if capability == corev1.Capability("ALL") {
				capabilitiesHardened = true
				break
			}
		}
	}

	if !capabilitiesHardened {
		issues = append(issues, "missing capabilities.drop: ALL")
	}

	seccompHardened := false

	if podSecurityContext != nil &&
		podSecurityContext.SeccompProfile != nil {
		seccompHardened = podSecurityContext.SeccompProfile.Type ==
			corev1.SeccompProfileTypeRuntimeDefault
	}

	if container.SecurityContext != nil &&
		container.SecurityContext.SeccompProfile != nil {
		seccompHardened = container.SecurityContext.SeccompProfile.Type ==
			corev1.SeccompProfileTypeRuntimeDefault

	}

	if !seccompHardened {
		issues = append(issues, "seccomp profile is not RuntimeDefault")
	}

	runAsNonRoot := false

	if podSecurityContext != nil &&
		podSecurityContext.RunAsNonRoot != nil {
		runAsNonRoot = *podSecurityContext.RunAsNonRoot
	}

	if container.SecurityContext != nil &&
		container.SecurityContext.RunAsNonRoot != nil {
		runAsNonRoot = *container.SecurityContext.RunAsNonRoot
	}

	if !runAsNonRoot {
		issues = append(issues, "runAsNonRoot is not explicitly true")
	}

	if len(issues) == 0 {
		return nil
	}
	return []Finding{
		{
			Namespace:     namespace,
			Name:          name,
			Kind:          kind,
			Container:     container.Name,
			ContainerType: containerType,
			Issues:        issues,
		},
	}

}
