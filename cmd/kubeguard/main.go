package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"

	"github.com/chinelo-obitube/kubeguard/internal/api"
	"github.com/chinelo-obitube/kubeguard/internal/hardening"
	"github.com/chinelo-obitube/kubeguard/internal/isolation"
	kubeclient "github.com/chinelo-obitube/kubeguard/internal/kubernetes"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	logger.Info("Kubeguard starting")

	client, err := kubeclient.NewClient()
	if err != nil {
		logger.Error("failed to create kubernetes client", "error", err)
		os.Exit(1)
	}
	logger.Info("Kubernetes client initialized successfully")

	isolationService := isolation.NewService(client)
	isolationHandler := &api.IsolationHandler{
		Service: isolationService,
		Logger:  logger,
	}

	hardeningService := hardening.NewService(client)
	hardeningHandler := &api.HardeningHandler{
		Service: hardeningService,
		Logger:  logger,
	}

	mux := http.NewServeMux()

	mux.HandleFunc("POST /hardening/preview", hardeningHandler.Preview)
	mux.HandleFunc("POST /hardening/apply", hardeningHandler.Apply)
	mux.HandleFunc("POST /isolation/enable", isolationHandler.Enable)
	mux.HandleFunc("POST /isolation/disable", isolationHandler.Disable)

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ready"))
	})

	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		if err := json.NewEncoder(w).Encode(map[string]any{
			"name":   "kubeguard",
			"status": "running",
			"endpoints": []string{
				"POST /hardening/preview",
				"POST /hardening/apply",
				"POST /isolation/enable",
				"POST /isolation/disable",
				"GET /healthz",
				"GET /readyz",
			},
		}); err != nil {
			logger.Error("failed to write root response", "error", err)
		}
	})

	logger.Info("HTTP server starting", "address", ":8080")

	if err := http.ListenAndServe(":8080", api.RequestLogger(logger, mux)); err != nil {
		logger.Error("HTTP server failed", "error", err)
		os.Exit(1)
	}
}
