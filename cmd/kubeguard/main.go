package main

import (
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

	logger.Info("HTTP server starting", "address", ":8080")

	if err := http.ListenAndServe(":8080", api.RequestLogger(logger, mux)); err != nil {
	logger.Error("HTTP server failed", "error", err)
	os.Exit(1)
}
}
