/*
Copyright AppsCode Inc. and Contributors

Licensed under the AppsCode Community License 1.0.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    https://github.com/appscode/licenses/raw/1.0.0/AppsCode-Community-1.0.0.md

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package admission

import (
	"context"
	"net/http"
	"testing"

	catalog "kubedb.dev/apimachinery/apis/catalog/v1alpha1"
	api "kubedb.dev/apimachinery/apis/kubedb/v1alpha2"
	extFake "kubedb.dev/apimachinery/client/clientset/versioned/fake"
	"kubedb.dev/apimachinery/client/clientset/versioned/scheme"

	"github.com/appscode/go/types"
	admission "k8s.io/api/admission/v1beta1"
	authenticationV1 "k8s.io/api/authentication/v1"
	core "k8s.io/api/core/v1"
	storageV1beta1 "k8s.io/api/storage/v1beta1"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	clientSetScheme "k8s.io/client-go/kubernetes/scheme"
	kmapi "kmodules.xyz/client-go/api/v1"
	"kmodules.xyz/client-go/meta"
	mona "kmodules.xyz/monitoring-agent-api/api/v1"
)

var requestKind = metaV1.GroupVersionKind{
	Group:   api.SchemeGroupVersion.Group,
	Version: api.SchemeGroupVersion.Version,
	Kind:    api.ResourceKindPerconaXtraDB,
}

func TestPerconaXtraDBValidator_Admit(t *testing.T) {
	if err := scheme.AddToScheme(clientSetScheme.Scheme); err != nil {
		t.Error(err)
	}
	for _, c := range cases {
		t.Run(c.testName, func(t *testing.T) {
			validator := PerconaXtraDBValidator{}

			validator.initialized = true
			validator.extClient = extFake.NewSimpleClientset(
				&catalog.PerconaXtraDBVersion{
					ObjectMeta: metaV1.ObjectMeta{
						Name: "5.7",
					},
					Spec: catalog.PerconaXtraDBVersionSpec{
						Version: "5.7",
					},
				},
				&catalog.PerconaXtraDBVersion{
					ObjectMeta: metaV1.ObjectMeta{
						Name: "5.6",
					},
					Spec: catalog.PerconaXtraDBVersionSpec{
						Version: "5.6",
					},
				},
				&catalog.PerconaXtraDBVersion{
					ObjectMeta: metaV1.ObjectMeta{
						Name: "5.7.25",
					},
					Spec: catalog.PerconaXtraDBVersionSpec{
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
				if _, err := validator.extClient.KubedbV1alpha2().PerconaXtraDBs(c.namespace).Create(context.TODO(), &c.object, metaV1.CreateOptions{}); err != nil && !kerr.IsAlreadyExists(err) {
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
	object     api.PerconaXtraDB
	oldObject  api.PerconaXtraDB
	heatUp     bool
	result     bool
}{
	{"Create Valid PerconaXtraDB",
		requestKind,
		"foo",
		"default",
		admission.Create,
		samplePerconaXtraDB(),
		api.PerconaXtraDB{},
		false,
		true,
	},
	{"Create Invalid percona-xtradb",
		requestKind,
		"foo",
		"default",
		admission.Create,
		getAwkwardPerconaXtraDB(),
		api.PerconaXtraDB{},
		false,
		false,
	},
	{"Edit PerconaXtraDB Spec.DatabaseSecret with Existing Secret",
		requestKind,
		"foo",
		"default",
		admission.Update,
		editExistingSecret(samplePerconaXtraDB()),
		samplePerconaXtraDB(),
		false,
		true,
	},
	{"Edit PerconaXtraDB Spec.DatabaseSecret with non Existing Secret",
		requestKind,
		"foo",
		"default",
		admission.Update,
		editNonExistingSecret(samplePerconaXtraDB()),
		samplePerconaXtraDB(),
		false,
		true,
	},
	{"Edit Status",
		requestKind,
		"foo",
		"default",
		admission.Update,
		editStatus(samplePerconaXtraDB()),
		samplePerconaXtraDB(),
		false,
		true,
	},
	{"Edit Spec.Monitor",
		requestKind,
		"foo",
		"default",
		admission.Update,
		editSpecMonitor(samplePerconaXtraDB()),
		samplePerconaXtraDB(),
		false,
		true,
	},
	{"Edit Invalid Spec.Monitor",
		requestKind,
		"foo",
		"default",
		admission.Update,
		editSpecInvalidMonitor(samplePerconaXtraDB()),
		samplePerconaXtraDB(),
		false,
		false,
	},
	{"Edit Spec.TerminationPolicy",
		requestKind,
		"foo",
		"default",
		admission.Update,
		pauseDatabase(samplePerconaXtraDB()),
		samplePerconaXtraDB(),
		false,
		true,
	},
	{"Delete PerconaXtraDB when Spec.TerminationPolicy=DoNotTerminate",
		requestKind,
		"foo",
		"default",
		admission.Delete,
		samplePerconaXtraDB(),
		api.PerconaXtraDB{},
		true,
		false,
	},
	{"Delete PerconaXtraDB when Spec.TerminationPolicy=Pause",
		requestKind,
		"foo",
		"default",
		admission.Delete,
		pauseDatabase(samplePerconaXtraDB()),
		api.PerconaXtraDB{},
		true,
		true,
	},
	{"Delete Non Existing PerconaXtraDB",
		requestKind,
		"foo",
		"default",
		admission.Delete,
		api.PerconaXtraDB{},
		api.PerconaXtraDB{},
		false,
		true,
	},

	// XtraDB Cluster
	{"Create a valid PerconaXtraDB Cluster",
		requestKind,
		"foo",
		"default",
		admission.Create,
		sampleValidXtraDBCluster(),
		api.PerconaXtraDB{},
		false,
		true,
	},
	{"Create an invalid PerconaXtraDB Cluster containing initscript",
		requestKind,
		"foo",
		"default",
		admission.Create,
		sampleXtraDBClusterContainingInitsript(),
		api.PerconaXtraDB{},
		false,
		false,
	},
	{"Create PerconaXtraDB Cluster with insufficient node replicas",
		requestKind,
		"foo",
		"default",
		admission.Create,
		insufficientNodeReplicas(),
		api.PerconaXtraDB{},
		false,
		false,
	},
	{"Create PerconaXtraDB Cluster with larger cluster name than recommended",
		requestKind,
		"foo",
		"default",
		admission.Create,
		largerClusterNameThanRecommended(),
		api.PerconaXtraDB{},
		false,
		false,
	},
	{"Edit spec.Init before provisioning complete",
		requestKind,
		"foo",
		"default",
		admission.Update,
		updateInit(samplePerconaXtraDB()),
		samplePerconaXtraDB(),
		true,
		true,
	},
	{"Edit spec.Init after provisioning complete",
		requestKind,
		"foo",
		"default",
		admission.Update,
		updateInit(completeProvisioning(samplePerconaXtraDB())),
		samplePerconaXtraDB(),
		true,
		false,
	},
}

func samplePerconaXtraDB() api.PerconaXtraDB {
	return api.PerconaXtraDB{
		TypeMeta: metaV1.TypeMeta{
			Kind:       api.ResourceKindPerconaXtraDB,
			APIVersion: api.SchemeGroupVersion.String(),
		},
		ObjectMeta: metaV1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
			Labels: map[string]string{
				api.LabelDatabaseKind: api.ResourceKindPerconaXtraDB,
			},
		},
		Spec: api.PerconaXtraDBSpec{
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
				WaitForInitialRestore: true,
			},
			TerminationPolicy: api.TerminationPolicyDoNotTerminate,
		},
	}
}

func getAwkwardPerconaXtraDB() api.PerconaXtraDB {
	px := samplePerconaXtraDB()
	px.Spec.Version = "3.0"
	return px
}

func editExistingSecret(old api.PerconaXtraDB) api.PerconaXtraDB {
	old.Spec.DatabaseSecret = &core.SecretVolumeSource{
		SecretName: "foo-auth",
	}
	return old
}

func editNonExistingSecret(old api.PerconaXtraDB) api.PerconaXtraDB {
	old.Spec.DatabaseSecret = &core.SecretVolumeSource{
		SecretName: "foo-auth-fused",
	}
	return old
}

func editStatus(old api.PerconaXtraDB) api.PerconaXtraDB {
	old.Status = api.PerconaXtraDBStatus{
		Phase: api.DatabasePhaseReady,
	}
	return old
}

func editSpecMonitor(old api.PerconaXtraDB) api.PerconaXtraDB {
	old.Spec.Monitor = &mona.AgentSpec{
		Agent: mona.AgentPrometheusBuiltin,
		Prometheus: &mona.PrometheusSpec{
			Exporter: mona.PrometheusExporterSpec{
				Port: 1289,
			},
		},
	}
	return old
}

// should be failed because more fields required for COreOS Monitoring
func editSpecInvalidMonitor(old api.PerconaXtraDB) api.PerconaXtraDB {
	old.Spec.Monitor = &mona.AgentSpec{
		Agent: mona.AgentPrometheusOperator,
	}
	return old
}

func pauseDatabase(old api.PerconaXtraDB) api.PerconaXtraDB {
	old.Spec.TerminationPolicy = api.TerminationPolicyHalt
	return old
}

func sampleXtraDBClusterContainingInitsript() api.PerconaXtraDB {
	perconaxtradb := samplePerconaXtraDB()
	perconaxtradb.Spec.Replicas = types.Int32P(api.PerconaXtraDBDefaultClusterSize)
	perconaxtradb.Spec.Init = &api.InitSpec{
		Script: &api.ScriptSourceSpec{
			VolumeSource: core.VolumeSource{
				GitRepo: &core.GitRepoVolumeSource{
					Repository: "https://kubedb.dev/percona-xtradb-init-scripts.git",
					Directory:  ".",
				},
			},
		},
	}

	return perconaxtradb
}

func sampleValidXtraDBCluster() api.PerconaXtraDB {
	perconaxtradb := samplePerconaXtraDB()
	perconaxtradb.Spec.Replicas = types.Int32P(api.PerconaXtraDBDefaultClusterSize)
	if perconaxtradb.Spec.Init != nil {
		perconaxtradb.Spec.Init.Script = nil
	}

	return perconaxtradb
}

func insufficientNodeReplicas() api.PerconaXtraDB {
	perconaxtradb := sampleValidXtraDBCluster()
	perconaxtradb.Spec.Replicas = types.Int32P(2)

	return perconaxtradb
}

func largerClusterNameThanRecommended() api.PerconaXtraDB {
	perconaxtradb := sampleValidXtraDBCluster()
	perconaxtradb.Name = "aaaaa-aaaaa-aaaaa-aaaaa-aaaaa-aaaaa"

	return perconaxtradb
}

func completeProvisioning(old api.PerconaXtraDB) api.PerconaXtraDB {
	old.Status.Conditions = []kmapi.Condition{
		{
			Type:   api.DatabaseProvisioned,
			Status: kmapi.ConditionTrue,
		},
	}
	return old
}

func updateInit(old api.PerconaXtraDB) api.PerconaXtraDB {
	old.Spec.Init.WaitForInitialRestore = false
	return old
}
