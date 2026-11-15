package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/chinelo-obitube/kubeguard/internal/hardening"
)

type HardeningHandler struct {
	Service *hardening.Service
	Logger  *slog.Logger
}

type HardeningRequest struct {
	Namespace string `json:"namespace"`
}

func (h *HardeningHandler) Preview(w http.ResponseWriter, r *http.Request) {
	var request HardeningRequest

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "invalid JSON request", http.StatusBadRequest)
		return
	}

	result, err := h.Service.Preview(request.Namespace)
	if err != nil {
		h.Logger.Error("hardening preview failed", "error", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (h *HardeningHandler) Apply(w http.ResponseWriter, r *http.Request) {
	var request HardeningRequest

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		h.Logger.Warn("invalid hardening apply request", "error", err)
		http.Error(w, "invalid JSON request", http.StatusBadRequest)
		return
	}

	result, err := h.Service.Apply(request.Namespace)
	if err != nil {
		h.Logger.Error(
			"hardening apply failed",
			"namespace", request.Namespace,
			"error", err,
		)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	for _, workload := range result {
		h.Logger.Info(
			"hardening workload processed",
			"operation", "hardening.apply",
			"namespace", workload.Namespace,
			"kind", workload.Kind,
			"workload", workload.Name,
			"updated", workload.Updated,
		)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}
