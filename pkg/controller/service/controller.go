package service

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"

	"github.com/everoute/runtime/pkg/options"
)

// Controller update external service external-name to leading node IP
type Controller struct {
	// handle service event
	serviceInformer       cache.SharedIndexInformer
	serviceLister         cache.Indexer
	serviceInformerSynced cache.InformerSynced

	// handle leading event
	electionClient options.LeaderElectionClient

	shouldHandleService func(*corev1.Service) bool
	publicIP            net.IP
	clientset           kubernetes.Interface
	reconcileQueue      workqueue.RateLimitingInterface
}

const (
	matchExternalServiceIndex      = "matchExternalServiceIndex"
	matchExternalServiceIndexValue = "true"
)

// New creates a new instance of controller
func New(
	clientset kubernetes.Interface,
	kubeFactory informers.SharedInformerFactory,
	electionClient options.LeaderElectionClient,
	publicIP net.IP,
	resyncPeriod time.Duration,
	includeServices ...string,
) *Controller {
	serviceInformer := kubeFactory.Core().V1().Services().Informer()

	c := &Controller{
		serviceInformer:       serviceInformer,
		serviceLister:         serviceInformer.GetIndexer(),
		serviceInformerSynced: serviceInformer.HasSynced,
		electionClient:        electionClient,
		publicIP:              publicIP,
		clientset:             clientset,
		reconcileQueue:        workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter()),
	}

	_ = lo.Must(serviceInformer.AddEventHandlerWithResyncPeriod(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.handleService,
		UpdateFunc: c.updateService,
	}, resyncPeriod))

	lo.Must0(serviceInformer.AddIndexers(cache.Indexers{
		matchExternalServiceIndex: c.matchExternalServiceIndexFunc,
	}))

	c.shouldHandleService = c.shouldHandleServiceFunc(includeServices)
	return c
}

// Run begins processing items until the stopCh closed
func (c *Controller) Run(stopCh <-chan struct{}) {
	defer runtime.HandleCrash()
	defer c.reconcileQueue.ShutDown()

	if !cache.WaitForNamedCacheSync("ExternalServiceController", stopCh,
		c.serviceInformerSynced,
	) {
		return
	}

	ctx := wait.ContextForChannel(stopCh)
	go wait.UntilWithContext(ctx, c.reconcileWorker, time.Second)
	go wait.UntilWithContext(ctx, c.electionNotifier, time.Second)

	<-stopCh
}

func (c *Controller) handleService(obj interface{}) {
	unknown, ok := obj.(cache.DeletedFinalStateUnknown)
	if ok {
		obj = unknown.Obj
	}
	if service := obj.(*corev1.Service); c.shouldHandleService(service) {
		c.reconcileQueue.Add(types.NamespacedName{
			Namespace: service.Namespace,
			Name:      service.Name,
		})
	}
}

func (c *Controller) updateService(old interface{}, new interface{}) {
	oldService := old.(*corev1.Service)
	newService := new.(*corev1.Service)

	// handle service when service external-name update
	if newService.Spec.ExternalName != oldService.Spec.ExternalName && c.shouldHandleService(newService) {
		c.handleService(newService)
	}
}

func (c *Controller) shouldHandleServiceFunc(services []string) func(*corev1.Service) bool {
	includeServiceSet := sets.New(services...)

	return func(service *corev1.Service) bool {
		namespacedName := types.NamespacedName{Namespace: service.Namespace, Name: service.Name}.String()
		return service.Spec.Type == corev1.ServiceTypeExternalName && includeServiceSet.Has(namespacedName)
	}
}

func (c *Controller) matchExternalServiceIndexFunc(obj interface{}) ([]string, error) {
	if c.shouldHandleService(obj.(*corev1.Service)) {
		return []string{matchExternalServiceIndexValue}, nil
	}
	return nil, nil
}

func (c *Controller) reconcileWorker(ctx context.Context) {
	for {
		key, quit := c.reconcileQueue.Get()
		if quit {
			return
		}

		err := c.doReconcile(ctx, key.(types.NamespacedName))
		if err != nil {
			klog.Errorf("reconcile external service %v: %s", key.(types.NamespacedName).String(), err)
			c.reconcileQueue.AddRateLimited(key)
			c.reconcileQueue.Done(key)
			continue
		}

		// stop the rate limiter from tracking the key
		c.reconcileQueue.Done(key)
		c.reconcileQueue.Forget(key)
	}
}

func (c *Controller) electionNotifier(ctx context.Context) {
	for c.electionClient.UntilLeadingStateUpdate(ctx.Done()) {
		c.reconcileQueue.Add(types.NamespacedName{})
	}
}

func (c *Controller) doReconcile(ctx context.Context, namespacedName types.NamespacedName) error {
	if !c.electionClient.IsLeader() { // never update when no-longer leading
		return nil
	}

	services, err := c.fetchExternalServices(namespacedName)
	if err != nil {
		return fmt.Errorf("fetch external services: %w", err)
	}

	for _, service := range services {
		if c.electionClient.IsLeader() && c.shouldHandleService(service) && service.Spec.ExternalName != c.publicIP.String() {
			klog.Infof("update service %s/%s external name to %s", service.Namespace, service.Name, c.publicIP.String())
			updateService := service.DeepCopy()
			updateService.Spec.ExternalName = c.publicIP.String()
			_, err := c.clientset.CoreV1().Services(service.Namespace).Update(ctx, updateService, metav1.UpdateOptions{})
			if err != nil {
				return fmt.Errorf("update service %s/%s: %w", service.Namespace, service.Name, err)
			}
		}
	}
	return nil
}

func (c *Controller) fetchExternalServices(namespacedName types.NamespacedName) ([]*corev1.Service, error) {
	if namespacedName.Namespace == "" && namespacedName.Name == "" {
		objects, err := c.serviceLister.ByIndex(matchExternalServiceIndex, matchExternalServiceIndexValue)
		services := make([]*corev1.Service, 0, len(objects))
		for _, obj := range objects {
			services = append(services, obj.(*corev1.Service))
		}
		return services, err
	}

	service, exists, err := c.serviceLister.GetByKey(namespacedName.String())
	if !exists || err != nil {
		return nil, err
	}
	return []*corev1.Service{service.(*corev1.Service)}, nil
}
