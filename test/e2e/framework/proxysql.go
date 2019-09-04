package framework

import (
	"time"

	jsonTypes "github.com/appscode/go/encoding/json/types"
	"github.com/appscode/go/types"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"kubedb.dev/apimachinery/apis/kubedb"
	api "kubedb.dev/apimachinery/apis/kubedb/v1alpha1"
)

var (
	proxysqlPvcStorageSize = "50Mi"
)

func (f *Invocation) ProxySQL(backendObjName string) *api.ProxySQL {
	mode := api.LoadBalanceModeGalera

	return &api.ProxySQL{
		ObjectMeta: metav1.ObjectMeta{
			Name:      proxysqlName(backendObjName),
			Namespace: f.namespace,
			Labels: map[string]string{
				"app": f.app,
			},
		},
		Spec: api.ProxySQLSpec{
			Version:  jsonTypes.StrYo(ProxySQLCatalogName),
			Replicas: types.Int32P(1),
			Mode:     &mode,
			Backend: &corev1.TypedLocalObjectReference{
				APIGroup: types.StringP(kubedb.GroupName),
				Kind:     api.ResourceKindPerconaXtraDB,
				Name:     backendObjName,
			},
			Storage: &corev1.PersistentVolumeClaimSpec{
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse(proxysqlPvcStorageSize),
					},
				},
				StorageClassName: types.StringP(f.StorageClass),
			},
		},
	}
}

func (f *Framework) CreateProxySQL(obj *api.ProxySQL) error {
	_, err := f.dbClient.KubedbV1alpha1().ProxySQLs(obj.Namespace).Create(obj)
	return err
}

//func (f *Framework) GetProxySQL(meta metav1.ObjectMeta) (*api.ProxySQL, error) {
//	return f.dbClient.KubedbV1alpha1().ProxySQLs(meta.Namespace).Get(meta.Name, metav1.GetOptions{})
//}

//func (f *Framework) PatchProxySQL(meta metav1.ObjectMeta, transform func(*api.ProxySQL) *api.ProxySQL) (*api.ProxySQL, error) {
//	proxysql, err := f.dbClient.KubedbV1alpha1().ProxySQLs(meta.Namespace).Get(meta.Name, metav1.GetOptions{})
//	if err != nil {
//		return nil, err
//	}
//	proxysql, _, err = util.PatchProxySQL(f.dbClient.KubedbV1alpha1(), proxysql, transform)
//	return proxysql, err
//}

func (f *Framework) DeleteProxySQL(meta metav1.ObjectMeta) error {
	return f.dbClient.KubedbV1alpha1().ProxySQLs(meta.Namespace).Delete(meta.Name, &metav1.DeleteOptions{})
}

//func (f *Framework) EventuallyProxySQL(meta metav1.ObjectMeta) GomegaAsyncAssertion {
//	return Eventually(
//		func() bool {
//			_, err := f.dbClient.KubedbV1alpha1().ProxySQLs(meta.Namespace).Get(meta.Name, metav1.GetOptions{})
//			if err != nil {
//				if kerr.IsNotFound(err) {
//					return false
//				}
//				Expect(err).NotTo(HaveOccurred())
//			}
//			return true
//		},
//		time.Minute*5,
//		time.Second*5,
//	)
//}

func (f *Framework) EventuallyProxySQLPhase(meta metav1.ObjectMeta) GomegaAsyncAssertion {
	return Eventually(
		func() api.DatabasePhase {
			db, err := f.dbClient.KubedbV1alpha1().ProxySQLs(meta.Namespace).Get(meta.Name, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			return db.Status.Phase
		},
		time.Minute*5,
		time.Second*5,
	)
}

func (f *Framework) EventuallyProxySQLRunning(meta metav1.ObjectMeta) GomegaAsyncAssertion {
	return Eventually(
		func() bool {
			proxysql, err := f.dbClient.KubedbV1alpha1().ProxySQLs(meta.Namespace).Get(meta.Name, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			return proxysql.Status.Phase == api.DatabasePhaseRunning
		},
		time.Minute*15,
		time.Second*5,
	)
}

//func (f *Framework) CleanProxySQL() {
//	proxysqlList, err := f.dbClient.KubedbV1alpha1().ProxySQLs(f.namespace).List(metav1.ListOptions{})
//	if err != nil {
//		return
//	}
//	for _, proxysql := range proxysqlList.Items {
//		if _, _, err := util.PatchProxySQL(f.dbClient.KubedbV1alpha1(), &proxysql, func(in *api.ProxySQL) *api.ProxySQL {
//			in.ObjectMeta.Finalizers = nil
//			in.Spec.TerminationPolicy = api.TerminationPolicyWipeOut
//			return in
//		}); err != nil {
//			fmt.Printf("error Patching ProxySQL. error: %v", err)
//		}
//	}
//	if err := f.dbClient.KubedbV1alpha1().ProxySQLs(f.namespace).DeleteCollection(deleteInForeground(), metav1.ListOptions{}); err != nil {
//		fmt.Printf("error in deletion of ProxySQL. Error: %v", err)
//	}
//}
//
//func (f *Framework) WaitUntilProxySQLReplicasBePatched(meta metav1.ObjectMeta, count int32) error {
//	return wait.PollImmediate(kutil.RetryInterval, kutil.ReadinessTimeout, func() (bool, error) {
//		proxysql, err := f.GetProxySQL(meta)
//		if err != nil {
//			return false, nil
//		}
//
//		if *proxysql.Spec.Replicas != count {
//			return false, nil
//		}
//
//		return true, nil
//	})
//}
