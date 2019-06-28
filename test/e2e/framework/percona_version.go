package framework

import (
	"fmt"

	api "github.com/kubedb/apimachinery/apis/catalog/v1alpha1"
	. "github.com/onsi/gomega"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (i *Invocation) PerconaVersion() *api.PerconaVersion {
	return &api.PerconaVersion{
		ObjectMeta: metav1.ObjectMeta{
			Name: DBVersion,
			Labels: map[string]string{
				"app": i.app,
			},
		},
		Spec: api.PerconaVersionSpec{
			Version: DBVersion,
			DB: api.PerconaVersionDatabase{
				Image: fmt.Sprintf("%s/percona-xtradb-cluster:%s", DockerRegistry, DBVersion),
			},
			Proxysql: api.PerconaVersionProxysql{
				Image: fmt.Sprintf("%s/proxysql-pxc:%s", DockerRegistry, DBVersion),
			},
			Exporter: api.PerconaVersionExporter{
				Image: fmt.Sprintf("%s/mysqld-exporter:%s", DockerRegistry, ExporterTag),
			},
			InitContainer: api.PerconaVersionInitContainer{
				Image: "kubedb/busybox",
			},
			//PodSecurityPolicies: api.PerconaVersionPodSecurityPolicy{
			//	DatabasePolicyName: "percona-db",
			//},
		},
	}
}

func (f *Framework) CreatePerconaVersion(obj *api.PerconaVersion) error {
	_, err := f.extClient.CatalogV1alpha1().PerconaVersions().Create(obj)
	if err != nil && kerr.IsAlreadyExists(err) {
		e2 := f.extClient.CatalogV1alpha1().PerconaVersions().Delete(obj.Name, &metav1.DeleteOptions{})
		Expect(e2).NotTo(HaveOccurred())
		_, e2 = f.extClient.CatalogV1alpha1().PerconaVersions().Create(obj)
		return e2
	}
	return nil
}

func (f *Framework) GetPerconaVersion(meta metav1.ObjectMeta) (*api.PerconaVersion, error) {
	return f.extClient.CatalogV1alpha1().PerconaVersions().Get(meta.Name, metav1.GetOptions{})
}

func (f *Framework) DeletePerconaVersion(meta metav1.ObjectMeta) error {
	return f.extClient.CatalogV1alpha1().PerconaVersions().Delete(meta.Name, &metav1.DeleteOptions{})
}
