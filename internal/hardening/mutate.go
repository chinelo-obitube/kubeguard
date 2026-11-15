package hardening

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func applyResourceDefaults(container *corev1.Container) bool {
	changed := false

	if container.Resources.Requests == nil {
		container.Resources.Requests = corev1.ResourceList{}
	}

	if container.Resources.Limits == nil {
		container.Resources.Limits = corev1.ResourceList{}
	}

	if _, exists := container.Resources.Requests[corev1.ResourceCPU]; !exists {
		container.Resources.Requests[corev1.ResourceCPU] = resource.MustParse("100m")
		changed = true
	}

	if _, exists := container.Resources.Requests[corev1.ResourceMemory]; !exists {
		container.Resources.Requests[corev1.ResourceMemory] = resource.MustParse("128Mi")
		changed = true
	}

	if _, exists := container.Resources.Limits[corev1.ResourceCPU]; !exists {
		container.Resources.Limits[corev1.ResourceCPU] = resource.MustParse("500m")
		changed = true
	}

	if _, exists := container.Resources.Limits[corev1.ResourceMemory]; !exists {
		container.Resources.Limits[corev1.ResourceMemory] = resource.MustParse("256Mi")
		changed = true
	}

	return changed
}

func applySecurityDefaults(
	container *corev1.Container,
	podSecurityContext *corev1.PodSecurityContext,
) bool {
	changed := false

	if container.SecurityContext == nil {
		container.SecurityContext = &corev1.SecurityContext{}
	}

	security := container.SecurityContext

	if security.AllowPrivilegeEscalation == nil {
		allow := false
		security.AllowPrivilegeEscalation = &allow
		changed = true
	}

	if security.Capabilities == nil {
		security.Capabilities = &corev1.Capabilities{}
	}
	hasDropAll := false
	for _, capability := range security.Capabilities.Drop {
		if capability == corev1.Capability("ALL") {
			hasDropAll = true
			break
		}
	}

	if !hasDropAll {
		security.Capabilities.Drop = append(security.Capabilities.Drop, corev1.Capability("ALL"))
		changed = true
	}
	podHasSeccomp := podSecurityContext != nil &&
		podSecurityContext.SeccompProfile != nil

	containerHasSeccomp := security.SeccompProfile != nil

	if !podHasSeccomp && !containerHasSeccomp {
		security.SeccompProfile = &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		}
		changed = true
	}

	return changed
}

func applyPodTemplateDefaults(template *corev1.PodTemplateSpec) bool {
	changed := false

	for i := range template.Spec.Containers {
		if applyResourceDefaults(&template.Spec.Containers[i]) {
			changed = true
		}

		if applySecurityDefaults(
			&template.Spec.Containers[i],
			template.Spec.SecurityContext) {
			changed = true
		}
	}

	for i := range template.Spec.InitContainers {
		if applyResourceDefaults(&template.Spec.InitContainers[i]) {
			changed = true
		}
		if applySecurityDefaults(&template.Spec.InitContainers[i],
			template.Spec.SecurityContext) {
			changed = true
		}
	}

	return changed
}

func planContainerChanges(container corev1.Container, podSecurityContext *corev1.PodSecurityContext) map[string]any {
	before := container.DeepCopy()
	after := container.DeepCopy()

	applyResourceDefaults(after)
	applySecurityDefaults(after, podSecurityContext)

	changes := make(map[string]any)

	for _, key := range []corev1.ResourceName{
		corev1.ResourceCPU,
		corev1.ResourceMemory,
	} {
		if _, exists := before.Resources.Requests[key]; !exists {
			quantity := after.Resources.Requests[key]
			changes["requests."+string(key)] = quantity.String()
		}
		if _, exists := before.Resources.Limits[key]; !exists {
			quantity := after.Resources.Limits[key]
			changes["limits."+string(key)] = quantity.String()
		}
	}

	if before.SecurityContext == nil ||
		before.SecurityContext.AllowPrivilegeEscalation == nil {
		changes["securityContext.allowPrivilegeEscalation"] = false
	}

	hasDropAll := false
	if before.SecurityContext != nil &&
		before.SecurityContext.Capabilities != nil {
		for _, capability := range before.SecurityContext.Capabilities.Drop {
			if capability == corev1.Capability("ALL") {
				hasDropAll = true
			}
		}
	}

	if !hasDropAll {
		changes["securityContext.capabilities.drop"] =
			after.SecurityContext.Capabilities.Drop
	}

	podHasSeccomp := podSecurityContext != nil &&
		podSecurityContext.SeccompProfile != nil

	containerHasSeccomp := before.SecurityContext != nil &&
		before.SecurityContext.SeccompProfile != nil

	if !podHasSeccomp && !containerHasSeccomp {
		changes["securityContext.seccompProfile.type"] = "RuntimeDefault"
	}

	return changes
}
