package e2e_test

import (
	"fmt"
	"strconv"

	"github.com/appscode/go/log"
	"github.com/appscode/go/strings"
	"github.com/appscode/go/types"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	cat_api "kubedb.dev/apimachinery/apis/catalog/v1alpha1"
	api "kubedb.dev/apimachinery/apis/kubedb/v1alpha1"
	"kubedb.dev/percona-xtradb/test/e2e/framework"
	"kubedb.dev/percona-xtradb/test/e2e/matcher"
)

var _ = Describe("Percona XtraDB cluster Tests", func() {
	var (
		err            error
		f              *framework.Invocation
		percona        *api.Percona
		perconaVer     *cat_api.PerconaVersion
		garbagePercona *api.PerconaList
		//skipMessage string
		dbName         string
		dbNameKubedb   string
		wsClusterStats map[string]string
	)

	var createAndWaitForRunning = func() {
		By("Create Percona Version: " + perconaVer.Name)
		err = f.CreatePerconaVersion(perconaVer)
		Expect(err).NotTo(HaveOccurred())

		By("Create Percona: " + percona.Name)
		err = f.CreatePercona(percona)
		Expect(err).NotTo(HaveOccurred())

		By("Wait for Running percona")
		f.EventuallyPerconaRunning(percona.ObjectMeta).Should(BeTrue())

		By("Wait for AppBinding to create")
		f.EventuallyAppBinding(percona.ObjectMeta).Should(BeTrue())

		By("Check valid AppBinding Specs")
		err := f.CheckAppBindingSpec(percona.ObjectMeta)
		Expect(err).NotTo(HaveOccurred())

		By("Waiting for database to be ready")
		f.EventuallyDatabaseReady(percona.ObjectMeta, false, dbName, 0).Should(BeTrue())
	}
	var deleteTestResource = func() {
		if percona == nil {
			log.Infoln("Skipping cleanup. Reason: percona is nil")
			return
		}

		By("Check if percona " + percona.Name + " exists.")
		my, err := f.GetPercona(percona.ObjectMeta)
		if err != nil {
			if kerr.IsNotFound(err) {
				// Percona was not created. Hence, rest of cleanup is not necessary.
				return
			}
			Expect(err).NotTo(HaveOccurred())
		}

		By("Delete percona")
		err = f.DeletePercona(percona.ObjectMeta)
		if err != nil {
			if kerr.IsNotFound(err) {
				log.Infoln("Skipping rest of the cleanup. Reason: Percona does not exist.")
				return
			}
			Expect(err).NotTo(HaveOccurred())
		}

		if my.Spec.TerminationPolicy == api.TerminationPolicyPause {
			By("Wait for percona to be paused")
			f.EventuallyDormantDatabaseStatus(percona.ObjectMeta).Should(matcher.HavePaused())

			By("WipeOut percona")
			_, err := f.PatchDormantDatabase(percona.ObjectMeta, func(in *api.DormantDatabase) *api.DormantDatabase {
				in.Spec.WipeOut = true
				return in
			})
			Expect(err).NotTo(HaveOccurred())

			By("Delete Dormant Database")
			err = f.DeleteDormantDatabase(percona.ObjectMeta)
			Expect(err).NotTo(HaveOccurred())
		}

		By("Wait for percona resources to be wipedOut")
		f.EventuallyWipedOut(percona.ObjectMeta).Should(Succeed())

		if percona == nil {
			log.Infoln("Skipping cleanup. Reason: percona is nil")
			return
		}

		By("Check if percona version " + perconaVer.Name + " exists.")
		_, err = f.GetPerconaVersion(perconaVer.ObjectMeta)
		if err != nil {
			if kerr.IsNotFound(err) {
				// Percona was not created. Hence, rest of cleanup is not necessary.
				return
			}
			Expect(err).NotTo(HaveOccurred())
		}

		By("Delete PerconaVersion")
		err = f.DeletePerconaVersion(perconaVer.ObjectMeta)
		if err != nil {
			if kerr.IsNotFound(err) {
				log.Infoln("Skipping rest of the cleanup. Reason: PerconaVersion does not exist.")
				return
			}
			Expect(err).NotTo(HaveOccurred())
		}
	}

	var baseName = func(proxysql bool) string {
		if proxysql {
			return percona.ProxysqlName()
		}

		return percona.Name
	}

	var readFromEachPrimary = func(clusterSize, rowCnt int, proxysql bool) {
		for j := 0; j < clusterSize; j += 1 {
			By(fmt.Sprintf("Read row from member '%s-%d'", baseName(proxysql), j))
			f.EventuallyCountRow(percona.ObjectMeta, proxysql, dbNameKubedb, j).Should(Equal(rowCnt))
		}
	}

	var writeTo_N_ReadFrom_EachPrimary = func(clusterSize, existingRowCnt int, proxysql bool) {
		for i := 0; i < clusterSize; i += 1 {
			rowCnt := existingRowCnt + i + 1
			By(fmt.Sprintf("Insert row on member '%s-%d'", baseName(proxysql), i))
			f.EventuallyInsertRow(percona.ObjectMeta, proxysql, dbNameKubedb, i, 1).Should(BeTrue())
			readFromEachPrimary(clusterSize, rowCnt, proxysql)
		}
	}

	var replicationCheck = func(clusterSize int, proxysql bool) {
		By("Checking replication")
		f.EventuallyCreateDatabase(percona.ObjectMeta, proxysql, dbName, 0).Should(BeTrue())
		f.EventuallyCreateTable(percona.ObjectMeta, proxysql, dbNameKubedb, 0).Should(BeTrue())

		writeTo_N_ReadFrom_EachPrimary(clusterSize, 0, proxysql)
	}

	var storeWsClusterStats = func() {
		pods, err := f.KubeClient().CoreV1().Pods(percona.Namespace).List(metav1.ListOptions{
			LabelSelector: labels.Set(percona.ClusterSelectors()).String(),
		})
		Expect(err).NotTo(HaveOccurred())
		clusterMembersAddr := make([]*string, 0)
		for _, pod := range pods.Items {
			addr := fmt.Sprintf("%s:%d", pod.Status.PodIP, api.MySQLNodePort)
			clusterMembersAddr = append(clusterMembersAddr, &addr)
		}

		wsClusterStats = map[string]string{
			"wsrep_local_state":         strconv.Itoa(4),
			"wsrep_local_state_comment": "Synced",
			"wsrep_incoming_addresses":  strings.Join(clusterMembersAddr, ","),
			"wsrep_evs_state":           "OPERATIONAL",
			"wsrep_cluster_size":        strconv.Itoa(len(pods.Items)),
			"wsrep_cluster_status":      "Primary",
			"wsrep_connected":           "ON",
			"wsrep_ready":               "ON",
		}
	}

	var CheckDBVersionForXtraDBCluster = func() {
		if framework.DBVersion != "5.7" {
			Skip("For XtraDB Cluster, currently supported DB version is '5.7'")
		}
	}

	BeforeEach(func() {
		f = root.Invoke()
		percona = f.PerconaXtraDBCluster()
		perconaVer = f.PerconaVersion()
		garbagePercona = new(api.PerconaList)
		//skipMessage = ""
		dbName = "mysql"
		dbNameKubedb = "kubedb"

		CheckDBVersionForXtraDBCluster()
	})

	Context("Behaviour tests", func() {

		AfterEach(func() {
			// delete resources for current Percona
			deleteTestResource()

			// old Percona are in garbagePercona list. delete their resources.
			for _, my := range garbagePercona.Items {
				*percona = my
				deleteTestResource()
			}

			By("Delete left over workloads if exists any")
			f.CleanWorkloadLeftOvers()
		})

		Context("Basic Cluster with 3 member", func() {
			BeforeEach(func() {
				createAndWaitForRunning()
				storeWsClusterStats()
			})

			It("should be possible to create a basic 3 member cluster", func() {
				for i := 0; i < api.PerconaDefaultClusterSize; i++ {
					By(fmt.Sprintf("Checking the cluster stats from Pod '%s-%d'", percona.Name, i))
					f.EventuallyCheckCluster(percona.ObjectMeta, false, dbName, i, wsClusterStats).
						Should(Equal(true))
				}

				replicationCheck(api.PerconaDefaultClusterSize, false)
			})
		})

		Context("Failover", func() {
			BeforeEach(func() {
				createAndWaitForRunning()
				storeWsClusterStats()
			})

			It("should failover successfully", func() {
				for i := 0; i < api.PerconaDefaultClusterSize; i++ {
					By(fmt.Sprintf("Checking the cluster stats from Pod '%s-%d'", percona.Name, i))
					f.EventuallyCheckCluster(percona.ObjectMeta, false, dbName, i, wsClusterStats).
						Should(Equal(true))
				}
				replicationCheck(api.PerconaDefaultClusterSize, false)

				By(fmt.Sprintf("Taking down the primary '%s-%d'", percona.Name, 0))
				err = f.RemoverPrimary(percona.ObjectMeta, 0)
				Expect(err).NotTo(HaveOccurred())

				By(fmt.Sprintf("Checking status after failing primary '%s-%d'", percona.Name, 0))
				for i := 0; i < api.PerconaDefaultClusterSize; i++ {
					By(fmt.Sprintf("Checking the cluster stats member count from Pod '%s-%d'", percona.Name, i))
					f.EventuallyCheckCluster(percona.ObjectMeta, false, dbName, i, wsClusterStats).
						Should(Equal(true))
				}

				By("Checking for data after failover")
				readFromEachPrimary(api.PerconaDefaultClusterSize, 3, false)
			})
		})

		Context("Scale up", func() {
			BeforeEach(func() {
				createAndWaitForRunning()
				storeWsClusterStats()
			})

			It("should be possible to scale up", func() {
				for i := 0; i < api.PerconaDefaultClusterSize; i++ {
					By(fmt.Sprintf("Checking the cluster stats from Pod '%s-%d'", percona.Name, i))
					f.EventuallyCheckCluster(percona.ObjectMeta, false, dbName, i, wsClusterStats).
						Should(Equal(true))
				}
				replicationCheck(api.PerconaDefaultClusterSize, false)

				By("Scaling up")
				percona, err = f.PatchPercona(percona.ObjectMeta, func(in *api.Percona) *api.Percona {
					in.Spec.Replicas = types.Int32P(api.PerconaDefaultClusterSize + 1)

					return in
				})
				Expect(err).NotTo(HaveOccurred())
				By("Wait for percona be patched")
				Expect(f.WaitUntilPerconaReplicasBePatched(percona.ObjectMeta, api.PerconaDefaultClusterSize+1)).
					NotTo(HaveOccurred())

				By("Wait for new member to be ready")
				Expect(f.WaitUntilPodRunningBySelector(percona, false)).NotTo(HaveOccurred())
				By("Wait for proxysql to be ready")
				Expect(f.WaitUntilPodRunningBySelector(percona, true)).NotTo(HaveOccurred())

				By("Checking status after scaling up")
				storeWsClusterStats()
				for i := 0; i < api.PerconaDefaultClusterSize+1; i++ {
					By(fmt.Sprintf("Checking the cluster stats member count from Pod '%s-%d'", percona.Name, i))
					f.EventuallyCheckCluster(percona.ObjectMeta, false, dbName, i, wsClusterStats).
						Should(Equal(true))
				}

				By("Checking for data after scaling up")
				readFromEachPrimary(api.PerconaDefaultClusterSize+1, 3, false)
				writeTo_N_ReadFrom_EachPrimary(api.PerconaDefaultClusterSize+1, 3, false)
			})
		})

		Context("Scale down", func() {
			BeforeEach(func() {
				percona.Spec.Replicas = types.Int32P(4)

				createAndWaitForRunning()
				storeWsClusterStats()
			})

			It("Should be possible to scale down", func() {
				for i := 0; i < api.PerconaDefaultClusterSize+1; i++ {
					By(fmt.Sprintf("Checking the cluster stats from Pod '%s-%d'", percona.Name, i))
					f.EventuallyCheckCluster(percona.ObjectMeta, false, dbName, i, wsClusterStats).
						Should(Equal(true))
				}
				replicationCheck(api.PerconaDefaultClusterSize+1, false)

				By("Scaling down")
				percona, err = f.PatchPercona(percona.ObjectMeta, func(in *api.Percona) *api.Percona {
					in.Spec.Replicas = types.Int32P(api.PerconaDefaultClusterSize)

					return in
				})
				Expect(err).NotTo(HaveOccurred())
				By("Wait for percona be patched")
				Expect(f.WaitUntilPerconaReplicasBePatched(percona.ObjectMeta, api.PerconaDefaultClusterSize)).
					NotTo(HaveOccurred())

				By("Wait for new member to be ready")
				Expect(f.WaitUntilPodRunningBySelector(percona, false)).NotTo(HaveOccurred())
				By("Wait for proxysql to be ready")
				Expect(f.WaitUntilPodRunningBySelector(percona, true)).NotTo(HaveOccurred())

				By("Checking status after scaling down")
				storeWsClusterStats()
				for i := 0; i < api.PerconaDefaultClusterSize; i++ {
					By(fmt.Sprintf("Checking the cluster stats member count from Pod '%s-%d'", percona.Name, i))
					f.EventuallyCheckCluster(percona.ObjectMeta, false, dbName, i, wsClusterStats).
						Should(Equal(true))
				}

				By("Checking for data after scaling down")
				readFromEachPrimary(api.PerconaDefaultClusterSize, 4, false)
				writeTo_N_ReadFrom_EachPrimary(api.PerconaDefaultClusterSize, 4, false)
			})
		})

		Context("Proxysql", func() {
			BeforeEach(func() {
				createAndWaitForRunning()
				storeWsClusterStats()

			})

			It("should configure poxysql for backend server", func() {
				for i := 0; i < api.PerconaDefaultClusterSize; i++ {
					By(fmt.Sprintf("Checking the cluster stats from Pod '%s-%d'", percona.Name, i))
					f.EventuallyCheckCluster(percona.ObjectMeta, false, dbName, i, wsClusterStats).
						Should(Equal(true))
				}
				for i := 0; i < int(*percona.Spec.PXC.Proxysql.Replicas); i++ {
					By(fmt.Sprintf("Checking the cluster stats from Proxysql Pod '%s-%d'", percona.ProxysqlName(), i))
					f.EventuallyCheckCluster(percona.ObjectMeta, true, dbName, i, wsClusterStats).
						Should(Equal(true))
				}
				replicationCheck(int(*percona.Spec.PXC.Proxysql.Replicas), true)
			})
		})
	})
})
