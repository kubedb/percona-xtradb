package controller

import (
	api "github.com/kubedb/apimachinery/apis/kubedb/v1alpha1"
	cs_util "github.com/kubedb/apimachinery/client/clientset/versioned/typed/kubedb/v1alpha1/util"
	"github.com/pkg/errors"
	core "k8s.io/api/core/v1"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/sets"
	clientsetscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/reference"
	core_util "kmodules.xyz/client-go/core/v1"
	dynamic_util "kmodules.xyz/client-go/dynamic"
	meta_util "kmodules.xyz/client-go/meta"
)

// WaitUntilPaused is an Interface of *amc.Controller
func (c *Controller) WaitUntilPaused(drmn *api.DormantDatabase) error {
	db := &api.Percona{
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

func (c *Controller) waitUntilRBACStuffDeleted(pxc *api.Percona) error {
	// Delete ServiceAccount
	if err := core_util.WaitUntillServiceAccountDeleted(c.Client, pxc.ObjectMeta); err != nil {
		return err
	}

	//// Delete Snapshot ServiceAccount
	//snapSAMeta := metav1.ObjectMeta{
	//	Name:      pxc.SnapshotSAName(),
	//	Namespace: pxc.Namespace,
	//}
	//if err := core_util.WaitUntillServiceAccountDeleted(c.Client, snapSAMeta); err != nil {
	//	return err
	//}

	return nil
}

// WipeOutDatabase is an Interface of *amc.Controller.
// It verifies and deletes secrets and other left overs of DBs except Snapshot and PVC.
func (c *Controller) WipeOutDatabase(drmn *api.DormantDatabase) error {
	ref, rerr := reference.GetReference(clientsetscheme.Scheme, drmn)
	if rerr != nil {
		return rerr
	}
	if err := c.wipeOutDatabase(drmn.ObjectMeta, drmn.GetDatabaseSecrets(), ref); err != nil {
		return errors.Wrap(err, "error in wiping out database.")
	}
	return nil
}

// wipeOutDatabase is a generic function to call from WipeOutDatabase and percona pause method.
func (c *Controller) wipeOutDatabase(meta metav1.ObjectMeta, secrets []string, ref *core.ObjectReference) error {
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
		ref)
}

func (c *Controller) deleteMatchingDormantDatabase(pxc *api.Percona) error {
	// Check if DormantDatabase exists or not
	ddb, err := c.ExtClient.KubedbV1alpha1().DormantDatabases(pxc.Namespace).Get(pxc.Name, metav1.GetOptions{})
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
	if err := c.ExtClient.KubedbV1alpha1().DormantDatabases(pxc.Namespace).Delete(pxc.Name,
		meta_util.DeleteInBackground()); err != nil && !kerr.IsNotFound(err) {
		return err
	}

	return nil
}

func (c *Controller) createDormantDatabase(pxc *api.Percona) (*api.DormantDatabase, error) {
	dormantDb := &api.DormantDatabase{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pxc.Name,
			Namespace: pxc.Namespace,
			Labels: map[string]string{
				api.LabelDatabaseKind: api.ResourceKindPercona,
			},
		},
		Spec: api.DormantDatabaseSpec{
			Origin: api.Origin{
				ObjectMeta: metav1.ObjectMeta{
					Name:              pxc.Name,
					Namespace:         pxc.Namespace,
					Labels:            pxc.Labels,
					Annotations:       pxc.Annotations,
					CreationTimestamp: pxc.CreationTimestamp,
				},
				Spec: api.OriginSpec{
					Percona: &pxc.Spec,
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

	dbList, err := c.pxcLister.Perconas(meta.Namespace).List(labels.Everything())
	if err != nil {
		return nil, err
	}
	for _, pxc := range dbList {
		if pxc.Name != meta.Name {
			secretUsed.Insert(pxc.Spec.GetSecrets()...)
		}
	}
	labelMap := map[string]string{
		api.LabelDatabaseKind: api.ResourceKindPercona,
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
