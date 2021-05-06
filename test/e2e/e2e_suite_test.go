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

package e2e_test

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"

	cs "kubedb.dev/apimachinery/client/clientset/versioned"
	"kubedb.dev/apimachinery/client/clientset/versioned/scheme"
	"kubedb.dev/percona-xtradb/test/e2e/framework"

	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/reporters"
	. "github.com/onsi/gomega"
	"gomodules.xyz/kglog"
	kext_cs "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1beta1"
	"k8s.io/client-go/kubernetes"
	clientSetScheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/util/homedir"
	"k8s.io/klog/v2"
	ka "k8s.io/kube-aggregator/pkg/client/clientset_generated/clientset"
	"kmodules.xyz/client-go/tools/clientcmd"
	appcat_cs "kmodules.xyz/custom-resources/client/clientset/versioned/typed/appcatalog/v1alpha1"
	scs "stash.appscode.dev/apimachinery/client/clientset/versioned"
)

var (
	storageClass   = "standard"
	kubeconfigPath = func() string {
		kubecfg := os.Getenv("KUBECONFIG")
		if kubecfg != "" {
			return kubecfg
		}
		return filepath.Join(homedir.HomeDir(), ".kube", "config")
	}()
	kubeContext = ""
)

func init() {
	if err := scheme.AddToScheme(clientSetScheme.Scheme); err != nil {
		klog.Println(err)
	}

	flag.StringVar(&kubeconfigPath, "kubeconfig", kubeconfigPath, "Path to kubeconfig file with authorization information (the master location is set by the master flag).")
	flag.StringVar(&kubeContext, "kube-context", "", "Name of kube context")

	flag.StringVar(&storageClass, "storageclass", storageClass, "Kubernetes StorageClass name")
	flag.StringVar(&framework.DBCatalogName, "db-catalog", framework.DBCatalogName, "PerconaXtraDB version")
	flag.StringVar(&framework.DockerRegistry, "docker-registry", framework.DockerRegistry, "User provided docker repository")
}

const (
	TIMEOUT        = 20 * time.Minute
	KUBECONFIG_KEY = "KUBECONFIG"
)

var (
	root *framework.Framework
)

func TestE2e(t *testing.T) {
	kglog.InitLogs()
	defer kglog.FlushLogs()
	RegisterFailHandler(Fail)
	SetDefaultEventuallyTimeout(TIMEOUT)

	junitReporter := reporters.NewJUnitReporter("junit.xml")
	RunSpecsWithDefaultAndCustomReporters(t, "e2e Suite", []Reporter{junitReporter})
}

var _ = BeforeSuite(func() {
	By("Using kubeconfig from " + kubeconfigPath)
	config, err := clientcmd.BuildConfigFromContext(kubeconfigPath, kubeContext)
	Expect(err).NotTo(HaveOccurred())
	// raise throttling time. ref: https://github.com/appscode/voyager/issues/640
	config.Burst = 100
	config.QPS = 100

	// Clients
	kubeClient := kubernetes.NewForConfigOrDie(config)
	apiExtKubeClient := kext_cs.NewForConfigOrDie(config)
	dbClient := cs.NewForConfigOrDie(config)
	kaClient := ka.NewForConfigOrDie(config)
	appCatalogClient, err := appcat_cs.NewForConfig(config)
	stashClient := scs.NewForConfigOrDie(config)

	if err != nil {
		klog.Fatalln(err)
	}

	// Framework
	root = framework.New(config, kubeClient, apiExtKubeClient, dbClient, kaClient, appCatalogClient, stashClient, storageClass)

	// Create namespace
	By("Using namespace " + root.Namespace())
	err = root.CreateNamespace()
	Expect(err).NotTo(HaveOccurred())

	root.EventuallyCRD().Should(Succeed())
})

var _ = AfterSuite(func() {
	By("Cleanup Left Overs")
	By("Delete left over PerconaXtraDB objects")
	root.CleanPerconaXtraDB()
	By("Delete Namespace")
	err := root.DeleteNamespace()
	Expect(err).NotTo(HaveOccurred())
})
