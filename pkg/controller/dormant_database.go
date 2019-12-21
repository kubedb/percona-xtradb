/*
Copyright The KubeDB Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package controller

import (
	api "kubedb.dev/apimachinery/apis/kubedb/v1alpha1"
	cs_util "kubedb.dev/apimachinery/client/clientset/versioned/typed/kubedb/v1alpha1/util"

	"github.com/pkg/errors"
	core "k8s.io/api/core/v1"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/sets"
	core_util "kmodules.xyz/client-go/core/v1"
	dynamic_util "kmodules.xyz/client-go/dynamic"
	meta_util "kmodules.xyz/client-go/meta"
	ofst "kmodules.xyz/offshoot-api/api/v1"
)

// WaitUntilPaused is an Interface of *amc.Controller
func (c *Controller) WaitUntilPaused(drmn *api.DormantDatabase) error {
	db := &api.PerconaXtraDB{
		ObjectMeta: metav1.ObjectMeta{
			Name:      drmn.OffshootName(),
			Namespace: drmn.Namespace,
		},
	}

	if err := core_util.WaitUntilPodDeletedBySelector(c.Client, db.Namespace, metav1.SetAsLabelSelector(db.OffshootSelectors())); err != nil {
		return err
	}

	if err := core_util.WaitUntilServiceDeletedBySelector(c.Client, db.Namespace, metav1.SetAsLabelSelector(db.OffshootSelectors())); err != nil {
		return err
	}

	if err := c.waitUntilRBACStuffDeleted(db); err != nil {
		return err
	}

	return nil
}

func (c *Controller) waitUntilRBACStuffDeleted(px *api.PerconaXtraDB) error {
	// Delete ServiceAccount
	if err := core_util.WaitUntillServiceAccountDeleted(c.Client, px.ObjectMeta); err != nil {
		return err
	}

	return nil
}

// WipeOutDatabase is an Interface of *amc.Controller.
// It verifies and deletes secrets and other left overs of DBs except Snapshot and PVC.
func (c *Controller) WipeOutDatabase(drmn *api.DormantDatabase) error {
	owner := metav1.NewControllerRef(drmn, api.SchemeGroupVersion.WithKind(api.ResourceKindDormantDatabase))
	if err := c.wipeOutDatabase(drmn.ObjectMeta, drmn.GetDatabaseSecrets(), owner); err != nil {
		return errors.Wrap(err, "error in wiping out database.")
	}
	return nil
}

// wipeOutDatabase is a generic function to call from WipeOutDatabase and percona-xtradb pause method.
func (c *Controller) wipeOutDatabase(meta metav1.ObjectMeta, secrets []string, owner *metav1.OwnerReference) error {
	secretUsed, err := c.secretsUsedByPeers(meta)
	if err != nil {
		return errors.Wrap(err, "error in getting used secret list")
	}
	unusedSecrets := sets.NewString(secrets...).Difference(secretUsed)
	return dynamic_util.EnsureOwnerReferenceForItems(
		c.DynamicClient,
		core.SchemeGroupVersion.WithResource("secrets"),
		meta.Namespace,
		unusedSecrets.List(),
		owner)
}

func (c *Controller) deleteMatchingDormantDatabase(px *api.PerconaXtraDB) error {
	// Check if DormantDatabase exists or not
	ddb, err := c.ExtClient.KubedbV1alpha1().DormantDatabases(px.Namespace).Get(px.Name, metav1.GetOptions{})
	if err != nil {
		if !kerr.IsNotFound(err) {
			return err
		}
		return nil
	}

	// Set WipeOut to false
	if _, _, err := cs_util.PatchDormantDatabase(c.ExtClient.KubedbV1alpha1(), ddb, func(in *api.DormantDatabase) *api.DormantDatabase {
		in.Spec.WipeOut = false
		return in
	}); err != nil {
		return err
	}

	// Delete  Matching dormantDatabase
	if err := c.ExtClient.KubedbV1alpha1().DormantDatabases(px.Namespace).Delete(px.Name,
		meta_util.DeleteInBackground()); err != nil && !kerr.IsNotFound(err) {
		return err
	}

	return nil
}

func (c *Controller) createDormantDatabase(px *api.PerconaXtraDB) (*api.DormantDatabase, error) {
	dormantDb := &api.DormantDatabase{
		ObjectMeta: metav1.ObjectMeta{
			Name:      px.Name,
			Namespace: px.Namespace,
			Labels: map[string]string{
				api.LabelDatabaseKind: api.ResourceKindPerconaXtraDB,
			},
		},
		Spec: api.DormantDatabaseSpec{
			Origin: api.Origin{
				PartialObjectMeta: ofst.PartialObjectMeta{
					Name:              px.Name,
					Namespace:         px.Namespace,
					Labels:            px.Labels,
					Annotations:       px.Annotations,
					CreationTimestamp: px.CreationTimestamp,
				},
				Spec: api.OriginSpec{
					PerconaXtraDB: &px.Spec,
				},
			},
		},
	}

	return c.ExtClient.KubedbV1alpha1().DormantDatabases(dormantDb.Namespace).Create(dormantDb)
}

// isSecretUsed gets the DBList of same kind, then checks if our required secret is used by those.
// Similarly, isSecretUsed also checks for DomantDB of similar dbKind label.
func (c *Controller) secretsUsedByPeers(meta metav1.ObjectMeta) (sets.String, error) {
	secretUsed := sets.NewString()

	dbList, err := c.pxLister.PerconaXtraDBs(meta.Namespace).List(labels.Everything())
	if err != nil {
		return nil, err
	}
	for _, px := range dbList {
		if px.Name != meta.Name {
			secretUsed.Insert(px.Spec.GetSecrets()...)
		}
	}
	labelMap := map[string]string{
		api.LabelDatabaseKind: api.ResourceKindPerconaXtraDB,
	}
	drmnList, err := c.ExtClient.KubedbV1alpha1().DormantDatabases(meta.Namespace).List(
		metav1.ListOptions{
			LabelSelector: labels.SelectorFromSet(labelMap).String(),
		},
	)
	if err != nil {
		return nil, err
	}
	for _, ddb := range drmnList.Items {
		if ddb.Name != meta.Name {
			secretUsed.Insert(ddb.GetDatabaseSecrets()...)
		}
	}

	return secretUsed, nil
}
