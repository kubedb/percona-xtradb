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
package e2e_test

import (
	"fmt"
	"os"
	"strconv"

	api "kubedb.dev/apimachinery/apis/kubedb/v1alpha1"
	"kubedb.dev/percona-xtradb/test/e2e/framework"
	"kubedb.dev/percona-xtradb/test/e2e/matcher"

	"github.com/appscode/go/log"
	"github.com/appscode/go/types"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	store "kmodules.xyz/objectstore-api/api/v1"
	stashV1alpha1 "stash.appscode.dev/stash/apis/stash/v1alpha1"
	stashV1beta1 "stash.appscode.dev/stash/apis/stash/v1beta1"
)

var _ = Describe("PerconaXtraDB cluster Tests", func() {
	const (
		googleProjectIDKey          = "GOOGLE_PROJECT_ID"
		googleServiceAccountJsonKey = "GOOGLE_SERVICE_ACCOUNT_JSON_KEY"
		googleBucketNameKey         = "GCS_BUCKET_NAME"
	)

	var (
		err                  error
		f                    *framework.Invocation
		px                   *api.PerconaXtraDB
		garbagePerconaXtraDB *api.PerconaXtraDBList
		dbName               string
		dbNameKubedb         string
		wsClusterStats       map[string]string
		secret               *corev1.Secret
	)

	var isSetEnv = func(key string) bool {
		_, set := os.LookupEnv(key)

		return set
	}

	var createAndWaitForRunningPerconaXtraDB = func() {
		By("Create PerconaXtraDB: " + px.Name)
		err = f.CreatePerconaXtraDB(px)
		Expect(err).NotTo(HaveOccurred())

		By("Wait for Running PerconaXtraDB")
		f.EventuallyPerconaXtraDBRunning(px.ObjectMeta).Should(BeTrue())

		By("Wait for AppBinding to create")
		f.EventuallyAppBinding(px.ObjectMeta).Should(BeTrue())

		By("Check valid AppBinding Specs")
		err := f.CheckAppBindingSpec(px.ObjectMeta)
		Expect(err).NotTo(HaveOccurred())

		By("Waiting for database to be ready")
		f.EventuallyDatabaseReady(px.ObjectMeta, dbName, 0).Should(BeTrue())
	}

	var deletePerconaXtraDBResource = func() {
		if px == nil {
			log.Infoln("Skipping cleanup. Reason: PerconaXtraDB object is nil")
			return
		}

		By("Check if PerconaXtraDB " + px.Name + " exists.")
		perconaxtradb, err := f.GetPerconaXtraDB(px.ObjectMeta)
		if err != nil {
			if kerr.IsNotFound(err) {
				// PerconaXtraDB was not created. Hence, rest of cleanup is not necessary.
				return
			}
			Expect(err).NotTo(HaveOccurred())
		}

		By("Delete PerconaXtraDB")
		err = f.DeletePerconaXtraDB(px.ObjectMeta)
		if err != nil {
			if kerr.IsNotFound(err) {
				log.Infoln("Skipping rest of the cleanup. Reason: PerconaXtraDB does not exist.")
				return
			}
			Expect(err).NotTo(HaveOccurred())
		}

		if perconaxtradb.Spec.TerminationPolicy == api.TerminationPolicyPause {
			By("Wait for PerconaXtraDB to be paused")
			f.EventuallyDormantDatabaseStatus(px.ObjectMeta).Should(matcher.HavePaused())

			By("WipeOut PerconaXtraDB")
			_, err := f.PatchDormantDatabase(px.ObjectMeta, func(in *api.DormantDatabase) *api.DormantDatabase {
				in.Spec.WipeOut = true
				return in
			})
			Expect(err).NotTo(HaveOccurred())

			By("Delete Dormant Database")
			err = f.DeleteDormantDatabase(px.ObjectMeta)
			Expect(err).NotTo(HaveOccurred())
		}

		By("Wait for perconaxtradb resources to be wipedOut")
		f.EventuallyWipedOut(px.ObjectMeta).Should(Succeed())
	}

	var deleteTestResource = func() {
		deletePerconaXtraDBResource()
	}

	var deleteLeftOverStuffs = func() {
		// old PerconaXtraDB are in garbagePerconaXtraDB list. delete their resources.
		for _, p := range garbagePerconaXtraDB.Items {
			*px = p
			deleteTestResource()
		}

		By("Delete left over workloads if exists any")
		f.CleanWorkloadLeftOvers()
	}

	var countRows = func(meta metav1.ObjectMeta, podIndex, expectedRowCnt int) {
		By(fmt.Sprintf("Read row from member '%s-%d'", meta.Name, podIndex))
		f.EventuallyCountRow(meta, dbNameKubedb, podIndex).Should(Equal(expectedRowCnt))
	}

	var insertRows = func(meta metav1.ObjectMeta, podIndex, rowCntToInsert int) {
		By(fmt.Sprintf("Insert row on member '%s-%d'", meta.Name, podIndex))
		f.EventuallyInsertRow(meta, dbNameKubedb, podIndex, rowCntToInsert).Should(BeTrue())
	}

	var create_Database_N_Table = func(meta metav1.ObjectMeta, podIndex int) {
		By("Create Database")
		f.EventuallyCreateDatabase(meta, dbName, podIndex).Should(BeTrue())

		By("Create Table")
		f.EventuallyCreateTable(meta, dbNameKubedb, podIndex).Should(BeTrue())
	}

	var readFromEachPrimary = func(meta metav1.ObjectMeta, clusterSize, rowCnt int) {
		for j := 0; j < clusterSize; j += 1 {
			countRows(meta, j, rowCnt)
		}
	}

	var writeTo_N_ReadFrom_EachPrimary = func(meta metav1.ObjectMeta, clusterSize, existingRowCnt int) {
		for i := 0; i < clusterSize; i += 1 {
			totalRowCnt := existingRowCnt + i + 1
			insertRows(meta, i, 1)
			readFromEachPrimary(meta, clusterSize, totalRowCnt)
		}
	}

	var replicationCheck = func(meta metav1.ObjectMeta, clusterSize int) {
		By("Checking replication")
		create_Database_N_Table(meta, 0)
		writeTo_N_ReadFrom_EachPrimary(meta, clusterSize, 0)
	}

	var storeWsClusterStats = func() {
		pods, err := f.KubeClient().CoreV1().Pods(px.Namespace).List(metav1.ListOptions{
			LabelSelector: labels.Set(px.OffshootSelectors()).String(),
		})
		Expect(err).NotTo(HaveOccurred())

		wsClusterStats = map[string]string{
			"wsrep_local_state":         strconv.Itoa(4),
			"wsrep_local_state_comment": "Synced",
			"wsrep_evs_state":           "OPERATIONAL",
			"wsrep_cluster_size":        strconv.Itoa(len(pods.Items)),
			"wsrep_cluster_status":      "Primary",
			"wsrep_connected":           "ON",
			"wsrep_ready":               "ON",
		}
	}

	var CheckDBVersionForXtraDBCluster = func() {
		if framework.DBCatalogName != "5.7-cluster" {
			Skip("For XtraDB Cluster, currently supported DB version is '5.7-cluster'")
		}
	}

	BeforeEach(func() {
		f = root.Invoke()
		px = f.PerconaXtraDBCluster()
		garbagePerconaXtraDB = new(api.PerconaXtraDBList)
		dbName = "mysql"
		dbNameKubedb = "kubedb"

		By("Ensure the apiservices are ready")
		f.EnsureAPIServiceReady().Should(Succeed())

		CheckDBVersionForXtraDBCluster()
	})

	JustAfterEach(func() {
		if CurrentGinkgoTestDescription().Failed {
			f.PrintDebugHelpers(px.Name, int(*px.Spec.Replicas))
		}
	})

	Context("Behaviour tests", func() {

		AfterEach(func() {
			// delete resources for current PerconaXtraDB
			deleteTestResource()
			deleteLeftOverStuffs()
		})

		Context("Basic Cluster with 3 member", func() {
			BeforeEach(func() {
				createAndWaitForRunningPerconaXtraDB()
				storeWsClusterStats()
			})

			FIt("should be possible to create a basic 3 member cluster", func() {
				for i := 0; i < api.PerconaXtraDBDefaultClusterSize; i++ {
					By(fmt.Sprintf("Checking the cluster stats from Pod '%s-%d'", px.Name, i))
					f.EventuallyCheckCluster(px.ObjectMeta, dbName, i, wsClusterStats).
						Should(Equal(true))
				}

				replicationCheck(px.ObjectMeta, api.PerconaXtraDBDefaultClusterSize)
			})
		})

		Context("Failover", func() {
			BeforeEach(func() {
				createAndWaitForRunningPerconaXtraDB()
				storeWsClusterStats()
			})

			It("should failover successfully", func() {
				for i := 0; i < api.PerconaXtraDBDefaultClusterSize; i++ {
					By(fmt.Sprintf("Checking the cluster stats from Pod '%s-%d'", px.Name, i))
					f.EventuallyCheckCluster(px.ObjectMeta, dbName, i, wsClusterStats).
						Should(Equal(true))
				}
				replicationCheck(px.ObjectMeta, api.PerconaXtraDBDefaultClusterSize)

				By(fmt.Sprintf("Taking down the primary '%s-%d'", px.Name, 0))
				err = f.RemoverPrimary(px.ObjectMeta, 0)
				Expect(err).NotTo(HaveOccurred())

				By(fmt.Sprintf("Checking status after failing primary '%s-%d'", px.Name, 0))
				for i := 0; i < api.PerconaXtraDBDefaultClusterSize; i++ {
					By(fmt.Sprintf("Checking the cluster stats member count from Pod '%s-%d'", px.Name, i))
					storeWsClusterStats()
					f.EventuallyCheckCluster(px.ObjectMeta, dbName, i, wsClusterStats).
						Should(Equal(true))
				}

				By("Checking for data after failover")
				readFromEachPrimary(px.ObjectMeta, api.PerconaXtraDBDefaultClusterSize, 3)
			})
		})

		Context("Scale up", func() {
			BeforeEach(func() {
				createAndWaitForRunningPerconaXtraDB()
				storeWsClusterStats()
			})

			It("should be possible to scale up", func() {
				for i := 0; i < api.PerconaXtraDBDefaultClusterSize; i++ {
					By(fmt.Sprintf("Checking the cluster stats from Pod '%s-%d'", px.Name, i))
					f.EventuallyCheckCluster(px.ObjectMeta, dbName, i, wsClusterStats).
						Should(Equal(true))
				}
				replicationCheck(px.ObjectMeta, api.PerconaXtraDBDefaultClusterSize)

				By("Scaling up")
				px, err = f.PatchPerconaXtraDB(px.ObjectMeta, func(in *api.PerconaXtraDB) *api.PerconaXtraDB {
					in.Spec.Replicas = types.Int32P(api.PerconaXtraDBDefaultClusterSize + 1)

					return in
				})
				Expect(err).NotTo(HaveOccurred())
				By("Wait for PerconaXtraDB be patched")
				Expect(f.WaitUntilPerconaXtraDBReplicasBePatched(px.ObjectMeta, api.PerconaXtraDBDefaultClusterSize+1)).
					NotTo(HaveOccurred())

				By("Wait for new member to be ready")
				Expect(f.WaitUntilPodRunningBySelector(
					px.Namespace, px.OffshootSelectors(), int(types.Int32(px.Spec.Replicas)),
				)).NotTo(HaveOccurred())

				By("Checking status after scaling up")
				storeWsClusterStats()
				for i := 0; i < api.PerconaXtraDBDefaultClusterSize+1; i++ {
					By(fmt.Sprintf("Checking the cluster stats member count from Pod '%s-%d'", px.Name, i))
					f.EventuallyCheckCluster(px.ObjectMeta, dbName, i, wsClusterStats).
						Should(Equal(true))
				}

				By("Checking for data after scaling up")
				readFromEachPrimary(px.ObjectMeta, api.PerconaXtraDBDefaultClusterSize+1, 3)
				writeTo_N_ReadFrom_EachPrimary(px.ObjectMeta, api.PerconaXtraDBDefaultClusterSize+1, 3)
			})
		})

		Context("Scale down", func() {
			BeforeEach(func() {
				px.Spec.Replicas = types.Int32P(4)

				createAndWaitForRunningPerconaXtraDB()
				storeWsClusterStats()
			})

			It("Should be possible to scale down", func() {
				for i := 0; i < api.PerconaXtraDBDefaultClusterSize+1; i++ {
					By(fmt.Sprintf("Checking the cluster stats from Pod '%s-%d'", px.Name, i))
					f.EventuallyCheckCluster(px.ObjectMeta, dbName, i, wsClusterStats).
						Should(Equal(true))
				}
				replicationCheck(px.ObjectMeta, api.PerconaXtraDBDefaultClusterSize+1)

				By("Scaling down")
				px, err = f.PatchPerconaXtraDB(px.ObjectMeta, func(in *api.PerconaXtraDB) *api.PerconaXtraDB {
					in.Spec.Replicas = types.Int32P(api.PerconaXtraDBDefaultClusterSize)

					return in
				})
				Expect(err).NotTo(HaveOccurred())
				By("Wait for PerconaXtraDB be patched")
				Expect(f.WaitUntilPerconaXtraDBReplicasBePatched(px.ObjectMeta, api.PerconaXtraDBDefaultClusterSize)).
					NotTo(HaveOccurred())

				By("Wait for new member to be ready")
				Expect(f.WaitUntilPodRunningBySelector(
					px.Namespace, px.OffshootSelectors(), int(types.Int32(px.Spec.Replicas)),
				)).NotTo(HaveOccurred())

				By("Checking status after scaling down")
				storeWsClusterStats()
				for i := 0; i < api.PerconaXtraDBDefaultClusterSize; i++ {
					By(fmt.Sprintf("Checking the cluster stats member count from Pod '%s-%d'", px.Name, i))
					f.EventuallyCheckCluster(px.ObjectMeta, dbName, i, wsClusterStats).
						Should(Equal(true))
				}

				By("Checking for data after scaling down")
				readFromEachPrimary(px.ObjectMeta, api.PerconaXtraDBDefaultClusterSize, 4)
				writeTo_N_ReadFrom_EachPrimary(px.ObjectMeta, api.PerconaXtraDBDefaultClusterSize, 4)
			})
		})
	})

	Context("Initialize", func() {
		// To run this test,
		// 1st: Deploy stash latest operator
		// 2nd: create mysql related tasks and functions either
		//	 or	from helm chart in `stash.appscode.dev/percona-xtradb/chart/stash-percona-xtradb`
		Context("With Stash/Restic", func() {
			var bc *stashV1beta1.BackupConfiguration
			var bs *stashV1beta1.BackupSession
			var rs *stashV1beta1.RestoreSession
			var repo *stashV1alpha1.Repository

			BeforeEach(func() {
				if !f.FoundStashCRDs() {
					Skip("Skipping tests for stash integration. reason: stash operator is not running.")
				}

				if !isSetEnv(googleProjectIDKey) ||
					!isSetEnv(googleServiceAccountJsonKey) ||
					!isSetEnv(googleBucketNameKey) {

					Skip("Skipping tests for stash integration. reason: " +
						fmt.Sprintf("env vars %q, %q and %q are required",
							googleProjectIDKey, googleServiceAccountJsonKey, googleBucketNameKey))
				}
			})

			AfterEach(func() {
				By("Deleting BackupConfiguration")
				err := f.DeleteBackupConfiguration(bc.ObjectMeta)
				Expect(err).NotTo(HaveOccurred())

				By("Deleting RestoreSession")
				err = f.DeleteRestoreSession(rs.ObjectMeta)
				Expect(err).NotTo(HaveOccurred())

				By("Deleting Repository")
				err = f.DeleteRepository(repo.ObjectMeta)
				Expect(err).NotTo(HaveOccurred())

				deleteTestResource()
				deleteLeftOverStuffs()
			})

			var createAndWaitForInitializing = func() {
				By("Creating MySQL: " + px.Name)
				err = f.CreatePerconaXtraDB(px)
				Expect(err).NotTo(HaveOccurred())

				By("Wait for Initializing mysql")
				f.EventuallyPerconaXtraDBPhase(px.ObjectMeta).Should(Equal(api.DatabasePhaseInitializing))
			}

			var shouldInitializeFromStash = func() {
				// Create and wait for running MySQL
				createAndWaitForRunningPerconaXtraDB()

				create_Database_N_Table(px.ObjectMeta, 0)
				insertRows(px.ObjectMeta, 0, 3)
				countRows(px.ObjectMeta, 0, 3)

				By("Create Secret")
				err = f.CreateSecret(secret)
				Expect(err).NotTo(HaveOccurred())

				By("Create Repositories")
				err = f.CreateRepository(repo)
				Expect(err).NotTo(HaveOccurred())

				By("Create BackupConfiguration")
				err = f.CreateBackupConfiguration(bc)
				Expect(err).NotTo(HaveOccurred())

				By("Wait until BackupSession be created")
				bs, err = f.WaitUntilBackkupSessionBeCreated(bc.ObjectMeta)

				// eventually backupsession succeeded
				By("Check for Succeeded backupsession")
				f.EventuallyBackupSessionPhase(bs.ObjectMeta).Should(Equal(stashV1beta1.BackupSessionSucceeded))

				oldPerconaXtraDB, err := f.GetPerconaXtraDB(px.ObjectMeta)
				Expect(err).NotTo(HaveOccurred())

				garbagePerconaXtraDB.Items = append(garbagePerconaXtraDB.Items, *oldPerconaXtraDB)

				By("Create PerconaXtraDB for initializing from stash")
				*px = *f.PerconaXtraDBCluster()
				rs = f.RestoreSession(px.ObjectMeta, oldPerconaXtraDB.ObjectMeta, oldPerconaXtraDB.Spec.Replicas)
				px.Spec.DatabaseSecret = oldPerconaXtraDB.Spec.DatabaseSecret
				px.Spec.Init = &api.InitSpec{
					StashRestoreSession: &corev1.LocalObjectReference{
						Name: rs.Name,
					},
				}

				// Create and wait for running MySQL
				createAndWaitForInitializing()

				By("Create RestoreSession")
				err = f.CreateRestoreSession(rs)
				Expect(err).NotTo(HaveOccurred())

				// eventually restoresession succeeded
				By("Check for Succeeded restoreSession")
				f.EventuallyRestoreSessionPhase(rs.ObjectMeta).Should(Equal(stashV1beta1.RestoreSessionSucceeded))

				By("Wait for Running mysql")
				f.EventuallyPerconaXtraDBRunning(px.ObjectMeta).Should(BeTrue())

				By("Wait for AppBinding to create")
				f.EventuallyAppBinding(px.ObjectMeta).Should(BeTrue())

				By("Check valid AppBinding Specs")
				err = f.CheckAppBindingSpec(px.ObjectMeta)
				Expect(err).NotTo(HaveOccurred())

				By("Waiting for database to be ready")
				f.EventuallyDatabaseReady(px.ObjectMeta, dbName, 0).Should(BeTrue())

				countRows(px.ObjectMeta, 0, 3)
			}

			Context("From GCS backend", func() {

				BeforeEach(func() {
					secret = f.SecretForGCSBackend()
					secret = f.PatchSecretForRestic(secret)
					bc = f.BackupConfiguration(px.ObjectMeta)
					repo = f.Repository(px.ObjectMeta, secret.Name)

					repo.Spec.Backend = store.Backend{
						GCS: &store.GCSSpec{
							Bucket: os.Getenv(googleBucketNameKey),
							Prefix: fmt.Sprintf("stash/%v/%v", px.Namespace, px.Name),
						},
						StorageSecretName: secret.Name,
					}
				})

				It("should run successfully", shouldInitializeFromStash)
			})
		})
	})
})
