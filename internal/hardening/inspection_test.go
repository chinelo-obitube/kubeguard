package hardening

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestInspectContainerAggregatesIssues(t *testing.T) {
	container := corev1.Container{
		Name: "app",
	}

	findings := inspectContainer(
		container,
		nil,
		"Container",
		"tenant-a",
		"gateway",
		"Deployment",
	)

	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}

	finding := findings[0]

	if finding.Container != "app" {
		t.Errorf("expected container app, got %s", finding.Container)
	}

	if finding.ContainerType != "Container" {
		t.Errorf("expected Container, got %s", finding.ContainerType)
	}

	if len(finding.Issues) != 8 {
		t.Errorf("expected 8 issues, got %d: %v",
			len(finding.Issues), finding.Issues)
	}
}

func TestInspectContainerFullyHardened(t *testing.T) {
	allowPrivilegeEscalation := false
	runAsNonRoot := true

	container := corev1.Container{
		Name: "app",

		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("128Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("500m"),
				corev1.ResourceMemory: resource.MustParse("256Mi"),
			},
		},

		SecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: &allowPrivilegeEscalation,
			RunAsNonRoot:             &runAsNonRoot,
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{"ALL"},
			},
			SeccompProfile: &corev1.SeccompProfile{
				Type: corev1.SeccompProfileTypeRuntimeDefault,
			},
		},
	}
	findings := inspectContainer(
		container,
		nil,
		"Container",
		"tenent-a",
		"gateway",
		"Deployment",
	)

	if len(findings) != 0 {
		t.Errorf("expected no findinfs, got %v", findings)
	}
}

func TestInspectContainerInheritsPodSecurityContext(t *testing.T) {
	runAsNonRoot := true
	podSecurityContext := &corev1.PodSecurityContext{
		RunAsNonRoot: &runAsNonRoot,
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		},
	}

	container := corev1.Container{
		Name: "app",
	}

	findings := inspectContainer(
		container,
		podSecurityContext,
		"Container",
		"tenant-a",
		"gateway",
		"Deployment",
	)

	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}

	if len(findings[0].Issues) != 6 {
		t.Errorf(
			"expected 6 issues, got %d: %v",
			len(findings[0].Issues),
			findings[0].Issues,
		)
	}
	for _, issue := range findings[0].Issues {
		if issue == "runAsNonRoot is not explicitly true" ||
			issue == "seccomp profile is not RuntimeDefault" {
			t.Errorf("unexpected inherited security issue: %s", issue)
		}
	}
}

func TestInspectContainerOverridesPodSecurityContext(t *testing.T) {
	podRunAsNonRoot := true
	containerRunAsNonRoot := false

	podSecurityContext := &corev1.PodSecurityContext{
		RunAsNonRoot: &podRunAsNonRoot,
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		},
	}

	container := corev1.Container{
		Name: "app",
		SecurityContext: &corev1.SecurityContext{
			RunAsNonRoot: &containerRunAsNonRoot,
			SeccompProfile: &corev1.SeccompProfile{
				Type: corev1.SeccompProfileTypeUnconfined,
			},
		},
	}

	findings := inspectContainer(
		container,
		podSecurityContext,
		"Container",
		"tenant-a",
		"gateway",
		"Deployment",
	)

	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}

	expectedIssues := map[string]bool{
		"runAsNonRoot is not explicitly true":   false,
		"seccomp profile is not RuntimeDefault": false,
	}

	for _, issue := range findings[0].Issues {
		if _, exists := expectedIssues[issue]; exists {
			expectedIssues[issue] = true
		}
	}

	for issue, found := range expectedIssues {
		if !found {
			t.Errorf("expected issue not reported: %s", issue)
		}
	}
}
