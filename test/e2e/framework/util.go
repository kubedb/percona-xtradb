package framework

import (
	"fmt"
	"strings"
	"time"

	"github.com/appscode/go/types"
	core "k8s.io/api/core/v1"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	core_util "kmodules.xyz/client-go/core/v1"
	api "kubedb.dev/apimachinery/apis/kubedb/v1alpha1"
)

const (
	updateRetryInterval = 10 * 1000 * 1000 * time.Nanosecond
	maxAttempts         = 5
)

func deleteInBackground() *metav1.DeleteOptions {
	policy := metav1.DeletePropagationBackground
	return &metav1.DeleteOptions{PropagationPolicy: &policy}
}

func deleteInForeground() *metav1.DeleteOptions {
	policy := metav1.DeletePropagationForeground
	return &metav1.DeleteOptions{PropagationPolicy: &policy}
}

func (f *Invocation) GetCustomConfig(configs []string) *core.ConfigMap {
	configs = append([]string{"[mysqld]"}, configs...)
	return &core.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      f.app,
			Namespace: f.namespace,
		},
		Data: map[string]string{
			"my-custom.cnf": strings.Join(configs, "\n"),
		},
	}
}

func (f *Invocation) CreateConfigMap(obj *core.ConfigMap) error {
	_, err := f.kubeClient.CoreV1().ConfigMaps(obj.Namespace).Create(obj)
	return err
}

func (f *Framework) DeleteConfigMap(meta metav1.ObjectMeta) error {
	err := f.kubeClient.CoreV1().ConfigMaps(meta.Namespace).Delete(meta.Name, deleteInForeground())
	if !kerr.IsNotFound(err) {
		return err
	}
	return nil
}

func (f *Framework) CleanWorkloadLeftOvers() {
	// delete statefulset
	if err := f.kubeClient.AppsV1().StatefulSets(f.namespace).DeleteCollection(deleteInForeground(), metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{
			api.LabelDatabaseKind: api.ResourceKindPercona,
		}).String(),
	}); err != nil && !kerr.IsNotFound(err) {
		fmt.Printf("error in deletion of Statefulset. Error: %v", err)
	}

	// delete pvc
	if err := f.kubeClient.CoreV1().PersistentVolumeClaims(f.namespace).DeleteCollection(deleteInForeground(), metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{
			api.LabelDatabaseKind: api.ResourceKindPercona,
		}).String(),
	}); err != nil && !kerr.IsNotFound(err) {
		fmt.Printf("error in deletion of PVC. Error: %v", err)
	}

	// delete secret
	if err := f.kubeClient.CoreV1().Secrets(f.namespace).DeleteCollection(deleteInForeground(), metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{
			api.LabelDatabaseKind: api.ResourceKindPercona,
		}).String(),
	}); err != nil && !kerr.IsNotFound(err) {
		fmt.Printf("error in deletion of Secret. Error: %v", err)
	}
}

func (f *Framework) WaitUntilPodRunningBySelector(percona *api.Percona, proxysql bool) error {
	if !proxysql {
		return core_util.WaitUntilPodRunningBySelector(
			f.kubeClient,
			percona.Namespace,
			&metav1.LabelSelector{
				MatchLabels: percona.ClusterSelectors(),
			},
			int(types.Int32(percona.Spec.Replicas)),
		)
	} else {
		return core_util.WaitUntilPodRunningBySelector(
			f.kubeClient,
			percona.Namespace,
			&metav1.LabelSelector{
				MatchLabels: percona.ProxysqlSelectors(),
			},
			int(types.Int32(percona.Spec.PXC.Proxysql.Replicas)),
		)
	}

}
