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

package framework

import (
	"context"
	"fmt"

	api "kubedb.dev/apimachinery/apis/kubedb/v1alpha1"

	"github.com/appscode/go/types"
	"github.com/appscode/go/wait"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	kutil "kmodules.xyz/client-go"
	"kmodules.xyz/client-go/discovery"
	meta_util "kmodules.xyz/client-go/meta"
	appcat "kmodules.xyz/custom-resources/apis/appcatalog/v1alpha1"
	ofst "kmodules.xyz/offshoot-api/api/v1"
	"stash.appscode.dev/apimachinery/apis"
	"stash.appscode.dev/apimachinery/apis/stash"
	stashV1alpha1 "stash.appscode.dev/apimachinery/apis/stash/v1alpha1"
	stashv1beta1 "stash.appscode.dev/apimachinery/apis/stash/v1beta1"
)

var (
	StashPerconaXtraDBBackupTask  = "percona-xtradb-backup-5.7"
	StashPerconaXtraDBRestoreTask = "percona-xtradb-restore-5.7"
)

func (f *Framework) FoundStashCRDs() bool {
	return discovery.ExistsGroupKind(f.kubeClient.Discovery(), stash.GroupName, stashv1beta1.ResourceKindRestoreSession)
}

func (f *Invocation) BackupConfiguration(meta metav1.ObjectMeta) *stashv1beta1.BackupConfiguration {
	return &stashv1beta1.BackupConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      meta.Name,
			Namespace: f.namespace,
		},
		Spec: stashv1beta1.BackupConfigurationSpec{
			Repository: corev1.LocalObjectReference{
				Name: meta.Name,
			},
			Schedule: "*/3 * * * *",
			RetentionPolicy: stashV1alpha1.RetentionPolicy{
				KeepLast: 5,
				Prune:    true,
			},
			BackupConfigurationTemplateSpec: stashv1beta1.BackupConfigurationTemplateSpec{
				Task: stashv1beta1.TaskRef{
					Name: StashPerconaXtraDBBackupTask,
				},
				Target: &stashv1beta1.BackupTarget{
					Ref: stashv1beta1.TargetRef{
						APIVersion: appcat.SchemeGroupVersion.String(),
						Kind:       appcat.ResourceKindApp,
						Name:       meta.Name,
					},
				},
			},
		},
	}
}

func (f *Framework) CreateBackupConfiguration(backupCfg *stashv1beta1.BackupConfiguration) error {
	_, err := f.stashClient.StashV1beta1().BackupConfigurations(backupCfg.Namespace).Create(context.TODO(), backupCfg, metav1.CreateOptions{})
	return err
}

func (f *Framework) DeleteBackupConfiguration(meta metav1.ObjectMeta) error {
	return f.stashClient.StashV1beta1().BackupConfigurations(meta.Namespace).Delete(context.TODO(), meta.Name, metav1.DeleteOptions{})
}

func (f *Framework) WaitUntilBackkupSessionBeCreated(bcMeta metav1.ObjectMeta) (bs *stashv1beta1.BackupSession, err error) {
	err = wait.PollImmediate(kutil.RetryInterval, kutil.ReadinessTimeout, func() (bool, error) {
		bsList, err := f.stashClient.StashV1beta1().BackupSessions(bcMeta.Namespace).List(context.TODO(), metav1.ListOptions{
			LabelSelector: labels.Set{
				apis.LabelInvokerType: "BackupConfiguration",
				apis.LabelInvokerName: bcMeta.Name,
			}.String(),
		})
		if err != nil {
			return false, err
		}
		if len(bsList.Items) == 0 {
			return false, nil
		}

		bs = &bsList.Items[0]

		return true, nil
	})

	return
}

func (f *Framework) EventuallyBackupSessionPhase(meta metav1.ObjectMeta) GomegaAsyncAssertion {
	return Eventually(
		func() (phase stashv1beta1.BackupSessionPhase) {
			bs, err := f.stashClient.StashV1beta1().BackupSessions(meta.Namespace).Get(context.TODO(), meta.Name, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			return bs.Status.Phase
		},
	)
}

func (f *Invocation) Repository(meta metav1.ObjectMeta, secretName string) *stashV1alpha1.Repository {
	return &stashV1alpha1.Repository{
		ObjectMeta: metav1.ObjectMeta{
			Name:      meta.Name,
			Namespace: f.namespace,
		},
	}
}

func (f *Framework) CreateRepository(repo *stashV1alpha1.Repository) error {
	_, err := f.stashClient.StashV1alpha1().Repositories(repo.Namespace).Create(context.TODO(), repo, metav1.CreateOptions{})
	return err
}

func (f *Framework) DeleteRepository(meta metav1.ObjectMeta) error {
	err := f.stashClient.StashV1alpha1().Repositories(meta.Namespace).Delete(context.TODO(), meta.Name, meta_util.DeleteInBackground())
	return err
}

func (f *Invocation) RestoreSessionForCluster(meta, oldMeta metav1.ObjectMeta, replicas *int32) *stashv1beta1.RestoreSession {
	return &stashv1beta1.RestoreSession{
		ObjectMeta: metav1.ObjectMeta{
			Name:      meta.Name,
			Namespace: f.namespace,
			Labels: map[string]string{
				"app":                 f.app,
				api.LabelDatabaseKind: api.ResourceKindPerconaXtraDB,
			},
		},
		Spec: stashv1beta1.RestoreSessionSpec{
			Repository: corev1.LocalObjectReference{
				Name: oldMeta.Name,
			},
			RestoreTargetSpec: stashv1beta1.RestoreTargetSpec{
				Task: stashv1beta1.TaskRef{
					Name: StashPerconaXtraDBRestoreTask,
				},
				Target: &stashv1beta1.RestoreTarget{
					Replicas: replicas,
					Rules: []stashv1beta1.Rule{
						{
							Snapshots:   []string{"latest"},
							TargetHosts: []string{},
							SourceHost:  "host-0",
						},
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      fmt.Sprintf("data-%s", meta.Name),
							MountPath: "/var/lib/mysql",
						},
					},
					VolumeClaimTemplates: []ofst.PersistentVolumeClaim{
						{
							PartialObjectMeta: ofst.PartialObjectMeta{
								Name: fmt.Sprintf("data-%s-${POD_ORDINAL}", meta.Name),
								Annotations: map[string]string{
									"volume.beta.kubernetes.io/storage-class": "standard",
								},
							},
							Spec: corev1.PersistentVolumeClaimSpec{
								AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
								StorageClassName: types.StringP("standard"),
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceStorage: resource.MustParse(DBPvcStorageSize),
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func (f *Invocation) RestoreSessionForStandalone(meta, oldMeta metav1.ObjectMeta) *stashv1beta1.RestoreSession {
	return &stashv1beta1.RestoreSession{
		ObjectMeta: metav1.ObjectMeta{
			Name:      meta.Name,
			Namespace: f.namespace,
			Labels: map[string]string{
				"app":                 f.app,
				api.LabelDatabaseKind: api.ResourceKindPerconaXtraDB,
			},
		},
		Spec: stashv1beta1.RestoreSessionSpec{
			Repository: corev1.LocalObjectReference{
				Name: oldMeta.Name,
			},
			RestoreTargetSpec: stashv1beta1.RestoreTargetSpec{
				Task: stashv1beta1.TaskRef{
					Name: StashPerconaXtraDBRestoreTask,
				},
				Target: &stashv1beta1.RestoreTarget{
					Ref: stashv1beta1.TargetRef{
						APIVersion: appcat.SchemeGroupVersion.String(),
						Kind:       appcat.ResourceKindApp,
						Name:       meta.Name,
					},
					Rules: []stashv1beta1.Rule{
						{
							Snapshots: []string{"latest"},
						},
					},
				},
			},
		},
	}
}

func (f *Framework) CreateRestoreSession(restoreSession *stashv1beta1.RestoreSession) error {
	_, err := f.stashClient.StashV1beta1().RestoreSessions(restoreSession.Namespace).Create(context.TODO(), restoreSession, metav1.CreateOptions{})
	return err
}

func (f *Framework) DeleteRestoreSession(meta metav1.ObjectMeta) error {
	err := f.stashClient.StashV1beta1().RestoreSessions(meta.Namespace).Delete(context.TODO(), meta.Name, metav1.DeleteOptions{})
	return err
}

func (f *Framework) EventuallyRestoreSessionPhase(meta metav1.ObjectMeta) GomegaAsyncAssertion {
	return Eventually(func() stashv1beta1.RestorePhase {
		restoreSession, err := f.stashClient.StashV1beta1().RestoreSessions(meta.Namespace).Get(context.TODO(), meta.Name, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		return restoreSession.Status.Phase
	})
}
