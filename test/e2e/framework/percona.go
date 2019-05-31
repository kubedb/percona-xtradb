package framework

import (
	"fmt"
	"time"

	"github.com/appscode/go/crypto/rand"
	jsonTypes "github.com/appscode/go/encoding/json/types"
	"github.com/appscode/go/types"
	api "github.com/kubedb/apimachinery/apis/kubedb/v1alpha1"
	"github.com/kubedb/apimachinery/client/clientset/versioned/typed/kubedb/v1alpha1/util"
	. "github.com/onsi/gomega"
	core "k8s.io/api/core/v1"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	JobPvcStorageSize = "2Gi"
	DBPvcStorageSize  = "1Gi"
)

func (f *Invocation) Percona() *api.Percona {
	return &api.Percona{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rand.WithUniqSuffix("percona"),
			Namespace: f.namespace,
			Labels: map[string]string{
				"app": f.app,
			},
		},
		Spec: api.PerconaSpec{
			Version: jsonTypes.StrYo(DBVersion),
			Storage: &core.PersistentVolumeClaimSpec{
				Resources: core.ResourceRequirements{
					Requests: core.ResourceList{
						core.ResourceStorage: resource.MustParse(DBPvcStorageSize),
					},
				},
				StorageClassName: types.StringP(f.StorageClass),
			},
		},
	}
}

func (f *Framework) CreatePercona(obj *api.Percona) error {
	_, err := f.extClient.KubedbV1alpha1().Perconas(obj.Namespace).Create(obj)
	return err
}

func (f *Framework) GetPercona(meta metav1.ObjectMeta) (*api.Percona, error) {
	return f.extClient.KubedbV1alpha1().Perconas(meta.Namespace).Get(meta.Name, metav1.GetOptions{})
}

func (f *Framework) PatchPercona(meta metav1.ObjectMeta, transform func(*api.Percona) *api.Percona) (*api.Percona, error) {
	percona, err := f.extClient.KubedbV1alpha1().Perconas(meta.Namespace).Get(meta.Name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	percona, _, err = util.PatchPercona(f.extClient.KubedbV1alpha1(), percona, transform)
	return percona, err
}

func (f *Framework) DeletePercona(meta metav1.ObjectMeta) error {
	return f.extClient.KubedbV1alpha1().Perconas(meta.Namespace).Delete(meta.Name, &metav1.DeleteOptions{})
}

func (f *Framework) EventuallyPercona(meta metav1.ObjectMeta) GomegaAsyncAssertion {
	return Eventually(
		func() bool {
			_, err := f.extClient.KubedbV1alpha1().Perconas(meta.Namespace).Get(meta.Name, metav1.GetOptions{})
			if err != nil {
				if kerr.IsNotFound(err) {
					return false
				}
				Expect(err).NotTo(HaveOccurred())
			}
			return true
		},
		time.Minute*5,
		time.Second*5,
	)
}

func (f *Framework) EventuallyPerconaRunning(meta metav1.ObjectMeta) GomegaAsyncAssertion {
	return Eventually(
		func() bool {
			percona, err := f.extClient.KubedbV1alpha1().Perconas(meta.Namespace).Get(meta.Name, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			return percona.Status.Phase == api.DatabasePhaseRunning
		},
		time.Minute*15,
		time.Second*5,
	)
}

func (f *Framework) CleanPercona() {
	perconaList, err := f.extClient.KubedbV1alpha1().Perconas(f.namespace).List(metav1.ListOptions{})
	if err != nil {
		return
	}
	for _, e := range perconaList.Items {
		if _, _, err := util.PatchPercona(f.extClient.KubedbV1alpha1(), &e, func(in *api.Percona) *api.Percona {
			in.ObjectMeta.Finalizers = nil
			in.Spec.TerminationPolicy = api.TerminationPolicyWipeOut
			return in
		}); err != nil {
			fmt.Printf("error Patching Percona. error: %v", err)
		}
	}
	if err := f.extClient.KubedbV1alpha1().Perconas(f.namespace).DeleteCollection(deleteInForeground(), metav1.ListOptions{}); err != nil {
		fmt.Printf("error in deletion of Percona. Error: %v", err)
	}
}
