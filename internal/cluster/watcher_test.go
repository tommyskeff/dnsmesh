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
