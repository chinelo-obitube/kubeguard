package hardening

import "k8s.io/client-go/kubernetes"

type Service struct {
	Client kubernetes.Interface
}

func NewService(client kubernetes.Interface) *Service {
	return &Service{
		Client: client,
	}
}
