package admission

import (
	"net/http"
	"testing"

	"github.com/appscode/go/types"
	admission "k8s.io/api/admission/v1beta1"
	apps "k8s.io/api/apps/v1"
	authenticationV1 "k8s.io/api/authentication/v1"
	core "k8s.io/api/core/v1"
	storageV1beta1 "k8s.io/api/storage/v1beta1"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	clientSetScheme "k8s.io/client-go/kubernetes/scheme"
	"kmodules.xyz/client-go/meta"
	mona "kmodules.xyz/monitoring-agent-api/api/v1"
	catalog "kubedb.dev/apimachinery/apis/catalog/v1alpha1"
	api "kubedb.dev/apimachinery/apis/kubedb/v1alpha1"
	extFake "kubedb.dev/apimachinery/client/clientset/versioned/fake"
	"kubedb.dev/apimachinery/client/clientset/versioned/scheme"
)

func init() {
	scheme.AddToScheme(clientSetScheme.Scheme)
}

var requestKind = metaV1.GroupVersionKind{
	Group:   api.SchemeGroupVersion.Group,
	Version: api.SchemeGroupVersion.Version,
	Kind:    api.ResourceKindPercona,
}

func TestPerconaValidator_Admit(t *testing.T) {
	for _, c := range cases {
		t.Run(c.testName, func(t *testing.T) {
			validator := PerconaValidator{}

			validator.initialized = true
			validator.extClient = extFake.NewSimpleClientset(
				&catalog.PerconaVersion{
					ObjectMeta: metaV1.ObjectMeta{
						Name: "5.7",
					},
					Spec: catalog.PerconaVersionSpec{
						Version: "5.7",
					},
				},
				&catalog.PerconaVersion{
					ObjectMeta: metaV1.ObjectMeta{
						Name: "5.6",
					},
					Spec: catalog.PerconaVersionSpec{
						Version: "5.6",
					},
				},
				&catalog.PerconaVersion{
					ObjectMeta: metaV1.ObjectMeta{
						Name: "5.7.25",
					},
					Spec: catalog.PerconaVersionSpec{
						Version: "5.7.25",
					},
				},
			)
			validator.client = fake.NewSimpleClientset(
				&core.Secret{
					ObjectMeta: metaV1.ObjectMeta{
						Name:      "foo-auth",
						Namespace: "default",
					},
				},
				&storageV1beta1.StorageClass{
					ObjectMeta: metaV1.ObjectMeta{
						Name: "standard",
					},
				},
			)

			objJS, err := meta.MarshalToJson(&c.object, api.SchemeGroupVersion)
			if err != nil {
				panic(err)
			}
			oldObjJS, err := meta.MarshalToJson(&c.oldObject, api.SchemeGroupVersion)
			if err != nil {
				panic(err)
			}

			req := new(admission.AdmissionRequest)

			req.Kind = c.kind
			req.Name = c.objectName
			req.Namespace = c.namespace
			req.Operation = c.operation
			req.UserInfo = authenticationV1.UserInfo{}
			req.Object.Raw = objJS
			req.OldObject.Raw = oldObjJS

			if c.heatUp {
				if _, err := validator.extClient.KubedbV1alpha1().Perconas(c.namespace).Create(&c.object); err != nil && !kerr.IsAlreadyExists(err) {
					t.Errorf(err.Error())
				}
			}
			if c.operation == admission.Delete {
				req.Object = runtime.RawExtension{}
			}
			if c.operation != admission.Update {
				req.OldObject = runtime.RawExtension{}
			}

			response := validator.Admit(req)
			if c.result == true {
				if response.Allowed != true {
					t.Errorf("expected: 'Allowed=true'. but got response: %v", response)
				}
			} else if c.result == false {
				if response.Allowed == true || response.Result.Code == http.StatusInternalServerError {
					t.Errorf("expected: 'Allowed=false', but got response: %v", response)
				}
			}
		})
	}

}

var cases = []struct {
	testName   string
	kind       metaV1.GroupVersionKind
	objectName string
	namespace  string
	operation  admission.Operation
	object     api.Percona
	oldObject  api.Percona
	heatUp     bool
	result     bool
}{
	{"Create Valid Percona",
		requestKind,
		"foo",
		"default",
		admission.Create,
		samplePercona(),
		api.Percona{},
		false,
		true,
	},
	{"Create Percona without single node replicas",
		requestKind,
		"foo",
		"default",
		admission.Create,
		perconaWithoutSingleReplica(),
		api.Percona{},
		false,
		false,
	},
	{"Create Invalid Percona",
		requestKind,
		"foo",
		"default",
		admission.Create,
		getAwkwardPercona(),
		api.Percona{},
		false,
		false,
	},
	{"Edit Percona Spec.DatabaseSecret with Existing Secret",
		requestKind,
		"foo",
		"default",
		admission.Update,
		editExistingSecret(samplePercona()),
		samplePercona(),
		false,
		true,
	},
	{"Edit Percona Spec.DatabaseSecret with non Existing Secret",
		requestKind,
		"foo",
		"default",
		admission.Update,
		editNonExistingSecret(samplePercona()),
		samplePercona(),
		false,
		true,
	},
	{"Edit Status",
		requestKind,
		"foo",
		"default",
		admission.Update,
		editStatus(samplePercona()),
		samplePercona(),
		false,
		true,
	},
	{"Edit Spec.Monitor",
		requestKind,
		"foo",
		"default",
		admission.Update,
		editSpecMonitor(samplePercona()),
		samplePercona(),
		false,
		true,
	},
	{"Edit Invalid Spec.Monitor",
		requestKind,
		"foo",
		"default",
		admission.Update,
		editSpecInvalidMonitor(samplePercona()),
		samplePercona(),
		false,
		false,
	},
	{"Edit Spec.TerminationPolicy",
		requestKind,
		"foo",
		"default",
		admission.Update,
		pauseDatabase(samplePercona()),
		samplePercona(),
		false,
		true,
	},
	{"Delete Percona when Spec.TerminationPolicy=DoNotTerminate",
		requestKind,
		"foo",
		"default",
		admission.Delete,
		samplePercona(),
		api.Percona{},
		true,
		false,
	},
	{"Delete Percona when Spec.TerminationPolicy=Pause",
		requestKind,
		"foo",
		"default",
		admission.Delete,
		pauseDatabase(samplePercona()),
		api.Percona{},
		true,
		true,
	},
	{"Delete Non Existing Percona",
		requestKind,
		"foo",
		"default",
		admission.Delete,
		api.Percona{},
		api.Percona{},
		false,
		true,
	},

	// XtraDB Cluster
	{"Create a valid Percona XtraDB Cluster",
		requestKind,
		"foo",
		"default",
		admission.Create,
		sampleXtraDBCluster(),
		api.Percona{},
		false,
		true,
	},
	{"Create Percona XtraDB Cluster with insufficient node replicas",
		requestKind,
		"foo",
		"default",
		admission.Create,
		insufficientNodeReplicas(),
		api.Percona{},
		false,
		false,
	},
	{"Create Percona XtraDB Cluster with empty cluster name",
		requestKind,
		"foo",
		"default",
		admission.Create,
		emptyClusterName(),
		api.Percona{},
		false,
		false,
	},
	{"Create Percona XtraDB Cluster with larger cluster name than recommended",
		requestKind,
		"foo",
		"default",
		admission.Create,
		largerClusterNameThanRecommended(),
		api.Percona{},
		false,
		false,
	},
	{"Create Percona XtraDB Cluster without single proxysql replicas",
		requestKind,
		"foo",
		"default",
		admission.Create,
		withoutSingleProxysqlReplicas(),
		api.Percona{},
		false,
		false,
	},
	{"Create Percona XtraDB Cluster with 0 proxysql replicas",
		requestKind,
		"foo",
		"default",
		admission.Create,
		withZeroProxysqlReplicas(),
		api.Percona{},
		false,
		false,
	},
}

func samplePercona() api.Percona {
	return api.Percona{
		TypeMeta: metaV1.TypeMeta{
			Kind:       api.ResourceKindPercona,
			APIVersion: api.SchemeGroupVersion.String(),
		},
		ObjectMeta: metaV1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
			Labels: map[string]string{
				api.LabelDatabaseKind: api.ResourceKindPercona,
			},
		},
		Spec: api.PerconaSpec{
			Version:     "5.7",
			Replicas:    types.Int32P(1),
			StorageType: api.StorageTypeDurable,
			Storage: &core.PersistentVolumeClaimSpec{
				StorageClassName: types.StringP("standard"),
				Resources: core.ResourceRequirements{
					Requests: core.ResourceList{
						core.ResourceStorage: resource.MustParse("100Mi"),
					},
				},
			},
			Init: &api.InitSpec{
				ScriptSource: &api.ScriptSourceSpec{
					VolumeSource: core.VolumeSource{
						GitRepo: &core.GitRepoVolumeSource{
							Repository: "https://kubedb.dev/percona-xtradb-init-scripts.git",
							Directory:  ".",
						},
					},
				},
			},
			UpdateStrategy: apps.StatefulSetUpdateStrategy{
				Type: apps.RollingUpdateStatefulSetStrategyType,
			},
			TerminationPolicy: api.TerminationPolicyDoNotTerminate,
		},
	}
}

func getAwkwardPercona() api.Percona {
	pxc := samplePercona()
	pxc.Spec.Version = "3.0"
	return pxc
}

func perconaWithoutSingleReplica() api.Percona {
	pxc := samplePercona()
	pxc.Spec.Replicas = types.Int32P(3)
	return pxc
}

func editExistingSecret(old api.Percona) api.Percona {
	old.Spec.DatabaseSecret = &core.SecretVolumeSource{
		SecretName: "foo-auth",
	}
	return old
}

func editNonExistingSecret(old api.Percona) api.Percona {
	old.Spec.DatabaseSecret = &core.SecretVolumeSource{
		SecretName: "foo-auth-fused",
	}
	return old
}

func editStatus(old api.Percona) api.Percona {
	old.Status = api.PerconaStatus{
		Phase: api.DatabasePhaseCreating,
	}
	return old
}

func editSpecMonitor(old api.Percona) api.Percona {
	old.Spec.Monitor = &mona.AgentSpec{
		Agent: mona.AgentPrometheusBuiltin,
		Prometheus: &mona.PrometheusSpec{
			Port: 1289,
		},
	}
	return old
}

// should be failed because more fields required for COreOS Monitoring
func editSpecInvalidMonitor(old api.Percona) api.Percona {
	old.Spec.Monitor = &mona.AgentSpec{
		Agent: mona.AgentCoreOSPrometheus,
	}
	return old
}

func pauseDatabase(old api.Percona) api.Percona {
	old.Spec.TerminationPolicy = api.TerminationPolicyPause
	return old
}

func sampleXtraDBCluster() api.Percona {
	percona := samplePercona()
	percona.Spec.Replicas = types.Int32P(3)
	percona.Spec.PXC = &api.PXCSpec{
		ClusterName: "foo-xtradb-cluster",
		Proxysql: api.ProxysqlSpec{
			Replicas: types.Int32P(1),
		},
	}

	return percona
}

func insufficientNodeReplicas() api.Percona {
	percona := sampleXtraDBCluster()
	percona.Spec.Replicas = types.Int32P(1)

	return percona
}

func emptyClusterName() api.Percona {
	percona := sampleXtraDBCluster()
	percona.Spec.PXC.ClusterName = ""

	return percona
}

func largerClusterNameThanRecommended() api.Percona {
	percona := sampleXtraDBCluster()
	percona.Spec.PXC.ClusterName = "aaaaa-aaaaa-aaaaa-aaaaa-aaaaa-aaaaa"

	return percona
}

func withoutSingleProxysqlReplicas() api.Percona {
	percona := sampleXtraDBCluster()
	percona.Spec.PXC.Proxysql.Replicas = types.Int32P(3)

	return percona
}

func withZeroProxysqlReplicas() api.Percona {
	percona := sampleXtraDBCluster()
	percona.Spec.PXC.Proxysql.Replicas = types.Int32P(0)

	return percona
}
