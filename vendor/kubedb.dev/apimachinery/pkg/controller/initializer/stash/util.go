/*
Copyright AppsCode Inc. and Contributors

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

package stash

import (
	"context"
	"fmt"
	"strings"
	"time"

	"kubedb.dev/apimachinery/apis/kubedb"
	api "kubedb.dev/apimachinery/apis/kubedb/v1alpha2"
	"kubedb.dev/apimachinery/client/clientset/versioned/scheme"

	"github.com/appscode/go/log"
	"github.com/appscode/go/types"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/reference"
	kmapi "kmodules.xyz/client-go/api/v1"
	core_util "kmodules.xyz/client-go/core/v1"
	"kmodules.xyz/client-go/discovery"
	dmcond "kmodules.xyz/client-go/dynamic/conditions"
	appcat "kmodules.xyz/custom-resources/apis/appcatalog"
	ab "kmodules.xyz/custom-resources/apis/appcatalog/v1alpha1"
	sapis "stash.appscode.dev/apimachinery/apis"
	"stash.appscode.dev/apimachinery/apis/stash"
	"stash.appscode.dev/apimachinery/apis/stash/v1beta1"
)

func (c *Controller) extractRestoreInfo(invoker interface{}) (*restoreInfo, error) {
	ri := &restoreInfo{
		invoker: core.TypedLocalObjectReference{
			APIGroup: types.StringP(stash.GroupName),
		},
		do: dmcond.DynamicOptions{
			Client: c.DynamicClient,
		},
	}
	var err error
	switch invoker := invoker.(type) {
	case *v1beta1.RestoreSession:
		// invoker information
		ri.invoker.Kind = invoker.Kind
		ri.invoker.Name = invoker.Name
		// target information
		ri.target = invoker.Spec.Target
		// restore status
		ri.phase = invoker.Status.Phase
		// database information
		ri.do.Namespace = invoker.Namespace
		ri.do.Kind = invoker.Labels[api.LabelDatabaseKind]
	case *v1beta1.RestoreBatch:
		// invoker information
		ri.invoker.Kind = invoker.Kind
		ri.invoker.Name = invoker.Name
		// target information
		// RestoreBatch can have multiple targets. In this case, only the database related target'c phase does matter.
		ri.target, err = c.identifyTarget(invoker.Spec.Members, ri.do.Namespace)
		if err != nil {
			return ri, err
		}
		// restore status
		// RestoreBatch can have multiple targets. In this case, finding the appropriate target is necessary.
		ri.phase = getTargetPhase(invoker.Status, ri.target)
		// database information
		ri.do.Namespace = invoker.Namespace
		ri.do.Kind = invoker.Labels[api.LabelDatabaseKind]
	default:
		return ri, fmt.Errorf("unknown restore invoker type")
	}
	// Now, extract the respective database group,version,resource
	err = c.extractDatabaseInfo(ri)
	if err != nil {
		return nil, err
	}
	return ri, nil
}

func (c *Controller) handleRestoreInvokerEvent(ri *restoreInfo) error {
	if ri == nil {
		return fmt.Errorf("invalid restore information. it must not be nil")
	}

	// Restore process has started, add "DataRestoreStarted" condition to the respective database CR
	err := ri.do.SetCondition(kmapi.Condition{
		Type:    api.DatabaseDataRestoreStarted,
		Status:  kmapi.ConditionTrue,
		Reason:  api.DataRestoreStartedByExternalInitializer,
		Message: fmt.Sprintf("Data restore started by initializer: %s/%s/%s.", *ri.invoker.APIGroup, ri.invoker.Kind, ri.invoker.Name),
	})
	if err != nil {
		return err
	}

	// Just log and return if the restore process hasn't completed yet.
	if ri.phase != v1beta1.RestoreSucceeded && ri.phase != v1beta1.RestoreFailed && ri.phase != v1beta1.RestorePhaseUnknown {
		log.Infof("restore process hasn't completed yet. Current restore phase: %s", ri.phase)
		return nil
	}

	// If the target could not be identified properly, we can't process further.
	if ri.target == nil {
		return fmt.Errorf("couldn't identify the restore target from invoker: %s/%s/%s", *ri.invoker.APIGroup, ri.invoker.Kind, ri.invoker.Name)
	}

	dbCond := kmapi.Condition{
		Type: api.DatabaseDataRestored,
	}

	if ri.phase == v1beta1.RestoreSucceeded {
		dbCond.Status = kmapi.ConditionTrue
		dbCond.Reason = api.DatabaseSuccessfullyRestored
		dbCond.Message = fmt.Sprintf("Successfully restored data by initializer %s %s/%s",
			ri.invoker.Kind,
			ri.do.Namespace,
			ri.invoker.Name,
		)
	} else {
		dbCond.Status = kmapi.ConditionFalse
		dbCond.Reason = api.FailedToRestoreData
		dbCond.Message = fmt.Sprintf("Failed to restore data by initializer %s %s/%s."+
			"\nRun 'kubectl describe %s %s -n %s' for more details.",
			ri.invoker.Kind,
			ri.do.Namespace,
			ri.invoker.Name,
			strings.ToLower(ri.invoker.Kind),
			ri.invoker.Name,
			ri.do.Namespace,
		)
	}

	// Add "DatabaseInitialized" dmcond to the respective database CR
	err = ri.do.SetCondition(dbCond)
	if err != nil {
		return err
	}
	// Write data restore completion event to the respective database CR
	return c.writeRestoreCompletionEvent(ri.do, dbCond)
}

func (c *Controller) identifyTarget(members []v1beta1.RestoreTargetSpec, namespace string) (*v1beta1.RestoreTarget, error) {
	// check if there is any AppBinding as target. if there any, this is the desired target.
	for i, m := range members {
		if m.Target != nil {
			ok, err := targetOfGroupKind(m.Target.Ref, appcat.GroupName, ab.ResourceKindApp)
			if err != nil {
				return nil, err
			}
			if ok {
				return members[i].Target, nil
			}
		}
	}
	// no AppBinding has found as target. the target might be resulting workload (i.e. StatefulSet or Deployment(for memcached)).
	// we should check the workload's owner reference to be sure.
	for i, m := range members {
		if m.Target != nil {
			ok, err := targetOfGroupKind(m.Target.Ref, apps.GroupName, sapis.KindStatefulSet)
			if err != nil {
				return nil, err
			}
			if ok {
				sts, err := c.Client.AppsV1().StatefulSets(namespace).Get(context.Background(), m.Target.Ref.Name, metav1.GetOptions{})
				if err != nil {
					return nil, err
				}
				// if the controller owner is a KubeDB resource, then this StatefulSet must be the desired target
				ok, _, err := core_util.IsOwnerOfGroup(metav1.GetControllerOf(sts), kubedb.GroupName)
				if err != nil {
					return nil, err
				}
				if ok {
					return members[i].Target, nil
				}
			}
		}
	}
	return nil, nil
}

func getTargetPhase(status v1beta1.RestoreBatchStatus, target *v1beta1.RestoreTarget) v1beta1.RestorePhase {
	if target != nil {
		for _, m := range status.Members {
			if sapis.TargetMatched(m.Ref, target.Ref) {
				return v1beta1.RestorePhase(m.Phase)
			}
		}
	}
	return status.Phase
}

// waitUntilStashInstalled waits for Controller to be installed. It check whether Controller has been installed or not by querying RestoreSession crd.
// It either waits until RestoreSession crd exists or throws error otherwise
func (c *Controller) waitUntilStashInstalled(stopCh <-chan struct{}) error {
	log.Infoln("Looking for the Stash operator.......")
	return wait.PollImmediateUntil(time.Second*10, func() (bool, error) {
		return discovery.ExistsGroupKind(c.Client.Discovery(), stash.GroupName, v1beta1.ResourceKindRestoreSession) ||
			discovery.ExistsGroupKind(c.Client.Discovery(), stash.GroupName, v1beta1.ResourceKindRestoreBatch), nil
	}, stopCh)
}

func (c *Controller) extractDatabaseInfo(ri *restoreInfo) error {
	if ri == nil {
		return fmt.Errorf("invalid restoreInfo. It must not be nil")
	}
	if ri.target == nil {
		return fmt.Errorf("invalid target. It must not be nil")
	}
	var owner *metav1.OwnerReference
	if matched, err := targetOfGroupKind(ri.target.Ref, appcat.GroupName, ab.ResourceKindApp); err == nil && matched {
		appBinding, err := c.AppCatalogClient.AppcatalogV1alpha1().AppBindings(ri.do.Namespace).Get(context.TODO(), ri.target.Ref.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		owner = metav1.GetControllerOf(appBinding)
	} else if matched, err := targetOfGroupKind(ri.target.Ref, apps.GroupName, sapis.KindStatefulSet); err == nil && matched {
		sts, err := c.AppCatalogClient.AppcatalogV1alpha1().AppBindings(ri.do.Namespace).Get(context.TODO(), ri.target.Ref.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		owner = metav1.GetControllerOf(sts)
	}
	if owner == nil {
		return fmt.Errorf("failed to extract database information from the target info. Reason: target does not have controlling owner")
	}
	gv, err := schema.ParseGroupVersion(owner.APIVersion)
	if err != nil {
		return err
	}
	ri.do.Name = owner.Name
	ri.do.GVR = schema.GroupVersionResource{
		Group:   gv.Group,
		Version: gv.Version,
	}
	switch owner.Kind {
	case api.ResourceKindElasticsearch:
		ri.do.GVR.Resource = api.ResourcePluralElasticsearch
	case api.ResourceKindMongoDB:
		ri.do.GVR.Resource = api.ResourcePluralMongoDB
	case api.ResourceKindMySQL:
		ri.do.GVR.Resource = api.ResourcePluralMySQL
	case api.ResourceKindPerconaXtraDB:
		ri.do.GVR.Resource = api.ResourcePluralPerconaXtraDB
	case api.ResourceKindPostgres:
		ri.do.GVR.Resource = api.ResourcePluralPostgres
	case api.ResourceKindRedis:
		ri.do.GVR.Resource = api.ResourcePluralRedis
	}
	return nil
}

func targetOfGroupKind(target v1beta1.TargetRef, group, kind string) (bool, error) {
	gv, err := schema.ParseGroupVersion(target.APIVersion)
	if err != nil {
		return false, err
	}
	return gv.Group == group && target.Kind == kind, nil
}

func (c *Controller) writeRestoreCompletionEvent(do dmcond.DynamicOptions, cond kmapi.Condition) error {
	// Get the database CR
	resp, err := do.Client.Resource(do.GVR).Namespace(do.Namespace).Get(context.TODO(), do.Name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	// Create database CR's reference
	ref, err := reference.GetReference(scheme.Scheme, resp)
	if err != nil {
		return err
	}

	eventType := core.EventTypeNormal
	if cond.Status != kmapi.ConditionTrue {
		eventType = core.EventTypeWarning
	}
	// create event
	c.Recorder.Eventf(ref, eventType, cond.Reason, cond.Message)
	return nil
}
