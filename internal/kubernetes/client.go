package kubernetes

import (
	"os"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func NewClient() (*kubernetes.Clientset, error) {
	// First, try to load the in-cluster configuration.
	config, err := rest.InClusterConfig()

	if err != nil {
		kubeconfig := os.Getenv("KUBECONFIG")
		// If we are not running inside Kubernetes,
		// fall back to the local kubeconfig.
		if kubeconfig == "" {
			kubeconfig = clientcmd.RecommendedHomeFile
		}

		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, err
		}
	}
	// Create the Kubernetes client using the configuration.
	return kubernetes.NewForConfig(config)
}
