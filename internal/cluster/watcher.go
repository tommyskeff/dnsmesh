package cluster

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"

	"github.com/dnsmesh/internal/logging"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	listerv1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

const (
	AnnotationExpose  = "dnsmesh.tommyjs.dev/expose"
	AnnotationService = "dnsmesh.tommyjs.dev/service"
	AnnotationRealm   = "dnsmesh.tommyjs.dev/realm"
)

type DNSEntry struct {
	Name   string // e.g. "nginx-server.prod-na.clusterset.local"
	IP     net.IP
	Source string // "pod" or "service"
}

type DNSTableUpdater interface {
	AddEntry(name string, ip net.IP, source string)
	RemoveEntry(name string, ip net.IP, source string)
}

type Watcher struct {
	clusterName string
	client      kubernetes.Interface
	domain      string
	updater     DNSTableUpdater

	informerFactory informers.SharedInformerFactory
	podLister       listerv1.PodLister
	serviceLister   listerv1.ServiceLister

	mu      sync.RWMutex
	entries map[string][]DNSEntry // key: resource UID
}

func NewWatcher(clusterName string, client kubernetes.Interface, domain string, updater DNSTableUpdater) *Watcher {
	return &Watcher{
		clusterName: clusterName,
		client:      client,
		domain:      domain,
		updater:     updater,
		entries:     make(map[string][]DNSEntry),
	}
}

func (w *Watcher) Start(ctx context.Context) error {
	w.informerFactory = informers.NewSharedInformerFactory(w.client, 0)

	podInformer := w.informerFactory.Core().V1().Pods()
	w.podLister = podInformer.Lister()

	_, err := podInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    w.onPodAdd,
		UpdateFunc: w.onPodUpdate,
		DeleteFunc: w.onPodDelete,
	})
	if err != nil {
		return fmt.Errorf("failed to add pod event handler: %w", err)
	}

	serviceInformer := w.informerFactory.Core().V1().Services()
	w.serviceLister = serviceInformer.Lister()

	_, err = serviceInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    w.onServiceAdd,
		UpdateFunc: w.onServiceUpdate,
		DeleteFunc: w.onServiceDelete,
	})
	if err != nil {
		return fmt.Errorf("failed to add service event handler: %w", err)
	}

	w.informerFactory.Start(ctx.Done())

	if !cache.WaitForCacheSync(ctx.Done(),
		podInformer.Informer().HasSynced,
		serviceInformer.Informer().HasSynced) {
		return fmt.Errorf("failed to sync informer caches for cluster %s", w.clusterName)
	}

	logging.Debug("[%s] Watcher started, watching pods and services", w.clusterName)
	return nil
}

func (w *Watcher) Stop() {
	if w.informerFactory != nil {
		w.informerFactory.Shutdown()
	}
}

func (w *Watcher) GetAllEntries() []DNSEntry {
	w.mu.RLock()
	defer w.mu.RUnlock()

	var all []DNSEntry
	for _, entries := range w.entries {
		all = append(all, entries...)
	}
	return all
}

func (w *Watcher) Sync() error {
	pods, err := w.podLister.List(labels.Everything())
	if err != nil {
		return fmt.Errorf("failed to list pods: %w", err)
	}
	for _, pod := range pods {
		w.processPod(pod)
	}

	services, err := w.serviceLister.List(labels.Everything())
	if err != nil {
		return fmt.Errorf("failed to list services: %w", err)
	}
	for _, svc := range services {
		w.processService(svc)
	}

	return nil
}

func (w *Watcher) onPodAdd(obj interface{}) {
	pod := obj.(*corev1.Pod)
	w.processPod(pod)
}

func (w *Watcher) onPodUpdate(oldObj, newObj interface{}) {
	oldPod := oldObj.(*corev1.Pod)
	newPod := newObj.(*corev1.Pod)

	if oldPod.Status.PodIP != newPod.Status.PodIP ||
		oldPod.Annotations[AnnotationExpose] != newPod.Annotations[AnnotationExpose] ||
		oldPod.Annotations[AnnotationService] != newPod.Annotations[AnnotationService] ||
		oldPod.Annotations[AnnotationRealm] != newPod.Annotations[AnnotationRealm] {
		w.removePodEntries(oldPod)
		w.processPod(newPod)
	}
}

func (w *Watcher) onPodDelete(obj interface{}) {
	pod := obj.(*corev1.Pod)
	w.removePodEntries(pod)
}

func (w *Watcher) onServiceAdd(obj interface{}) {
	svc := obj.(*corev1.Service)
	w.processService(svc)
}

func (w *Watcher) onServiceUpdate(oldObj, newObj interface{}) {
	oldSvc := oldObj.(*corev1.Service)
	newSvc := newObj.(*corev1.Service)

	if oldSvc.Spec.ClusterIP != newSvc.Spec.ClusterIP ||
		oldSvc.Annotations[AnnotationExpose] != newSvc.Annotations[AnnotationExpose] ||
		oldSvc.Annotations[AnnotationService] != newSvc.Annotations[AnnotationService] ||
		oldSvc.Annotations[AnnotationRealm] != newSvc.Annotations[AnnotationRealm] {
		w.removeServiceEntries(oldSvc)
		w.processService(newSvc)
	}
}

func (w *Watcher) onServiceDelete(obj interface{}) {
	svc := obj.(*corev1.Service)
	w.removeServiceEntries(svc)
}

func (w *Watcher) processPod(pod *corev1.Pod) {
	if pod.Annotations[AnnotationExpose] != "true" {
		return
	}

	serviceName, ok := pod.Annotations[AnnotationService]
	if !ok || serviceName == "" {
		return
	}

	realm, ok := pod.Annotations[AnnotationRealm]
	if !ok || realm == "" {
		return
	}

	podIP := pod.Status.PodIP
	if podIP == "" {
		return
	}

	ip := net.ParseIP(podIP)
	if ip == nil {
		return
	}

	dnsName := w.buildDNSName(serviceName, realm)
	entry := DNSEntry{
		Name:   dnsName,
		IP:     ip,
		Source: "pod",
	}

	uid := string(pod.UID)
	w.mu.Lock()
	w.entries[uid] = []DNSEntry{entry}
	w.mu.Unlock()

	if w.updater != nil {
		w.updater.AddEntry(dnsName, ip, "pod")
		logging.Debug("[%s] Added pod entry: %s -> %s", w.clusterName, dnsName, ip)
	}
}

func (w *Watcher) removePodEntries(pod *corev1.Pod) {
	uid := string(pod.UID)
	w.mu.Lock()
	entries, ok := w.entries[uid]
	if ok {
		delete(w.entries, uid)
	}
	w.mu.Unlock()

	if ok && w.updater != nil {
		for _, entry := range entries {
			w.updater.RemoveEntry(entry.Name, entry.IP, entry.Source)
			logging.Debug("[%s] Removed pod entry: %s -> %s", w.clusterName, entry.Name, entry.IP)
		}
	}
}

func (w *Watcher) processService(svc *corev1.Service) {
	if svc.Annotations[AnnotationExpose] != "true" {
		return
	}

	serviceName, ok := svc.Annotations[AnnotationService]
	if !ok || serviceName == "" {
		return
	}

	realm, ok := svc.Annotations[AnnotationRealm]
	if !ok || realm == "" {
		return
	}

	clusterIP := svc.Spec.ClusterIP
	if clusterIP == "" || clusterIP == "None" {
		return
	}

	ip := net.ParseIP(clusterIP)
	if ip == nil {
		return
	}

	dnsName := w.buildDNSName(serviceName, realm)
	entry := DNSEntry{
		Name:   dnsName,
		IP:     ip,
		Source: "service",
	}

	uid := string(svc.UID)
	w.mu.Lock()
	w.entries[uid] = []DNSEntry{entry}
	w.mu.Unlock()

	if w.updater != nil {
		w.updater.AddEntry(dnsName, ip, "service")
		logging.Debug("[%s] Added service entry: %s -> %s", w.clusterName, dnsName, ip)
	}
}

func (w *Watcher) removeServiceEntries(svc *corev1.Service) {
	uid := string(svc.UID)
	w.mu.Lock()
	entries, ok := w.entries[uid]
	if ok {
		delete(w.entries, uid)
	}
	w.mu.Unlock()

	if ok && w.updater != nil {
		for _, entry := range entries {
			w.updater.RemoveEntry(entry.Name, entry.IP, entry.Source)
			logging.Debug("[%s] Removed service entry: %s -> %s", w.clusterName, entry.Name, entry.IP)
		}
	}
}

func (w *Watcher) buildDNSName(serviceName, realm string) string {
	name := strings.ToLower(serviceName + "." + realm + "." + w.domain)
	if !strings.HasSuffix(name, ".") {
		name += "."
	}
	return name
}
