package controller

import (
	"github.com/appscode/go/encoding/json/types"
	"github.com/appscode/go/log"
	pcm "github.com/coreos/prometheus-operator/pkg/client/versioned/typed/monitoring/v1"
	core "k8s.io/api/core/v1"
	crd_api "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	crd_cs "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/labels"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	reg_util "kmodules.xyz/client-go/admissionregistration/v1beta1"
	apiext_util "kmodules.xyz/client-go/apiextensions/v1beta1"
	meta_util "kmodules.xyz/client-go/meta"
	"kmodules.xyz/client-go/tools/queue"
	appcat "kmodules.xyz/custom-resources/apis/appcatalog/v1alpha1"
	appcat_cs "kmodules.xyz/custom-resources/client/clientset/versioned/typed/appcatalog/v1alpha1"
	"kubedb.dev/apimachinery/apis"
	authorization "kubedb.dev/apimachinery/apis/authorization/v1alpha1"
	catalog "kubedb.dev/apimachinery/apis/catalog/v1alpha1"
	api "kubedb.dev/apimachinery/apis/kubedb/v1alpha1"
	cs "kubedb.dev/apimachinery/client/clientset/versioned"
	"kubedb.dev/apimachinery/client/clientset/versioned/typed/kubedb/v1alpha1/util"
	api_listers "kubedb.dev/apimachinery/client/listers/kubedb/v1alpha1"
	amc "kubedb.dev/apimachinery/pkg/controller"
	drmnc "kubedb.dev/apimachinery/pkg/controller/dormantdatabase"
	snapc "kubedb.dev/apimachinery/pkg/controller/snapshot"
	"kubedb.dev/apimachinery/pkg/eventer"
)

type Controller struct {
	amc.Config
	*amc.Controller

	// Prometheus client
	promClient pcm.MonitoringV1Interface
	// Cron Controller
	cronController snapc.CronControllerInterface
	// Event Recorder
	recorder record.EventRecorder
	// labelselector for event-handler of Snapshot, Dormant and Job
	selector labels.Selector

	// Percona
	pxcQueue    *queue.Worker
	pxcInformer cache.SharedIndexInformer
	pxcLister   api_listers.PerconaLister
}

//var _ amc.Snapshotter = &Controller{}
var _ amc.Deleter = &Controller{}

func New(
	clientConfig *rest.Config,
	client kubernetes.Interface,
	apiExtKubeClient crd_cs.ApiextensionsV1beta1Interface,
	extClient cs.Interface,
	dynamicClient dynamic.Interface,
	appCatalogClient appcat_cs.AppcatalogV1alpha1Interface,
	promClient pcm.MonitoringV1Interface,
	cronController snapc.CronControllerInterface,
	opt amc.Config,
	recorder record.EventRecorder,
) *Controller {
	return &Controller{
		Controller: &amc.Controller{
			ClientConfig:     clientConfig,
			Client:           client,
			ExtClient:        extClient,
			ApiExtKubeClient: apiExtKubeClient,
			DynamicClient:    dynamicClient,
			AppCatalogClient: appCatalogClient,
		},
		Config:         opt,
		promClient:     promClient,
		cronController: cronController,
		recorder:       recorder,
		selector: labels.SelectorFromSet(map[string]string{
			api.LabelDatabaseKind: api.ResourceKindPercona,
		}),
	}
}

// Ensuring Custom Resource Definitions
func (c *Controller) EnsureCustomResourceDefinitions() error {
	log.Infoln("Ensuring CustomResourceDefinition...")
	crds := []*crd_api.CustomResourceDefinition{
		api.Percona{}.CustomResourceDefinition(),
		catalog.PerconaVersion{}.CustomResourceDefinition(),
		api.DormantDatabase{}.CustomResourceDefinition(),
		api.Snapshot{}.CustomResourceDefinition(),
		// TODO: need to be clear about this role crd and tasks related to this
		authorization.MySQLRole{}.CustomResourceDefinition(),
		authorization.DatabaseAccessRequest{}.CustomResourceDefinition(),
		appcat.AppBinding{}.CustomResourceDefinition(),
	}
	return apiext_util.RegisterCRDs(c.ApiExtKubeClient, crds)
}

// Init initializes percona, DormantDB amd Snapshot watcher
func (c *Controller) Init() error {
	c.initWatcher()
	c.DrmnQueue = drmnc.NewController(c.Controller, c, c.Config, nil, c.recorder).AddEventHandlerFunc(c.selector)
	//c.SnapQueue, c.JobQueue = snapc.NewController(c.Controller, c, c.Config, nil, c.recorder).AddEventHandlerFunc(c.selector)

	return nil
}

// RunControllers runs queue.worker
func (c *Controller) RunControllers(stopCh <-chan struct{}) {
	// Start Cron
	c.cronController.StartCron()

	// Watch x  TPR objects
	c.pxcQueue.Run(stopCh)
	c.DrmnQueue.Run(stopCh)
	//c.SnapQueue.Run(stopCh)
	//c.JobQueue.Run(stopCh)
}

// Blocks caller. Intended to be called as a Go routine.
func (c *Controller) Run(stopCh <-chan struct{}) {
	go c.StartAndRunControllers(stopCh)

	if c.EnableMutatingWebhook {
		cancel1, _ := reg_util.SyncMutatingWebhookCABundle(c.ClientConfig, mutatingWebhookConfig)
		defer cancel1()
	}
	if c.EnableValidatingWebhook {
		cancel2, _ := reg_util.SyncValidatingWebhookCABundle(c.ClientConfig, validatingWebhookConfig)
		defer cancel2()
	}

	<-stopCh
	c.cronController.StopCron()
}

// StartAndRunControllers starts InformetFactory and runs queue.worker
func (c *Controller) StartAndRunControllers(stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()

	log.Infoln("Starting KubeDB controller")
	c.KubeInformerFactory.Start(stopCh)
	c.KubedbInformerFactory.Start(stopCh)

	// Wait for all involved caches to be synced, before processing items from the queue is started
	for t, v := range c.KubeInformerFactory.WaitForCacheSync(stopCh) {
		if !v {
			log.Fatalf("%v timed out waiting for caches to sync", t)
			return
		}
	}
	for t, v := range c.KubedbInformerFactory.WaitForCacheSync(stopCh) {
		if !v {
			log.Fatalf("%v timed out waiting for caches to sync", t)
			return
		}
	}

	c.RunControllers(stopCh)

	<-stopCh
	log.Infoln("Stopping KubeDB controller")
}

func (c *Controller) pushFailureEvent(pxc *api.Percona, reason string) {
	c.recorder.Eventf(
		pxc,
		core.EventTypeWarning,
		eventer.EventReasonFailedToStart,
		`Fail to be ready Percona: "%v". Reason: %v`,
		pxc.Name,
		reason,
	)

	per, err := util.UpdatePerconaStatus(c.ExtClient.KubedbV1alpha1(), pxc, func(in *api.PerconaStatus) *api.PerconaStatus {
		in.Phase = api.DatabasePhaseFailed
		in.Reason = reason
		in.ObservedGeneration = types.NewIntHash(pxc.Generation, meta_util.GenerationHash(pxc))
		return in
	}, apis.EnableStatusSubresource)

	if err != nil {
		c.recorder.Eventf(
			pxc,
			core.EventTypeWarning,
			eventer.EventReasonFailedToUpdate,
			err.Error(),
		)
	}
	pxc.Status = per.Status
}
