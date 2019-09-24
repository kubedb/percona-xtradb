package e2e_test

import (
	"flag"
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/appscode/go/homedir"
	. "github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/reporters"
	. "github.com/onsi/gomega"
	kext_cs "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1beta1"
	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/client-go/kubernetes"
	clientSetScheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/tools/clientcmd"
	ka "k8s.io/kube-aggregator/pkg/client/clientset_generated/clientset"
	"kmodules.xyz/client-go/logs"
	appcat_cs "kmodules.xyz/custom-resources/client/clientset/versioned/typed/appcatalog/v1alpha1"
	cs "kubedb.dev/apimachinery/client/clientset/versioned"
	"kubedb.dev/apimachinery/client/clientset/versioned/scheme"
	"kubedb.dev/percona-xtradb/test/e2e/framework"
	scs "stash.appscode.dev/stash/client/clientset/versioned"
)

var (
	storageClass = "standard"
)

func init() {
	if err := scheme.AddToScheme(clientSetScheme.Scheme); err != nil {
		log.Println(err)
	}

	flag.StringVar(&storageClass, "storageclass", storageClass, "Kubernetes StorageClass name")
	flag.StringVar(&framework.DBCatalogName, "db-catalog", framework.DBCatalogName, "PerconaXtraDB version")
	flag.StringVar(&framework.DockerRegistry, "docker-registry", framework.DockerRegistry, "User provided docker repository")
	flag.BoolVar(&framework.SelfHostedOperator, "selfhosted-operator", framework.SelfHostedOperator, "Enable this for provided controller")

	// for ProxySQL
	flag.BoolVar(&framework.ProxySQLTest, "proxysql", framework.ProxySQLTest, "Enable this for proxysql controller")
	flag.StringVar(&framework.ProxySQLCatalogName, "psql-catalog", framework.ProxySQLCatalogName, "ProxySQL version")
}

const (
	TIMEOUT        = 20 * time.Minute
	KUBECONFIG_KEY = "KUBECONFIG"
)

var (
	//ctrl *controller.Controller
	root *framework.Framework
)

func TestE2e(t *testing.T) {
	logs.InitLogs()
	defer logs.FlushLogs()
	RegisterFailHandler(Fail)
	SetDefaultEventuallyTimeout(TIMEOUT)

	junitReporter := reporters.NewJUnitReporter("junit.xml")
	RunSpecsWithDefaultAndCustomReporters(t, "e2e Suite", []Reporter{junitReporter})
}

var _ = BeforeSuite(func() {
	// Kubernetes config
	//fmt.Println(">>>>>>>", sh.NewSession().Command("ls", "/.kube", "-lah").Run())
	//fmt.Println(">>>>>>>>>", homedir.HomeDir())
	//fmt.Println()
	//kubeconfigPath, found := os.LookupEnv(KUBECONFIG_KEY)
	//if !found {
	//	kubeconfigPath = filepath.Join(homedir.HomeDir(), ".kube/config")
	//}

	kubeconfigPath := filepath.Join(homedir.HomeDir(), os.Getenv(KUBECONFIG_KEY))
	By("Using kubeconfig from " + kubeconfigPath)
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
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
		log.Fatalln(err)
	}

	// Framework
	root = framework.New(config, kubeClient, apiExtKubeClient, dbClient, kaClient, appCatalogClient, stashClient, storageClass)

	// Create namespace
	By("Using namespace " + root.Namespace())
	err = root.CreateNamespace()
	Expect(err).NotTo(HaveOccurred())

	if !framework.SelfHostedOperator {
		stopCh := genericapiserver.SetupSignalHandler()
		go root.RunOperatorAndServer(config, kubeconfigPath, stopCh)
	}

	root.EventuallyCRD().Should(Succeed())
	root.EventuallyAPIServiceReady().Should(Succeed())
})

var _ = AfterSuite(func() {

	By("Cleanup Left Overs")
	if !framework.SelfHostedOperator {
		By("Delete Admission Controller Configs")
		root.CleanAdmissionConfigs()
	}
	By("Delete left over PerconaXtraDB objects")
	root.CleanPerconaXtraDB()
	By("Delete left over Dormant Database objects")
	root.CleanDormantDatabase()
	By("Delete left over Snapshot objects")
	root.CleanSnapshot()
	By("Delete Namespace")
	err := root.DeleteNamespace()
	Expect(err).NotTo(HaveOccurred())
})
