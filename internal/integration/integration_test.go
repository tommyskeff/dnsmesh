//go:build integration
// +build integration

package integration

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/dnsmesh/internal/cluster"
	"github.com/dnsmesh/internal/dns"
	mdns "github.com/miekg/dns"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

const testDomain = "clusterset.local"

func TestIntegration_EndToEnd(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	dnsTable := dns.NewTable()
	client := fake.NewSimpleClientset()

	watcher := cluster.NewWatcher("test-cluster", client, testDomain, dnsTable)
	if err := watcher.Start(ctx); err != nil {
		t.Fatalf("failed to start watcher: %v", err)
	}
	defer watcher.Stop()

	_, err := client.CoreV1().Pods("default").Create(ctx, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pricing-server-abc123",
			Namespace: "default",
			UID:       "pod-1",
			Annotations: map[string]string{
				cluster.AnnotationExpose:  "true",
				cluster.AnnotationService: "pricing-server",
				cluster.AnnotationRealm:   "prod-na",
			},
		},
		Status: corev1.PodStatus{
			PodIP: "10.0.0.1",
		},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create pod: %v", err)
	}

	_, err = client.CoreV1().Pods("default").Create(ctx, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pricing-server-def456",
			Namespace: "default",
			UID:       "pod-2",
			Annotations: map[string]string{
				cluster.AnnotationExpose:  "true",
				cluster.AnnotationService: "pricing-server",
				cluster.AnnotationRealm:   "prod-na",
			},
		},
		Status: corev1.PodStatus{
			PodIP: "10.0.0.2",
		},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create pod: %v", err)
	}

	_, err = client.CoreV1().Services("default").Create(ctx, &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "user-service",
			Namespace: "default",
			UID:       "svc-1",
			Annotations: map[string]string{
				cluster.AnnotationExpose:  "true",
				cluster.AnnotationService: "user-service",
				cluster.AnnotationRealm:   "prod-eu",
			},
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: "10.100.0.1",
		},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	dnsServer := dns.NewServer(dnsTable, 30)

	t.Run("pricing-server resolves", func(t *testing.T) {
		m := dnsServer.HandleTestQuery("pricing-server.prod-na.clusterset.local", mdns.TypeA)

		if m.Rcode != mdns.RcodeSuccess {
			t.Errorf("expected NOERROR, got %s", mdns.RcodeToString[m.Rcode])
		}

		if len(m.Answer) != 1 {
			t.Fatalf("expected 1 answer, got %d", len(m.Answer))
		}

		a := m.Answer[0].(*mdns.A)
		if !a.A.Equal(net.ParseIP("10.0.0.1")) && !a.A.Equal(net.ParseIP("10.0.0.2")) {
			t.Errorf("unexpected IP: %v", a.A)
		}
	})

	t.Run("user-service resolves", func(t *testing.T) {
		m := dnsServer.HandleTestQuery("user-service.prod-eu.clusterset.local", mdns.TypeA)

		if m.Rcode != mdns.RcodeSuccess {
			t.Errorf("expected NOERROR, got %s", mdns.RcodeToString[m.Rcode])
		}

		if len(m.Answer) != 1 {
			t.Fatalf("expected 1 answer, got %d", len(m.Answer))
		}

		a := m.Answer[0].(*mdns.A)
		if !a.A.Equal(net.ParseIP("10.100.0.1")) {
			t.Errorf("expected IP 10.100.0.1, got %v", a.A)
		}
	})

	t.Run("nonexistent returns NXDOMAIN", func(t *testing.T) {
		m := dnsServer.HandleTestQuery("nonexistent.prod.clusterset.local", mdns.TypeA)

		if m.Rcode != mdns.RcodeNameError {
			t.Errorf("expected NXDOMAIN, got %s", mdns.RcodeToString[m.Rcode])
		}
	})

	t.Run("random selection for multiple IPs", func(t *testing.T) {
		ips := dnsTable.Lookup("pricing-server.prod-na.clusterset.local")
		if len(ips) != 2 {
			t.Errorf("expected 2 IPs, got %d", len(ips))
		}

		m := dnsServer.HandleTestQuery("pricing-server.prod-na.clusterset.local", mdns.TypeA)
		if len(m.Answer) != 1 {
			t.Fatalf("expected 1 answer, got %d", len(m.Answer))
		}
		a := m.Answer[0].(*mdns.A)
		if !a.A.Equal(net.ParseIP("10.0.0.1")) && !a.A.Equal(net.ParseIP("10.0.0.2")) {
			t.Errorf("unexpected IP: %v", a.A)
		}
	})
}

func TestIntegration_DynamicUpdates(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	dnsTable := dns.NewTable()
	client := fake.NewSimpleClientset()

	watcher := cluster.NewWatcher("test-cluster", client, testDomain, dnsTable)
	if err := watcher.Start(ctx); err != nil {
		t.Fatalf("failed to start watcher: %v", err)
	}
	defer watcher.Stop()

	dnsServer := dns.NewServer(dnsTable, 30)

	m := dnsServer.HandleTestQuery("dynamic-service.staging.clusterset.local", mdns.TypeA)
	if m.Rcode != mdns.RcodeNameError {
		t.Error("expected NXDOMAIN initially")
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dynamic-pod",
			Namespace: "default",
			UID:       "dynamic-pod-1",
			Annotations: map[string]string{
				cluster.AnnotationExpose:  "true",
				cluster.AnnotationService: "dynamic-service",
				cluster.AnnotationRealm:   "staging",
			},
		},
		Status: corev1.PodStatus{
			PodIP: "10.0.1.1",
		},
	}

	_, err := client.CoreV1().Pods("default").Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create pod: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	m = dnsServer.HandleTestQuery("dynamic-service.staging.clusterset.local", mdns.TypeA)
	if m.Rcode != mdns.RcodeSuccess {
		t.Errorf("expected NOERROR after pod creation, got %s", mdns.RcodeToString[m.Rcode])
	}

	err = client.CoreV1().Pods("default").Delete(ctx, "dynamic-pod", metav1.DeleteOptions{})
	if err != nil {
		t.Fatalf("failed to delete pod: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	m = dnsServer.HandleTestQuery("dynamic-service.staging.clusterset.local", mdns.TypeA)
	if m.Rcode != mdns.RcodeNameError {
		t.Errorf("expected NXDOMAIN after pod deletion, got %s", mdns.RcodeToString[m.Rcode])
	}
}

func TestIntegration_MultipleRealms(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	dnsTable := dns.NewTable()
	client := fake.NewSimpleClientset()

	watcher := cluster.NewWatcher("test-cluster", client, testDomain, dnsTable)
	if err := watcher.Start(ctx); err != nil {
		t.Fatalf("failed to start watcher: %v", err)
	}
	defer watcher.Stop()

	_, err := client.CoreV1().Pods("namespace1").Create(ctx, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-in-prod-na",
			Namespace: "namespace1",
			UID:       "prod-na-pod-1",
			Annotations: map[string]string{
				cluster.AnnotationExpose:  "true",
				cluster.AnnotationService: "my-service",
				cluster.AnnotationRealm:   "prod-na",
			},
		},
		Status: corev1.PodStatus{
			PodIP: "10.1.0.1",
		},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create pod in prod-na: %v", err)
	}

	_, err = client.CoreV1().Pods("namespace2").Create(ctx, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-in-prod-eu",
			Namespace: "namespace2",
			UID:       "prod-eu-pod-1",
			Annotations: map[string]string{
				cluster.AnnotationExpose:  "true",
				cluster.AnnotationService: "my-service",
				cluster.AnnotationRealm:   "prod-eu",
			},
		},
		Status: corev1.PodStatus{
			PodIP: "10.2.0.1",
		},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("failed to create pod in prod-eu: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	dnsServer := dns.NewServer(dnsTable, 30)

	// Different realms should be separate
	ipsNA := dnsTable.Lookup("my-service.prod-na.clusterset.local")
	if len(ipsNA) != 1 {
		t.Errorf("expected 1 IP in prod-na, got %d", len(ipsNA))
	}

	ipsEU := dnsTable.Lookup("my-service.prod-eu.clusterset.local")
	if len(ipsEU) != 1 {
		t.Errorf("expected 1 IP in prod-eu, got %d", len(ipsEU))
	}

	m := dnsServer.HandleTestQuery("my-service.prod-na.clusterset.local", mdns.TypeA)
	if m.Rcode != mdns.RcodeSuccess {
		t.Errorf("expected NOERROR for prod-na, got %s", mdns.RcodeToString[m.Rcode])
	}

	m = dnsServer.HandleTestQuery("my-service.prod-eu.clusterset.local", mdns.TypeA)
	if m.Rcode != mdns.RcodeSuccess {
		t.Errorf("expected NOERROR for prod-eu, got %s", mdns.RcodeToString[m.Rcode])
	}
}
