package api

import (
	"encoding/json"
	"fmt"
	"github.com/chinelo-obitube/kubeguard/internal/hardening"
	"log/slog"
	"net/http"
	"time"
)

type HardeningHandler struct {
	Service *hardening.Service
	Logger  *slog.Logger
}

type HardeningRequest struct {
	Namespace  string   `json:"namespace,omitempty"`
	Namespaces []string `json:"namespaces,omitempty"`
}

func (r HardeningRequest) TargetNamespaces() ([]string, error) {
	if r.Namespace != "" && len(r.Namespaces) > 0 {
		return nil, fmt.Errorf("provide either namespace or namespaces, not both")
	}

	namespaces := r.Namespaces
	if r.Namespace != "" {
		namespaces = []string{r.Namespace}
	}

	if len(namespaces) == 0 {
		return nil, fmt.Errorf("at least one namespace is required")
	}

	seen := make(map[string]bool, len(namespaces))

	for _, namespace := range namespaces {
		if err := hardening.ValidateNamespace(namespace); err != nil {
			return nil, err
		}

		if seen[namespace] {
			return nil, fmt.Errorf("duplicate namespace %q", namespace)
		}

		seen[namespace] = true
	}

	return namespaces, nil
}

func (h *HardeningHandler) Preview(w http.ResponseWriter, r *http.Request) {
	var request HardeningRequest

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		h.Logger.Warn("invalid hardening preview request", "error", err)
		http.Error(w, "invalid JSON request", http.StatusBadRequest)
		return
	}

	namespaces, err := request.TargetNamespaces()
	if err != nil {
		h.Logger.Warn("invalid hardening preview targets", "error", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	start := time.Now()

	h.Logger.Info(
		"hardening preview started",
		"operation", "hardening.preview",
		"namespaces", namespaces,
	)

	results := make([]hardening.PreviewItem, 0)

	for _, namespace := range namespaces {
		items, err := h.Service.Preview(namespace)
		if err != nil {
			h.Logger.Error(
				"hardening preview failed",
				"operation", "hardening.preview",
				"namespace", namespace,
				"duration_ms", time.Since(start).Milliseconds(),
				"error", err,
			)

			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		results = append(results, items...)
	}

	h.Logger.Info(
		"hardening preview completed",
		"operation", "hardening.preview",
		"namespaces", namespaces,
		"preview_items", len(results),
		"duration_ms", time.Since(start).Milliseconds(),
	)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

func (h *HardeningHandler) Apply(w http.ResponseWriter, r *http.Request) {
	var request HardeningRequest

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		h.Logger.Warn("invalid hardening apply request", "error", err)
		http.Error(w, "invalid JSON request", http.StatusBadRequest)
		return
	}

	namespaces, err := request.TargetNamespaces()
	if err != nil {
		h.Logger.Warn("invalid hardening apply targets", "error", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	start := time.Now()

	h.Logger.Info(
		"hardening apply started",
		"operation", "hardening.apply",
		"namespaces", namespaces,
	)

	results := make([]hardening.ApplyResult, 0)
	updatedCount := 0

	for _, namespace := range namespaces {
		items, err := h.Service.Apply(namespace)
		results = append(results, items...)

		if err != nil {
			h.Logger.Error(
				"hardening apply failed; earlier updates may remain",
				"operation", "hardening.apply",
				"namespace", namespace,
				"namespaces", namespaces,
				"workloads_processed", len(results),
				"duration_ms", time.Since(start).Milliseconds(),
				"error", err,
			)

			http.Error(
				w,
				"hardening apply failed in namespace "+namespace+
					"; earlier updates may remain: "+err.Error(),
				http.StatusInternalServerError,
			)
			return
		}

		for _, workload := range items {
			if workload.Updated {
				updatedCount++
			}

			h.Logger.Info(
				"hardening workload processed",
				"operation", "hardening.apply",
				"namespace", workload.Namespace,
				"kind", workload.Kind,
				"workload", workload.Name,
				"updated", workload.Updated,
			)
		}
	}

	h.Logger.Info(
		"hardening apply completed",
		"operation", "hardening.apply",
		"namespaces", namespaces,
		"workloads_processed", len(results),
		"workloads_updated", updatedCount,
		"duration_ms", time.Since(start).Milliseconds(),
	)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}
