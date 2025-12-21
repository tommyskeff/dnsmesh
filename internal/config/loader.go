package config

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/dnsmesh/internal/logging"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	ConfigSecretKey = "config"
	PollInterval    = 60 * time.Second
)

type Loader struct {
	client     kubernetes.Interface
	namespace  string
	secretName string

	mu                  sync.RWMutex
	config              *GlobalConfig
	lastResourceVersion string

	onChange func(*GlobalConfig)
}

func NewLoader(secretName string, onChange func(*GlobalConfig)) (*Loader, error) {
	restConfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get in-cluster config: %w", err)
	}

	client, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	namespace, err := getInClusterNamespace()
	if err != nil {
		return nil, fmt.Errorf("failed to get namespace: %w", err)
	}

	return &Loader{
		client:     client,
		namespace:  namespace,
		secretName: secretName,
		onChange:   onChange,
	}, nil
}

func NewLoaderWithClient(client kubernetes.Interface, namespace, secretName string, onChange func(*GlobalConfig)) *Loader {
	return &Loader{
		client:     client,
		namespace:  namespace,
		secretName: secretName,
		onChange:   onChange,
	}
}

func getInClusterNamespace() (string, error) {
	data, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err != nil {
		return "", fmt.Errorf("failed to read namespace: %w", err)
	}
	return string(data), nil
}

func (l *Loader) Load(ctx context.Context) error {
	secret, err := l.client.CoreV1().Secrets(l.namespace).Get(ctx, l.secretName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get secret %s/%s: %w", l.namespace, l.secretName, err)
	}

	configData, ok := secret.Data[ConfigSecretKey]
	if !ok {
		return fmt.Errorf("secret %s/%s does not contain key '%s'", l.namespace, l.secretName, ConfigSecretKey)
	}

	config, err := ParseConfig(configData)
	if err != nil {
		return fmt.Errorf("failed to parse config from secret: %w", err)
	}

	l.mu.Lock()
	changed := l.lastResourceVersion != string(secret.ResourceVersion)
	l.config = config
	l.lastResourceVersion = string(secret.ResourceVersion)
	l.mu.Unlock()

	if changed && l.onChange != nil {
		l.onChange(config)
	}

	return nil
}

func (l *Loader) GetConfig() *GlobalConfig {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.config
}

func (l *Loader) Start(ctx context.Context) error {
	if err := l.Load(ctx); err != nil {
		return err
	}
	go l.poll(ctx)
	return nil
}

func (l *Loader) poll(ctx context.Context) {
	ticker := time.NewTicker(PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := l.Load(ctx); err != nil {
				logging.Error("Error loading config: %v", err)
			}
		}
	}
}
