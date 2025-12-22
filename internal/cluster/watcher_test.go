package cluster

import (
	"context"
	"net"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

type mockUpdater struct {
	entries map[string][]net.IP
}

func newMockUpdater() *mockUpdater {
	return &mockUpdater{
		entries: make(map[string][]net.IP),
	}
}

func (m *mockUpdater) AddEntry(name string, ip net.IP, source string) {
	m.entries[name] = append(m.entries[name], ip)
}

func (m *mockUpdater) RemoveEntry(name string, ip net.IP, source string) {
	ips := m.entries[name]
	for i, existingIP := range ips {
		if existingIP.Equal(ip) {
			m.entries[name] = append(ips[:i], ips[i+1:]...)
			break
		}
	}
	if len(m.entries[name]) == 0 {
		delete(m.entries, name)
	}
}

func (m *mockUpdater) getIPs(name string) []net.IP {
	return m.entries[name]
}

func TestWatcher_ProcessPod(t *testing.T) {
	client := fake.NewSimpleClientset()
	updater := newMockUpdater()
	watcher := NewWatcher("test-cluster", client, "clusterset.local", updater)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := watcher.Start(ctx); err != nil {
		t.Fatalf("failed to start watcher: %v", err)
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pricing-server-abc123",
			Namespace: "default",
			UID:       "pod-uid-1",
			Annotations: map[string]string{
				AnnotationExpose:  "true",
				AnnotationService: "pricing-server",
				AnnotationRealm:   "prod-na",
			},
		},
		Status: corev1.PodStatus{
			PodIP: "10.0.0.1",
		},
	}

	_, err := client.CoreV1().Pods("default").Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create pod: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	ips := updater.getIPs("pricing-server.prod-na.clusterset.local.")
	if len(ips) != 1 {
		t.Errorf("expected 1 IP, got %d (entries: %v)", len(ips), updater.entries)
	}
}

func TestWatcher_ProcessPodWithDifferentRealm(t *testing.T) {
	client := fake.NewSimpleClientset()
	updater := newMockUpdater()
	watcher := NewWatcher("test-cluster", client, "clusterset.local", updater)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := watcher.Start(ctx); err != nil {
		t.Fatalf("failed to start watcher: %v", err)
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "api-gateway-xyz",
			Namespace: "default",
			UID:       "pod-uid-2",
			Annotations: map[string]string{
				AnnotationExpose:  "true",
				AnnotationService: "api-gateway",
				AnnotationRealm:   "staging",
			},
		},
		Status: corev1.PodStatus{
			PodIP: "10.0.0.2",
		},
	}

	_, err := client.CoreV1().Pods("default").Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create pod: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	ips := updater.getIPs("api-gateway.staging.clusterset.local.")
	if len(ips) != 1 {
		t.Errorf("expected 1 IP with staging realm, got %d", len(ips))
	}
}

func TestWatcher_IgnorePodWithoutExpose(t *testing.T) {
	client := fake.NewSimpleClientset()
	updater := newMockUpdater()
	watcher := NewWatcher("test-cluster", client, "clusterset.local", updater)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := watcher.Start(ctx); err != nil {
		t.Fatalf("failed to start watcher: %v", err)
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "no-expose-pod",
			Namespace: "default",
			UID:       "pod-uid-no-expose",
			Annotations: map[string]string{
				AnnotationService: "some-service",
				AnnotationRealm:   "prod",
			},
		},
		Status: corev1.PodStatus{
			PodIP: "10.0.0.99",
		},
	}

	_, err := client.CoreV1().Pods("default").Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create pod: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	if len(updater.entries) != 0 {
		t.Errorf("expected 0 entries for pod without expose=true, got %d", len(updater.entries))
	}
}

func TestWatcher_IgnorePodWithExposeFalse(t *testing.T) {
	client := fake.NewSimpleClientset()
	updater := newMockUpdater()
	watcher := NewWatcher("test-cluster", client, "clusterset.local", updater)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := watcher.Start(ctx); err != nil {
		t.Fatalf("failed to start watcher: %v", err)
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "expose-false-pod",
			Namespace: "default",
			UID:       "pod-uid-expose-false",
			Annotations: map[string]string{
				AnnotationExpose:  "false",
				AnnotationService: "some-service",
				AnnotationRealm:   "prod",
			},
		},
		Status: corev1.PodStatus{
			PodIP: "10.0.0.98",
		},
	}

	_, err := client.CoreV1().Pods("default").Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create pod: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	if len(updater.entries) != 0 {
		t.Errorf("expected 0 entries for pod with expose=false, got %d", len(updater.entries))
	}
}

func TestWatcher_IgnorePodWithoutService(t *testing.T) {
	client := fake.NewSimpleClientset()
	updater := newMockUpdater()
	watcher := NewWatcher("test-cluster", client, "clusterset.local", updater)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := watcher.Start(ctx); err != nil {
		t.Fatalf("failed to start watcher: %v", err)
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "no-service-pod",
			Namespace: "default",
			UID:       "pod-uid-no-service",
			Annotations: map[string]string{
				AnnotationExpose: "true",
				AnnotationRealm:  "prod",
			},
		},
		Status: corev1.PodStatus{
			PodIP: "10.0.0.97",
		},
	}

	_, err := client.CoreV1().Pods("default").Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create pod: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	if len(updater.entries) != 0 {
		t.Errorf("expected 0 entries for pod without service, got %d", len(updater.entries))
	}
}

func TestWatcher_IgnorePodWithoutRealm(t *testing.T) {
	client := fake.NewSimpleClientset()
	updater := newMockUpdater()
	watcher := NewWatcher("test-cluster", client, "clusterset.local", updater)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := watcher.Start(ctx); err != nil {
		t.Fatalf("failed to start watcher: %v", err)
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "missing-realm-pod",
			Namespace: "default",
			UID:       "pod-uid-missing-realm",
			Annotations: map[string]string{
				AnnotationExpose:  "true",
				AnnotationService: "some-service",
			},
		},
		Status: corev1.PodStatus{
			PodIP: "10.0.0.96",
		},
	}

	_, err := client.CoreV1().Pods("default").Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create pod: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	if len(updater.entries) != 0 {
		t.Errorf("expected 0 entries for pod without realm, got %d", len(updater.entries))
	}
}

func TestWatcher_IgnorePodWithoutIP(t *testing.T) {
	client := fake.NewSimpleClientset()
	updater := newMockUpdater()
	watcher := NewWatcher("test-cluster", client, "clusterset.local", updater)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := watcher.Start(ctx); err != nil {
		t.Fatalf("failed to start watcher: %v", err)
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pending-pod",
			Namespace: "default",
			UID:       "pod-uid-4",
			Annotations: map[string]string{
				AnnotationExpose:  "true",
				AnnotationService: "pending-service",
				AnnotationRealm:   "prod",
			},
		},
		Status: corev1.PodStatus{
			PodIP: "",
		},
	}

	_, err := client.CoreV1().Pods("default").Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create pod: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	if len(updater.entries) != 0 {
		t.Errorf("expected 0 entries for pod without IP, got %d", len(updater.entries))
	}
}

func TestWatcher_ProcessService(t *testing.T) {
	client := fake.NewSimpleClientset()
	updater := newMockUpdater()
	watcher := NewWatcher("test-cluster", client, "clusterset.local", updater)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := watcher.Start(ctx); err != nil {
		t.Fatalf("failed to start watcher: %v", err)
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "user-service",
			Namespace: "default",
			UID:       "svc-uid-1",
			Annotations: map[string]string{
				AnnotationExpose:  "true",
				AnnotationService: "user-service",
				AnnotationRealm:   "prod-eu",
			},
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: "10.100.0.1",
		},
	}

	_, err := client.CoreV1().Services("default").Create(ctx, svc, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	ips := updater.getIPs("user-service.prod-eu.clusterset.local.")
	if len(ips) != 1 {
		t.Errorf("expected 1 IP, got %d", len(ips))
	}
}

func TestWatcher_IgnoreHeadlessService(t *testing.T) {
	client := fake.NewSimpleClientset()
	updater := newMockUpdater()
	watcher := NewWatcher("test-cluster", client, "clusterset.local", updater)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := watcher.Start(ctx); err != nil {
		t.Fatalf("failed to start watcher: %v", err)
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "headless-service",
			Namespace: "default",
			UID:       "svc-uid-2",
			Annotations: map[string]string{
				AnnotationExpose:  "true",
				AnnotationService: "headless",
				AnnotationRealm:   "prod",
			},
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: "None",
		},
	}

	_, err := client.CoreV1().Services("default").Create(ctx, svc, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	if len(updater.entries) != 0 {
		t.Errorf("expected 0 entries for headless service, got %d", len(updater.entries))
	}
}

func TestWatcher_DeletePod(t *testing.T) {
	client := fake.NewSimpleClientset()
	updater := newMockUpdater()
	watcher := NewWatcher("test-cluster", client, "clusterset.local", updater)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := watcher.Start(ctx); err != nil {
		t.Fatalf("failed to start watcher: %v", err)
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "to-delete",
			Namespace: "default",
			UID:       "pod-uid-5",
			Annotations: map[string]string{
				AnnotationExpose:  "true",
				AnnotationService: "delete-test",
				AnnotationRealm:   "prod",
			},
		},
		Status: corev1.PodStatus{
			PodIP: "10.0.0.5",
		},
	}

	_, err := client.CoreV1().Pods("default").Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create pod: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	if len(updater.getIPs("delete-test.prod.clusterset.local.")) != 1 {
		t.Fatal("expected entry to exist before delete")
	}
	err = client.CoreV1().Pods("default").Delete(ctx, "to-delete", metav1.DeleteOptions{})
	if err != nil {
		t.Fatalf("failed to delete pod: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	if len(updater.getIPs("delete-test.prod.clusterset.local.")) != 0 {
		t.Error("expected entry to be removed after delete")
	}
}

func TestResolvePlaceholders(t *testing.T) {
	tests := []struct {
		name         string
		template     string
		placeholders map[string]string
		expected     string
	}{
		{
			name:     "no placeholders",
			template: "my-service",
			placeholders: map[string]string{
				"name": "my-pod",
			},
			expected: "my-service",
		},
		{
			name:     "single placeholder",
			template: "{name}",
			placeholders: map[string]string{
				"name": "my-pod",
			},
			expected: "my-pod",
		},
		{
			name:     "multiple placeholders",
			template: "{name}-{namespace}",
			placeholders: map[string]string{
				"name":      "my-pod",
				"namespace": "default",
			},
			expected: "my-pod-default",
		},
		{
			name:     "placeholder with surrounding text",
			template: "service-{ordinal}-{namespace}",
			placeholders: map[string]string{
				"ordinal":   "2",
				"namespace": "prod",
			},
			expected: "service-2-prod",
		},
		{
			name:     "empty placeholder value",
			template: "service-{ordinal}",
			placeholders: map[string]string{
				"ordinal": "",
			},
			expected: "service-",
		},
		{
			name:     "unknown placeholder left as-is",
			template: "{unknown}",
			placeholders: map[string]string{
				"name": "my-pod",
			},
			expected: "{unknown}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolvePlaceholders(tt.template, tt.placeholders)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestIpToDashForm(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"10.0.0.1", "10-0-0-1"},
		{"192.168.1.100", "192-168-1-100"},
		{"2001:db8::1", "2001-db8--1"},
		{"::1", "--1"},
	}

	for _, tt := range tests {
		result := ipToDashForm(tt.input)
		if result != tt.expected {
			t.Errorf("ipToDashForm(%q) = %q, expected %q", tt.input, result, tt.expected)
		}
	}
}

func TestWatcher_PlaceholderInServiceAnnotation(t *testing.T) {
	client := fake.NewSimpleClientset()
	updater := newMockUpdater()
	watcher := NewWatcher("test-cluster", client, "clusterset.local", updater)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := watcher.Start(ctx); err != nil {
		t.Fatalf("failed to start watcher: %v", err)
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-app-0",
			Namespace: "production",
			UID:       "pod-uid-placeholder-1",
			Annotations: map[string]string{
				AnnotationExpose:  "true",
				AnnotationService: "{name}",
				AnnotationRealm:   "prod",
			},
		},
		Spec: corev1.PodSpec{
			NodeName: "node-1",
		},
		Status: corev1.PodStatus{
			PodIP: "10.0.0.1",
		},
	}

	_, err := client.CoreV1().Pods("production").Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create pod: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	ips := updater.getIPs("my-app-0.prod.clusterset.local.")
	if len(ips) != 1 {
		t.Errorf("expected 1 IP for {name} placeholder, got %d (entries: %v)", len(ips), updater.entries)
	}
}

func TestWatcher_PlaceholderInRealmAnnotation(t *testing.T) {
	client := fake.NewSimpleClientset()
	updater := newMockUpdater()
	watcher := NewWatcher("test-cluster", client, "clusterset.local", updater)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := watcher.Start(ctx); err != nil {
		t.Fatalf("failed to start watcher: %v", err)
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-app-0",
			Namespace: "production",
			UID:       "pod-uid-placeholder-2",
			Annotations: map[string]string{
				AnnotationExpose:  "true",
				AnnotationService: "my-app",
				AnnotationRealm:   "{namespace}",
			},
		},
		Status: corev1.PodStatus{
			PodIP: "10.0.0.2",
		},
	}

	_, err := client.CoreV1().Pods("production").Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create pod: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	ips := updater.getIPs("my-app.production.clusterset.local.")
	if len(ips) != 1 {
		t.Errorf("expected 1 IP for {namespace} placeholder, got %d (entries: %v)", len(ips), updater.entries)
	}
}

func TestWatcher_StatefulSetOrdinalPlaceholder(t *testing.T) {
	client := fake.NewSimpleClientset()
	updater := newMockUpdater()
	watcher := NewWatcher("test-cluster", client, "clusterset.local", updater)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := watcher.Start(ctx); err != nil {
		t.Fatalf("failed to start watcher: %v", err)
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "redis-cluster-2",
			Namespace: "default",
			UID:       "pod-uid-sts-ordinal",
			Annotations: map[string]string{
				AnnotationExpose:  "true",
				AnnotationService: "redis-{ordinal}",
				AnnotationRealm:   "prod",
			},
		},
		Status: corev1.PodStatus{
			PodIP: "10.0.0.3",
		},
	}

	_, err := client.CoreV1().Pods("default").Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create pod: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	ips := updater.getIPs("redis-2.prod.clusterset.local.")
	if len(ips) != 1 {
		t.Errorf("expected 1 IP for {ordinal} placeholder, got %d (entries: %v)", len(ips), updater.entries)
	}
}

func TestWatcher_IpPlaceholder(t *testing.T) {
	client := fake.NewSimpleClientset()
	updater := newMockUpdater()
	watcher := NewWatcher("test-cluster", client, "clusterset.local", updater)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := watcher.Start(ctx); err != nil {
		t.Fatalf("failed to start watcher: %v", err)
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "web-server-0",
			Namespace: "default",
			UID:       "pod-uid-ip-placeholder",
			Annotations: map[string]string{
				AnnotationExpose:  "true",
				AnnotationService: "web-{ip}",
				AnnotationRealm:   "prod",
			},
		},
		Status: corev1.PodStatus{
			PodIP: "10.0.0.4",
		},
	}

	_, err := client.CoreV1().Pods("default").Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create pod: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	ips := updater.getIPs("web-10-0-0-4.prod.clusterset.local.")
	if len(ips) != 1 {
		t.Errorf("expected 1 IP for {ip} placeholder, got %d (entries: %v)", len(ips), updater.entries)
	}
}

func TestWatcher_KindPlaceholder(t *testing.T) {
	client := fake.NewSimpleClientset()
	updater := newMockUpdater()
	watcher := NewWatcher("test-cluster", client, "clusterset.local", updater)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := watcher.Start(ctx); err != nil {
		t.Fatalf("failed to start watcher: %v", err)
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-app",
			Namespace: "default",
			UID:       "pod-uid-kind",
			Annotations: map[string]string{
				AnnotationExpose:  "true",
				AnnotationService: "my-app",
				AnnotationRealm:   "{kind}",
			},
		},
		Status: corev1.PodStatus{
			PodIP: "10.0.0.5",
		},
	}

	_, err := client.CoreV1().Pods("default").Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create pod: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	ips := updater.getIPs("my-app.pod.clusterset.local.")
	if len(ips) != 1 {
		t.Errorf("expected 1 IP for {kind} placeholder on pod, got %d (entries: %v)", len(ips), updater.entries)
	}
}

func TestWatcher_ServicePlaceholders(t *testing.T) {
	client := fake.NewSimpleClientset()
	updater := newMockUpdater()
	watcher := NewWatcher("test-cluster", client, "clusterset.local", updater)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := watcher.Start(ctx); err != nil {
		t.Fatalf("failed to start watcher: %v", err)
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "api-gateway",
			Namespace: "prod-namespace",
			UID:       "svc-uid-placeholder",
			Annotations: map[string]string{
				AnnotationExpose:  "true",
				AnnotationService: "{name}",
				AnnotationRealm:   "{namespace}-{kind}",
			},
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: "10.100.0.5",
		},
	}

	_, err := client.CoreV1().Services("prod-namespace").Create(ctx, svc, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	ips := updater.getIPs("api-gateway.prod-namespace-service.clusterset.local.")
	if len(ips) != 1 {
		t.Errorf("expected 1 IP for service placeholders, got %d (entries: %v)", len(ips), updater.entries)
	}
}

func TestWatcher_MultiplePlaceholders(t *testing.T) {
	client := fake.NewSimpleClientset()
	updater := newMockUpdater()
	watcher := NewWatcher("test-cluster", client, "clusterset.local", updater)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := watcher.Start(ctx); err != nil {
		t.Fatalf("failed to start watcher: %v", err)
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "redis-cluster-1",
			Namespace: "cache",
			UID:       "pod-uid-multi",
			Annotations: map[string]string{
				AnnotationExpose:  "true",
				AnnotationService: "{namespace}-redis-{ordinal}",
				AnnotationRealm:   "{kind}-realm",
			},
		},
		Status: corev1.PodStatus{
			PodIP: "10.0.0.10",
		},
	}

	_, err := client.CoreV1().Pods("cache").Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create pod: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	ips := updater.getIPs("cache-redis-1.pod-realm.clusterset.local.")
	if len(ips) != 1 {
		t.Errorf("expected 1 IP for multiple placeholders, got %d (entries: %v)", len(ips), updater.entries)
	}
}
