package controller

import (
	"fmt"

	"github.com/appscode/go/encoding/json/types"
	"github.com/appscode/go/log"
	"github.com/pkg/errors"
	core "k8s.io/api/core/v1"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	clientsetscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/reference"
	kutil "kmodules.xyz/client-go"
	dynamic_util "kmodules.xyz/client-go/dynamic"
	meta_util "kmodules.xyz/client-go/meta"
	"kubedb.dev/apimachinery/apis"
	api "kubedb.dev/apimachinery/apis/kubedb/v1alpha1"
	"kubedb.dev/apimachinery/client/clientset/versioned/typed/kubedb/v1alpha1/util"
	"kubedb.dev/apimachinery/pkg/eventer"
	validator "kubedb.dev/percona-xtradbpkg/admission"
)

func (c *Controller) create(pxc *api.Percona) error {
	if err := validator.ValidatePercona(c.Client, c.ExtClient, pxc, true); err != nil {
		c.recorder.Event(
			pxc,
			core.EventTypeWarning,
			eventer.EventReasonInvalid,
			err.Error(),
		)
		log.Errorln(err)
		// stop Scheduler in case there is any.
		c.cronController.StopBackupScheduling(pxc.ObjectMeta)
		return nil
	}

	// Delete Matching DormantDatabase if exists any
	if err := c.deleteMatchingDormantDatabase(pxc); err != nil {
		return fmt.Errorf(`failed to delete dormant Database : "%v/%v". Reason: %v`, pxc.Namespace, pxc.Name, err)
	}

	if pxc.Status.Phase == "" {
		per, err := util.UpdatePerconaStatus(c.ExtClient.KubedbV1alpha1(), pxc, func(in *api.PerconaStatus) *api.PerconaStatus {
			in.Phase = api.DatabasePhaseCreating
			return in
		}, apis.EnableStatusSubresource)
		if err != nil {
			return err
		}
		pxc.Status = per.Status
	}



	// create Governing Service
	governingService, err := c.createPerconaGoverningService(pxc)
	if err != nil {
		return fmt.Errorf(`failed to create Service: "%v/%v". Reason: %v`, pxc.Namespace, governingService, err)
	}
	c.GoverningService = governingService

	if c.EnableRBAC {
		// Ensure ClusterRoles for statefulsets
		if err := c.ensureRBACStuff(pxc); err != nil {
			return err
		}
	}

	// ensure database Service
	vt1, err := c.ensureService(pxc)
	if err != nil {
		return err
	}

	if err := c.ensureDatabaseSecret(pxc); err != nil {
		return err
	}

	// ensure database StatefulSet
	vt2, err := c.ensurePerconaXtraDBNode(pxc)
	if err != nil {
		return err
	}

	if vt1 == kutil.VerbCreated && vt2 == kutil.VerbCreated {
		c.recorder.Event(
			pxc,
			core.EventTypeNormal,
			eventer.EventReasonSuccessful,
			"Successfully created Percona",
		)
	} else if vt1 == kutil.VerbPatched || vt2 == kutil.VerbPatched {
		c.recorder.Event(
			pxc,
			core.EventTypeNormal,
			eventer.EventReasonSuccessful,
			"Successfully patched Percona",
		)
	}

	if _, err := meta_util.GetString(pxc.Annotations, api.AnnotationInitialized); err == kutil.ErrNotFound &&
		pxc.Spec.Init != nil && pxc.Spec.Init.SnapshotSource != nil {

		snapshotSource := pxc.Spec.Init.SnapshotSource

		if pxc.Status.Phase == api.DatabasePhaseInitializing {
			return nil
		}
		jobName := fmt.Sprintf("%s-%s", api.DatabaseNamePrefix, snapshotSource.Name)
		if _, err := c.Client.BatchV1().Jobs(snapshotSource.Namespace).Get(jobName, metav1.GetOptions{}); err != nil {
			if !kerr.IsNotFound(err) {
				return err
			}
		} else {
			return nil
		}
		if err := c.initialize(pxc); err != nil {
			return fmt.Errorf("failed to complete initialization for %v/%v. Reason: %v", pxc.Namespace, pxc.Name, err)
		}
		return nil
	}

	per, err := util.UpdatePerconaStatus(c.ExtClient.KubedbV1alpha1(), pxc, func(in *api.PerconaStatus) *api.PerconaStatus {
		in.Phase = api.DatabasePhaseRunning
		in.ObservedGeneration = types.NewIntHash(pxc.Generation, meta_util.GenerationHash(pxc))
		return in
	}, apis.EnableStatusSubresource)
	if err != nil {
		return err
	}
	pxc.Status = per.Status

	// ensure StatsService for desired monitoring
	if _, err := c.ensureStatsService(pxc); err != nil {
		c.recorder.Eventf(
			pxc,
			core.EventTypeWarning,
			eventer.EventReasonFailedToCreate,
			"Failed to manage monitoring system. Reason: %v",
			err,
		)
		log.Errorln(err)
		return nil
	}

	if err := c.manageMonitor(pxc); err != nil {
		c.recorder.Eventf(
			pxc,
			core.EventTypeWarning,
			eventer.EventReasonFailedToCreate,
			"Failed to manage monitoring system. Reason: %v",
			err,
		)
		log.Errorln(err)
		return nil
	}

	_, err = c.ensureAppBinding(pxc)
	if err != nil {
		log.Errorln(err)
		return err
	}

	return nil
}

func (c *Controller) initialize(pxc *api.Percona) error {
	// TODO: integrate stash
	return nil
}

func (c *Controller) terminate(pxc *api.Percona) error {
	ref, rerr := reference.GetReference(clientsetscheme.Scheme, pxc)
	if rerr != nil {
		return rerr
	}

	// If TerminationPolicy is "pause", keep everything (ie, PVCs,Secrets,Snapshots) intact.
	// In operator, create dormantdatabase
	if pxc.Spec.TerminationPolicy == api.TerminationPolicyPause {
		if err := c.removeOwnerReferenceFromOffshoots(pxc, ref); err != nil {
			return err
		}

		if _, err := c.createDormantDatabase(pxc); err != nil {
			if kerr.IsAlreadyExists(err) {
				// if already exists, check if it is database of another Kind and return error in that case.
				// If the Kind is same, we can safely assume that the DormantDB was not deleted in before,
				// Probably because, User is more faster (create-delete-create-again-delete...) than operator!
				// So reuse that DormantDB!
				ddb, err := c.ExtClient.KubedbV1alpha1().DormantDatabases(pxc.Namespace).Get(pxc.Name, metav1.GetOptions{})
				if err != nil {
					return err
				}
				if val, _ := meta_util.GetStringValue(ddb.Labels, api.LabelDatabaseKind); val != api.ResourceKindPercona {
					return fmt.Errorf(`DormantDatabase "%v" of kind %v already exists`, pxc.Name, val)
				}
			} else {
				return fmt.Errorf(`failed to create DormantDatabase: "%v/%v". Reason: %v`, pxc.Namespace, pxc.Name, err)
			}
		}
	} else {
		// If TerminationPolicy is "wipeOut", delete everything (ie, PVCs,Secrets,Snapshots).
		// If TerminationPolicy is "delete", delete PVCs and keep snapshots,secrets intact.
		// In both these cases, don't create dormantdatabase
		if err := c.setOwnerReferenceToOffshoots(pxc, ref); err != nil {
			return err
		}
	}

	c.cronController.StopBackupScheduling(pxc.ObjectMeta)

	if pxc.Spec.Monitor != nil {
		if _, err := c.deleteMonitor(pxc); err != nil {
			log.Errorln(err)
			return nil
		}
	}
	return nil
}

func (c *Controller) setOwnerReferenceToOffshoots(pxc *api.Percona, ref *core.ObjectReference) error {
	selector := labels.SelectorFromSet(pxc.OffshootSelectors())

	// If TerminationPolicy is "wipeOut", delete snapshots and secrets,
	// else, keep it intact.
	if pxc.Spec.TerminationPolicy == api.TerminationPolicyWipeOut {
		if err := dynamic_util.EnsureOwnerReferenceForSelector(
			c.DynamicClient,
			api.SchemeGroupVersion.WithResource(api.ResourcePluralSnapshot),
			pxc.Namespace,
			selector,
			ref); err != nil {
			return err
		}
		if err := c.wipeOutDatabase(pxc.ObjectMeta, pxc.Spec.GetSecrets(), ref); err != nil {
			return errors.Wrap(err, "error in wiping out database.")
		}
	} else {
		// Make sure snapshot and secret's ownerreference is removed.
		if err := dynamic_util.RemoveOwnerReferenceForSelector(
			c.DynamicClient,
			api.SchemeGroupVersion.WithResource(api.ResourcePluralSnapshot),
			pxc.Namespace,
			selector,
			ref); err != nil {
			return err
		}
		if err := dynamic_util.RemoveOwnerReferenceForItems(
			c.DynamicClient,
			core.SchemeGroupVersion.WithResource("secrets"),
			pxc.Namespace,
			pxc.Spec.GetSecrets(),
			ref); err != nil {
			return err
		}
	}
	// delete PVC for both "wipeOut" and "delete" TerminationPolicy.
	return dynamic_util.EnsureOwnerReferenceForSelector(
		c.DynamicClient,
		core.SchemeGroupVersion.WithResource("persistentvolumeclaims"),
		pxc.Namespace,
		selector,
		ref)
}

func (c *Controller) removeOwnerReferenceFromOffshoots(pxc *api.Percona, ref *core.ObjectReference) error {
	// First, Get LabelSelector for Other Components
	labelSelector := labels.SelectorFromSet(pxc.OffshootSelectors())

	if err := dynamic_util.RemoveOwnerReferenceForSelector(
		c.DynamicClient,
		api.SchemeGroupVersion.WithResource(api.ResourcePluralSnapshot),
		pxc.Namespace,
		labelSelector,
		ref); err != nil {
		return err
	}
	if err := dynamic_util.RemoveOwnerReferenceForSelector(
		c.DynamicClient,
		core.SchemeGroupVersion.WithResource("persistentvolumeclaims"),
		pxc.Namespace,
		labelSelector,
		ref); err != nil {
		return err
	}
	if err := dynamic_util.RemoveOwnerReferenceForItems(
		c.DynamicClient,
		core.SchemeGroupVersion.WithResource("secrets"),
		pxc.Namespace,
		pxc.Spec.GetSecrets(),
		ref); err != nil {
		return err
	}
	return nil
}
