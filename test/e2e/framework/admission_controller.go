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
package framework

import (
	"fmt"
	"strings"
	"time"

	"github.com/appscode/go/log"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	kApi "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1beta1"
	kutil "kmodules.xyz/client-go"
	admsn_kutil "kmodules.xyz/client-go/admissionregistration/v1beta1"
	meta_util "kmodules.xyz/client-go/meta"
)

func (f *Framework) isApiSvcReady(apiSvcName string) error {
	apiSvc, err := f.kaClient.ApiregistrationV1beta1().APIServices().Get(apiSvcName, metav1.GetOptions{})
	if err != nil {
		return err
	}
	for _, cond := range apiSvc.Status.Conditions {
		if cond.Type == kApi.Available && cond.Status == kApi.ConditionTrue {
			log.Infof("APIService %v status is true", apiSvcName)
			return nil
		}
	}
	log.Errorf("APIService %v not ready yet", apiSvcName)
	return fmt.Errorf("APIService %v not ready yet", apiSvcName)
}

func (f *Framework) restartOperator() {
	if pods, err := f.kubeClient.CoreV1().Pods("kube-system").List(metav1.ListOptions{
		LabelSelector: labels.Set{
			"app": "kubedb",
		}.String(),
	}); err != nil {
		Expect(err).NotTo(HaveOccurred())
	} else {
		for i := range pods.Items {
			if strings.HasPrefix(pods.Items[i].Name, "kubedb") {
				err = f.kubeClient.CoreV1().Pods("kube-system").Delete(pods.Items[i].Name, &metav1.DeleteOptions{})
				Expect(err).NotTo(HaveOccurred())

				break
			}
		}
	}
}

func (f *Framework) EnsureAPIServiceReady() GomegaAsyncAssertion {
	if f.isApiSvcReady("v1alpha1.mutators.kubedb.com") != nil ||
		f.isApiSvcReady("v1alpha1.validators.kubedb.com") != nil {
		f.restartOperator()
	}

	return f.EventuallyAPIServiceReady()
}

func (f *Framework) EventuallyAPIServiceReady() GomegaAsyncAssertion {
	return Eventually(
		func() error {
			if err := f.isApiSvcReady("v1alpha1.mutators.kubedb.com"); err != nil {
				return err
			}
			if err := f.isApiSvcReady("v1alpha1.validators.kubedb.com"); err != nil {
				return err
			}
			time.Sleep(time.Second * 5) // let the resource become available

			// Check if the annotations of validating webhook is updated by operator/controller
			apiSvc, err := f.kaClient.ApiregistrationV1beta1().APIServices().Get("v1alpha1.validators.kubedb.com", metav1.GetOptions{})
			if err != nil {
				return err
			}

			if _, err := meta_util.GetString(apiSvc.Annotations, admsn_kutil.KeyAdmissionWebhookActive); err == kutil.ErrNotFound {
				log.Errorf("APIService v1alpha1.validators.kubedb.com not ready yet")
				return err
			}
			return nil
		},
		time.Minute*10,
		time.Second*10,
	)
}
