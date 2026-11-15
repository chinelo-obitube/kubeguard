package hardening

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestInspectFindsDeployment(t *testing.T) {
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gateway",
			Namespace: "tenant-a",
		},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "app",
						},
					},
				},
			},
		},
	}

	client := fake.NewSimpleClientset(deployment)
	service := NewService(client)

	findings, err := service.Inspect("tenant-a")
	if err != nil {
		t.Fatalf("Inspect returned an errror: %v", err)
	}

	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}

	if findings[0].Kind != "Deployment" {
		t.Errorf("expected Deployment, got %s", findings[0].Kind)
	}

	if findings[0].Name != "gateway" {
		t.Errorf("expected gateway, got %s", findings[0].Name)
	}

	if findings[0].Container != "app" {
		t.Errorf("expected app, got %s", findings[0].Container)
	}
}

func TestInspectOnlyRequestedNamespace(t *testing.T) {
	deploymentA := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gateway",
			Namespace: "tenant-a",
		},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "app"},
					},
				},
			},
		},
	}

	deploymentB := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dashboard",
			Namespace: "tenant-b",
		},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "app"},
					},
				},
			},
		},
	}

	client := fake.NewSimpleClientset(deploymentA, deploymentB)
	service := NewService(client)

	findings, err := service.Inspect("tenant-a")
	if err != nil {
		t.Fatalf("Inspect returned an error: %v", err)
	}

	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d: %v", len(findings), findings)
	}

	if findings[0].Namespace != "tenant-a" {
		t.Errorf(
			"expected namespace tenant-a, got %s",
			findings[0].Namespace,
		)
	}

	if findings[0].Name != "gateway" {
		t.Errorf(
			"expected gateway, got %s",
			findings[0].Name,
		)
	}
}
