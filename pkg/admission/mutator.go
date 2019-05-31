package admission

import (
	"fmt"
	"sync"

	"github.com/appscode/go/log"
	api "github.com/kubedb/apimachinery/apis/kubedb/v1alpha1"
	cs "github.com/kubedb/apimachinery/client/clientset/versioned"
	"github.com/pkg/errors"
	admission "k8s.io/api/admission/v1beta1"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	kutil "kmodules.xyz/client-go"
	core_util "kmodules.xyz/client-go/core/v1"
	meta_util "kmodules.xyz/client-go/meta"
	mona "kmodules.xyz/monitoring-agent-api/api/v1"
	hookapi "kmodules.xyz/webhook-runtime/admission/v1beta1"
)

type PerconaMutator struct {
	client      kubernetes.Interface
	extClient   cs.Interface
	lock        sync.RWMutex
	initialized bool
}

var _ hookapi.AdmissionHook = &PerconaMutator{}

func (a *PerconaMutator) Resource() (plural schema.GroupVersionResource, singular string) {
	return schema.GroupVersionResource{
			Group:    "mutators.kubedb.com",
			Version:  "v1alpha1",
			Resource: "perconamutators",
		},
		"perconamutator"
}

func (a *PerconaMutator) Initialize(config *rest.Config, stopCh <-chan struct{}) error {
	a.lock.Lock()
	defer a.lock.Unlock()

	a.initialized = true

	var err error
	if a.client, err = kubernetes.NewForConfig(config); err != nil {
		return err
	}
	if a.extClient, err = cs.NewForConfig(config); err != nil {
		return err
	}
	return err
}

func (a *PerconaMutator) Admit(req *admission.AdmissionRequest) *admission.AdmissionResponse {
	status := &admission.AdmissionResponse{}

	// N.B.: No Mutating for delete
	if (req.Operation != admission.Create && req.Operation != admission.Update) ||
		len(req.SubResource) != 0 ||
		req.Kind.Group != api.SchemeGroupVersion.Group ||
		req.Kind.Kind != api.ResourceKindPercona {
		status.Allowed = true
		return status
	}

	a.lock.RLock()
	defer a.lock.RUnlock()
	if !a.initialized {
		return hookapi.StatusUninitialized()
	}
	obj, err := meta_util.UnmarshalFromJSON(req.Object.Raw, api.SchemeGroupVersion)
	if err != nil {
		return hookapi.StatusBadRequest(err)
	}
	perconaMod, err := setDefaultValues(a.client, a.extClient, obj.(*api.Percona).DeepCopy())
	if err != nil {
		return hookapi.StatusForbidden(err)
	} else if perconaMod != nil {
		patch, err := meta_util.CreateJSONPatch(req.Object.Raw, perconaMod)
		if err != nil {
			return hookapi.StatusInternalServerError(err)
		}
		status.Patch = patch
		patchType := admission.PatchTypeJSONPatch
		status.PatchType = &patchType
	}

	status.Allowed = true
	return status
}

// setDefaultValues provides the defaulting that is performed in mutating stage of creating/updating a MySQL database
func setDefaultValues(client kubernetes.Interface, extClient cs.Interface, pxc *api.Percona) (runtime.Object, error) {
	if pxc.Spec.Version == "" {
		return nil, errors.New(`'spec.version' is missing`)
	}

	pxc.SetDefaults()

	if err := setDefaultsFromDormantDB(extClient, pxc); err != nil {
		return nil, err
	}

	// If monitoring spec is given without port,
	// set default Listening port
	setMonitoringPort(pxc)

	return pxc, nil
}

// setDefaultsFromDormantDB takes values from Similar Dormant Database
func setDefaultsFromDormantDB(extClient cs.Interface, pxc *api.Percona) error {
	// Check if DormantDatabase exists or not
	dormantDb, err := extClient.KubedbV1alpha1().DormantDatabases(pxc.Namespace).Get(pxc.Name, metav1.GetOptions{})
	if err != nil {
		if !kerr.IsNotFound(err) {
			return err
		}
		return nil
	}

	// Check DatabaseKind
	if value, _ := meta_util.GetStringValue(dormantDb.Labels, api.LabelDatabaseKind); value != api.ResourceKindPercona {
		return errors.New(fmt.Sprintf(`invalid Percona: "%v/%v". Exists DormantDatabase "%v/%v" of different Kind`, pxc.Namespace, pxc.Name, dormantDb.Namespace, dormantDb.Name))
	}

	// Check Origin Spec
	ddbOriginSpec := dormantDb.Spec.Origin.Spec.Percona
	ddbOriginSpec.SetDefaults()

	// If DatabaseSecret of new object is not given,
	// Take dormantDatabaseSecretName
	if pxc.Spec.DatabaseSecret == nil {
		pxc.Spec.DatabaseSecret = ddbOriginSpec.DatabaseSecret
	}

	// If Monitoring Spec of new object is not given,
	// Take Monitoring Settings from Dormant
	if pxc.Spec.Monitor == nil {
		pxc.Spec.Monitor = ddbOriginSpec.Monitor
	} else {
		ddbOriginSpec.Monitor = pxc.Spec.Monitor
	}

	// Skip checking UpdateStrategy
	ddbOriginSpec.UpdateStrategy = pxc.Spec.UpdateStrategy

	// Skip checking TerminationPolicy
	ddbOriginSpec.TerminationPolicy = pxc.Spec.TerminationPolicy

	if !meta_util.Equal(ddbOriginSpec, &pxc.Spec) {
		diff := meta_util.Diff(ddbOriginSpec, &pxc.Spec)
		log.Errorf("percona spec mismatches with OriginSpec in DormantDatabases. Diff: %v", diff)
		return errors.New(fmt.Sprintf("percona spec mismatches with OriginSpec in DormantDatabases. Diff: %v", diff))
	}

	if _, err := meta_util.GetString(pxc.Annotations, api.AnnotationInitialized); err == kutil.ErrNotFound &&
		pxc.Spec.Init != nil &&
		pxc.Spec.Init.SnapshotSource != nil {
		pxc.Annotations = core_util.UpsertMap(pxc.Annotations, map[string]string{
			api.AnnotationInitialized: "",
		})
	}

	// Delete  Matching dormantDatabase in Controller

	return nil
}

// Assign Default Monitoring Port if MonitoringSpec Exists
// and the AgentVendor is Prometheus.
func setMonitoringPort(pxc *api.Percona) {
	if pxc.Spec.Monitor != nil &&
		pxc.GetMonitoringVendor() == mona.VendorPrometheus {
		if pxc.Spec.Monitor.Prometheus == nil {
			pxc.Spec.Monitor.Prometheus = &mona.PrometheusSpec{}
		}
		if pxc.Spec.Monitor.Prometheus.Port == 0 {
			pxc.Spec.Monitor.Prometheus.Port = api.PrometheusExporterPortNumber
		}
	}
}
