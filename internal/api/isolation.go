package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/chinelo-obitube/kubeguard/internal/isolation"
)

type IsolationHandler struct {
	Service *isolation.Service
	Logger  *slog.Logger
}

func (h *IsolationHandler) Enable(w http.ResponseWriter, r *http.Request) {
	var request isolation.Request

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		h.Logger.Warn("invalid isolation enable request", "error", err)
		http.Error(w, "invalid JSON request", http.StatusBadRequest)
		return
	}

	if err := request.Validate(); err != nil {
		h.Logger.Warn("isolation enable validation failed", "error", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	attrs := []any{
		"operation", "isolation.enable",
		"namespace_a", request.GroupA.Namespace,
		"selector_a", request.GroupA.Selector,
		"namespace_b", request.GroupB.Namespace,
		"selector_b", request.GroupB.Selector,
	}
	start := time.Now()

	h.Logger.Info(
		"isolation enable started",
		attrs...,
	)

	if err := h.Service.Enable(request); err != nil {
		h.Logger.Error(
			"isolation enable failed",
			append(
				append([]any{}, attrs...),
				"duration_ms", time.Since(start).Milliseconds(),
				"error", err,
			)...,
		)

		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	h.Logger.Info(
		"network isolation enabled",
		append(
			append([]any{}, attrs...),
			"duration_ms", time.Since(start).Milliseconds(),
		)...,
	)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "enabled",
	})
}

func (h *IsolationHandler) Disable(w http.ResponseWriter, r *http.Request) {
	var request isolation.Request

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		h.Logger.Warn("invalid isolation disable request", "error", err)
		http.Error(w, "invalid JSON request", http.StatusBadRequest)
		return
	}

	if err := request.Validate(); err != nil {
		h.Logger.Warn("isolation disable validation failed", "error", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	attrs := []any{
		"operation", "isolation.disable",
		"namespace_a", request.GroupA.Namespace,
		"selector_a", request.GroupA.Selector,
		"namespace_b", request.GroupB.Namespace,
		"selector_b", request.GroupB.Selector,
	}
	start := time.Now()

	h.Logger.Info(
		"isolation disable started",
		attrs...,
	)

	if err := h.Service.Disable(request); err != nil {
		h.Logger.Error(
			"isolation disable failed",
			append(
				append([]any{}, attrs...),
				"duration_ms", time.Since(start).Milliseconds(),
				"error", err,
			)...,
		)

		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	h.Logger.Info(
		"network isolation disabled",
		append(
			append([]any{}, attrs...),
			"duration_ms", time.Since(start).Milliseconds(),
		)...,
	)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "disabled",
	})
}
