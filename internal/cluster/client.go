package cluster

import (
	"fmt"

	"github.com/tommyskeff/dnsmesh/internal/config"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func NewClientFromConfig(cfg config.ClusterConfig) (*kubernetes.Clientset, error) {
	restConfig := &rest.Config{
		Host:        cfg.APIServer,
		BearerToken: cfg.Token,
		TLSClientConfig: rest.TLSClientConfig{
			CAData: []byte(cfg.CACert),
		},
	}

	client, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create client for cluster %s: %w", cfg.Name, err)
	}

	return client, nil
}

type ClientFactory struct {
	clients map[string]*kubernetes.Clientset
}

func NewClientFactory(clusters []config.ClusterConfig) (*ClientFactory, error) {
	clients := make(map[string]*kubernetes.Clientset, len(clusters))

	for _, cluster := range clusters {
		client, err := NewClientFromConfig(cluster)
		if err != nil {
			return nil, err
		}
		clients[cluster.Name] = client
	}

	return &ClientFactory{clients: clients}, nil
}

func (f *ClientFactory) GetClient(name string) (*kubernetes.Clientset, bool) {
	client, ok := f.clients[name]
	return client, ok
}

func (f *ClientFactory) GetAllClients() map[string]*kubernetes.Clientset {
	return f.clients
}
