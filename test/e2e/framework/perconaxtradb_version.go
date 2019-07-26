package framework

import (
	"fmt"

	. "github.com/onsi/gomega"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	api "kubedb.dev/apimachinery/apis/catalog/v1alpha1"
)

func (i *Invocation) PerconaXtraDBVersion() *api.PerconaXtraDBVersion {
	return &api.PerconaXtraDBVersion{
		ObjectMeta: metav1.ObjectMeta{
			Name: DBVersion,
			Labels: map[string]string{
				"app": i.app,
			},
		},
		Spec: api.PerconaXtraDBVersionSpec{
			Version: DBVersion,
			DB: api.PerconaXtraDBVersionDatabase{
				Image: fmt.Sprintf("%s/percona-xtradb-cluster:%s", DockerRegistry, DBVersion),
			},
			Proxysql: api.PerconaXtraDBVersionProxysql{
				Image: fmt.Sprintf("%s/proxysql-pxc:%s", DockerRegistry, DBVersion),
			},
			Exporter: api.PerconaXtraDBVersionExporter{
				Image: fmt.Sprintf("%s/mysqld-exporter:%s", DockerRegistry, ExporterTag),
			},
			InitContainer: api.PerconaXtraDBVersionInitContainer{
				Image: "kubedb/busybox",
			},
			//PodSecurityPolicies: api.PerconaXtraDBVersionPodSecurityPolicy{
			//	DatabasePolicyName: "perconaxtradb-db",
			//},
		},
	}
}

func (f *Framework) CreatePerconaXtraDBVersion(obj *api.PerconaXtraDBVersion) error {
	_, err := f.extClient.CatalogV1alpha1().PerconaXtraDBVersions().Create(obj)
	if err != nil && kerr.IsAlreadyExists(err) {
		e2 := f.extClient.CatalogV1alpha1().PerconaXtraDBVersions().Delete(obj.Name, &metav1.DeleteOptions{})
		Expect(e2).NotTo(HaveOccurred())
		_, e2 = f.extClient.CatalogV1alpha1().PerconaXtraDBVersions().Create(obj)
		return e2
	}
	return nil
}

func (f *Framework) GetPerconaXtraDBVersion(meta metav1.ObjectMeta) (*api.PerconaXtraDBVersion, error) {
	return f.extClient.CatalogV1alpha1().PerconaXtraDBVersions().Get(meta.Name, metav1.GetOptions{})
}

func (f *Framework) DeletePerconaXtraDBVersion(meta metav1.ObjectMeta) error {
	return f.extClient.CatalogV1alpha1().PerconaXtraDBVersions().Delete(meta.Name, &metav1.DeleteOptions{})
}
