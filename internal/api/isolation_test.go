
package api

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/chinelo-obitube/kubeguard/internal/isolation"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestIsolationHTTPEnableDisable(t *testing.T) {
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

	handler := &IsolationHandler{
		Service: isolation.NewService(client),
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	body := `{
		"groupA": {
			"namespace": "frontend",
			"selector": {"run": "client"}
		},
		"groupB": {
			"namespace": "backend",
			"selector": {"app": "test-server"}
		}
	}`

	tests := []struct {
		name       string
		path       string
		handle     http.HandlerFunc
		wantStatus int
		wantBody   string
	}{
		{
			name:       "enable isolation",
			path:       "/isolation/enable",
			handle:     handler.Enable,
			wantStatus: http.StatusCreated,
			wantBody:   `"status":"enabled"`,
		},
		{
			name:       "disable isolation",
			path:       "/isolation/disable",
			handle:     handler.Disable,
			wantStatus: http.StatusOK,
			wantBody:   `"status":"disabled"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(
				http.MethodPost,
				tt.path,
				strings.NewReader(body),
			)
			rec := httptest.NewRecorder()

			tt.handle(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf(
					"expected HTTP %d, got %d: %s",
					tt.wantStatus,
					rec.Code,
					rec.Body.String(),
				)
			}

			if !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Fatalf(
					"expected response containing %q, got %s",
					tt.wantBody,
					rec.Body.String(),
				)
			}
		})
	}
}
