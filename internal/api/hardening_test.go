package api

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/chinelo-obitube/kubeguard/internal/hardening"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestHardeningMultipleNamespaces(t *testing.T) {
	client := fake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-a",
				Namespace: "tenant-a",
			},
			Spec: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: "app", Image: "nginx:alpine"},
						},
					},
				},
			},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-b",
				Namespace: "tenant-b",
			},
			Spec: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: "app", Image: "nginx:alpine"},
						},
					},
				},
			},
		},
	)

	handler := &HardeningHandler{
		Service: hardening.NewService(client),
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	body := `{"namespaces":["tenant-a","tenant-b"]}`

	// Preview must succeed and must not modify workloads.
	previewReq := httptest.NewRequest(
		http.MethodPost,
		"/hardening/preview",
		strings.NewReader(body),
	)
	previewRec := httptest.NewRecorder()
	handler.Preview(previewRec, previewReq)

	if previewRec.Code != http.StatusOK {
		t.Fatalf("preview returned %d: %s",
			previewRec.Code, previewRec.Body.String())
	}

	for _, ns := range []string{"tenant-a", "tenant-b"} {
		deployment, err := client.AppsV1().
			Deployments(ns).
			Get(previewReq.Context(), "app-"+string(ns[len(ns)-1]), metav1.GetOptions{})
		if err != nil {
			t.Fatal(err)
		}

		if len(deployment.Spec.Template.Spec.Containers[0].Resources.Requests) != 0 {
			t.Fatalf("preview modified deployment in %s", ns)
		}
	}

	// Apply must update both namespaces.
	applyReq := httptest.NewRequest(
		http.MethodPost,
		"/hardening/apply",
		strings.NewReader(body),
	)
	applyRec := httptest.NewRecorder()
	handler.Apply(applyRec, applyReq)

	if applyRec.Code != http.StatusOK {
		t.Fatalf("apply returned %d: %s",
			applyRec.Code, applyRec.Body.String())
	}

	for _, tc := range []struct {
		namespace string
		name      string
	}{
		{"tenant-a", "app-a"},
		{"tenant-b", "app-b"},
	} {
		deployment, err := client.AppsV1().
			Deployments(tc.namespace).
			Get(applyReq.Context(), tc.name, metav1.GetOptions{})
		if err != nil {
			t.Fatal(err)
		}

		container := deployment.Spec.Template.Spec.Containers[0]

		if len(container.Resources.Requests) == 0 ||
			len(container.Resources.Limits) == 0 {
			t.Fatalf("resources not hardened for %s/%s",
				tc.namespace, tc.name)
		}

		if container.SecurityContext == nil {
			t.Fatalf("security context missing for %s/%s",
				tc.namespace, tc.name)
		}
	}

	// Reapplying should not modify either workload.
	repeatReq := httptest.NewRequest(
		http.MethodPost,
		"/hardening/apply",
		strings.NewReader(body),
	)
	repeatRec := httptest.NewRecorder()
	handler.Apply(repeatRec, repeatReq)

	if repeatRec.Code != http.StatusOK {
		t.Fatalf("repeat apply returned %d: %s",
			repeatRec.Code, repeatRec.Body.String())
	}

	if strings.Contains(repeatRec.Body.String(), `"updated":true`) {
		t.Fatalf("expected idempotent apply, got %s",
			repeatRec.Body.String())
	}
}

func TestHardeningRejectsProtectedNamespaceBeforeApply(t *testing.T) {
	client := fake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-a",
				Namespace: "tenant-a",
			},
			Spec: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: "app", Image: "nginx:alpine"},
						},
					},
				},
			},
		},
	)

	handler := &HardeningHandler{
		Service: hardening.NewService(client),
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	req := httptest.NewRequest(
		http.MethodPost,
		"/hardening/apply",
		strings.NewReader(
			`{"namespaces":["tenant-a","kube-system"]}`,
		),
	)
	rec := httptest.NewRecorder()
	handler.Apply(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s",
			rec.Code, rec.Body.String())
	}

	deployment, err := client.AppsV1().
		Deployments("tenant-a").
		Get(req.Context(), "app-a", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if len(deployment.Spec.Template.Spec.Containers[0].Resources.Requests) != 0 {
		t.Fatal("tenant-a was modified before protected namespace rejection")
	}
}
