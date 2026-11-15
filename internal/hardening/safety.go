package hardening

import "fmt"

var protectedNamespaces = map[string]bool{
	"kube-system":      true,
	"kube-public":      true,
	"kube-node-lease":  true,
	"kubeguard-system": true,
	"calico-system":    true,
	"tigera-operator":  true,
}

func validateNamespace(namespace string) error {
	if namespace == "" {
		return fmt.Errorf("namespace must not be empty")
	}

	if protectedNamespaces[namespace] {
		return fmt.Errorf("namespace %q is protected", namespace)
	}

	return nil
}
