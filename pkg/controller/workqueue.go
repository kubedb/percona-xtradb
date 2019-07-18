package controller

import (
	"github.com/appscode/go/log"
	core_util "kmodules.xyz/client-go/core/v1"
	"kmodules.xyz/client-go/tools/queue"
	"kubedb.dev/apimachinery/apis"
	api "kubedb.dev/apimachinery/apis/kubedb/v1alpha1"
	"kubedb.dev/apimachinery/client/clientset/versioned/typed/kubedb/v1alpha1/util"
)

func (c *Controller) initWatcher() {
	c.pxcInformer = c.KubedbInformerFactory.Kubedb().V1alpha1().Perconas().Informer()
	c.pxcQueue = queue.New("Percona", c.MaxNumRequeues, c.NumThreads, c.runPercona)
	c.pxcLister = c.KubedbInformerFactory.Kubedb().V1alpha1().Perconas().Lister()
	c.pxcInformer.AddEventHandler(queue.NewObservableUpdateHandler(c.pxcQueue.GetQueue(), apis.EnableStatusSubresource))
}

func (c *Controller) runPercona(key string) error {
	log.Debugln("started processing, key:", key)
	obj, exists, err := c.pxcInformer.GetIndexer().GetByKey(key)
	if err != nil {
		log.Errorf("Fetching object with key %s from store failed with %v", key, err)
		return err
	}

	if !exists {
		log.Debugf("Percona %s does not exist anymore", key)
	} else {
		// Note that you also have to check the uid if you have a local controlled resource, which
		// is dependent on the actual instance, to detect that a Percona was recreated with the same name
		pxc := obj.(*api.Percona).DeepCopy()
		if pxc.DeletionTimestamp != nil {
			if core_util.HasFinalizer(pxc.ObjectMeta, api.GenericKey) {
				if err := c.terminate(pxc); err != nil {
					log.Errorln(err)
					return err
				}
				pxc, _, err = util.PatchPercona(c.ExtClient.KubedbV1alpha1(), pxc, func(in *api.Percona) *api.Percona {
					in.ObjectMeta = core_util.RemoveFinalizer(in.ObjectMeta, api.GenericKey)
					return in
				})
				return err
			}
		} else {
			pxc, _, err = util.PatchPercona(c.ExtClient.KubedbV1alpha1(), pxc, func(in *api.Percona) *api.Percona {
				in.ObjectMeta = core_util.AddFinalizer(in.ObjectMeta, api.GenericKey)
				return in
			})
			if err != nil {
				return err
			}
			if err := c.create(pxc); err != nil {
				log.Errorln(err)
				c.pushFailureEvent(pxc, err.Error())
				return err
			}
		}
	}
	return nil
}
