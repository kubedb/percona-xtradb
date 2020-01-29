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

	api "kubedb.dev/apimachinery/apis/kubedb/v1alpha1"
	"kubedb.dev/apimachinery/client/clientset/versioned/typed/kubedb/v1alpha1/util"
	"kubedb.dev/percona-xtradb/test/e2e/framework"
	"kubedb.dev/percona-xtradb/test/e2e/matcher"

	"github.com/appscode/go/log"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	core "k8s.io/api/core/v1"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	meta_util "kmodules.xyz/client-go/meta"
	store "kmodules.xyz/objectstore-api/api/v1"
	stashV1alpha1 "stash.appscode.dev/stash/apis/stash/v1alpha1"
	stashV1beta1 "stash.appscode.dev/stash/apis/stash/v1beta1"
)

const (
	S3_BUCKET_NAME       = "S3_BUCKET_NAME"
	GCS_BUCKET_NAME      = "GCS_BUCKET_NAME"
	AZURE_CONTAINER_NAME = "AZURE_CONTAINER_NAME"
	SWIFT_CONTAINER_NAME = "SWIFT_CONTAINER_NAME"
	MYSQL_DATABASE       = "MYSQL_DATABASE"
	MYSQL_ROOT_PASSWORD  = "MYSQL_ROOT_PASSWORD"
)

const (
	googleProjectIDKey          = "GOOGLE_PROJECT_ID"
	googleServiceAccountJsonKey = "GOOGLE_SERVICE_ACCOUNT_JSON_KEY"
	googleBucketNameKey         = "GCS_BUCKET_NAME"
)

var _ = Describe("PerconaXtraDB", func() {
	var (
		err                  error
		f                    *framework.Invocation
		perconaxtradb        *api.PerconaXtraDB
		garbagePerconaXtraDB *api.PerconaXtraDBList
		secret               *core.Secret
		skipMessage          string
		dbName               string
		dbNameKubedb         string
	)

	BeforeEach(func() {
		f = root.Invoke()
		perconaxtradb = f.PerconaXtraDB()
		garbagePerconaXtraDB = new(api.PerconaXtraDBList)
		skipMessage = ""
		dbName = "mysql"
		dbNameKubedb = "kubedb"
	})

	var isSetEnv = func(key string) bool {
		_, set := os.LookupEnv(key)

		return set
	}

	var createAndWaitForRunning = func() {
		By("Create PerconaXtraDB: " + perconaxtradb.Name)
		err = f.CreatePerconaXtraDB(perconaxtradb)
		Expect(err).NotTo(HaveOccurred())

		By("Wait for Running PerconaXtraDB")
		f.EventuallyPerconaXtraDBRunning(perconaxtradb.ObjectMeta).Should(BeTrue())

		By("Wait for AppBinding to create")
		f.EventuallyAppBinding(perconaxtradb.ObjectMeta).Should(BeTrue())

		By("Check valid AppBinding Specs")
		err := f.CheckAppBindingSpec(perconaxtradb.ObjectMeta)
		Expect(err).NotTo(HaveOccurred())

		By("Waiting for database to be ready")
		f.EventuallyDatabaseReady(perconaxtradb.ObjectMeta, dbName, 0).Should(BeTrue())
	}

	var create_Database_N_Table = func(meta metav1.ObjectMeta, podIndex int) {
		By("Create Database")
		f.EventuallyCreateDatabase(meta, dbName, podIndex).Should(BeTrue())

		By("Create Table")
		f.EventuallyCreateTable(meta, dbNameKubedb, podIndex).Should(BeTrue())
	}

	var countRows = func(meta metav1.ObjectMeta, podIndex, expectedRowCnt int) {
		By(fmt.Sprintf("Read row from member '%s-%d'", meta.Name, podIndex))
		f.EventuallyCountRow(meta, dbNameKubedb, podIndex).Should(Equal(expectedRowCnt))
	}

	var insertRows = func(meta metav1.ObjectMeta, podIndex, rowCntToInsert int) {
		By(fmt.Sprintf("Insert row on member '%s-%d'", meta.Name, podIndex))
		f.EventuallyInsertRow(meta, dbNameKubedb, podIndex, rowCntToInsert).Should(BeTrue())
	}

	var testGeneralBehaviour = func() {
		if skipMessage != "" {
			Skip(skipMessage)
		}
		// Create PerconaXtraDB
		createAndWaitForRunning()

		By("Creating Table")
		f.EventuallyCreateTable(perconaxtradb.ObjectMeta, dbName, 0).Should(BeTrue())

		By("Inserting Rows")
		f.EventuallyInsertRow(perconaxtradb.ObjectMeta, dbName, 0, 3).Should(BeTrue())

		By("Checking Row Count of Table")
		f.EventuallyCountRow(perconaxtradb.ObjectMeta, dbName, 0).Should(Equal(3))

		By("Delete PerconaXtraDB")
		err = f.DeletePerconaXtraDB(perconaxtradb.ObjectMeta)
		Expect(err).NotTo(HaveOccurred())

		By("Wait for perconaxtradb to be deleted")
		f.EventuallyPerconaXtraDB(perconaxtradb.ObjectMeta).Should(BeFalse())

		// Create PerconaXtraDB object again to resume it
		By("Create PerconaXtraDB: " + perconaxtradb.Name)
		err = f.CreatePerconaXtraDB(perconaxtradb)
		Expect(err).NotTo(HaveOccurred())

		By("Wait for Running PerconaXtraDB")
		f.EventuallyPerconaXtraDBRunning(perconaxtradb.ObjectMeta).Should(BeTrue())

		By("Checking Row Count of Table")
		f.EventuallyCountRow(perconaxtradb.ObjectMeta, dbName, 0).Should(Equal(3))

	}

	var shouldInsertData = func() {
		// Create and wait for running PerconaXtraDB
		createAndWaitForRunning()

		By("Creating Table")
		f.EventuallyCreateTable(perconaxtradb.ObjectMeta, dbName, 0).Should(BeTrue())

		By("Inserting Row")
		f.EventuallyInsertRow(perconaxtradb.ObjectMeta, dbName, 0, 3).Should(BeTrue())

		By("Checking Row Count of Table")
		f.EventuallyCountRow(perconaxtradb.ObjectMeta, dbName, 0).Should(Equal(3))

		By("Create Secret")
		err := f.CreateSecret(secret)
		Expect(err).NotTo(HaveOccurred())

	}

	var deleteTestResource = func() {
		if perconaxtradb == nil {
			log.Infoln("Skipping cleanup. Reason: PerconaXtraDB object is nil")
			return
		}

		By("Check if perconaxtradb " + perconaxtradb.Name + " exists.")
		px, err := f.GetPerconaXtraDB(perconaxtradb.ObjectMeta)
		if err != nil && kerr.IsNotFound(err) {
			// PerconaXtraDB was not created. Hence, rest of cleanup is not necessary.
			return
		}
		Expect(err).NotTo(HaveOccurred())

		By("Update perconaxtradb to set spec.terminationPolicy = WipeOut")
		_, err = f.PatchPerconaXtraDB(px.ObjectMeta, func(in *api.PerconaXtraDB) *api.PerconaXtraDB {
			in.Spec.TerminationPolicy = api.TerminationPolicyWipeOut
			return in
		})
		Expect(err).NotTo(HaveOccurred())

		By("Delete perconaxtradb")
		err = f.DeletePerconaXtraDB(perconaxtradb.ObjectMeta)
		if err != nil && kerr.IsNotFound(err) {
			// PerconaXtraDB was not created. Hence, rest of cleanup is not necessary.
			return
		}
		Expect(err).NotTo(HaveOccurred())

		By("Wait for perconaxtradb to be deleted")
		f.EventuallyPerconaXtraDB(perconaxtradb.ObjectMeta).Should(BeFalse())

		By("Wait for perconaxtradb resources to be wipedOut")
		f.EventuallyWipedOut(perconaxtradb.ObjectMeta).Should(Succeed())
	}

	var deleteLeftOverStuffs = func() {
		// old PerconaXtraDB are in garbagePerconaXtraDB list. delete their resources.
		for _, p := range garbagePerconaXtraDB.Items {
			*perconaxtradb = p
			deleteTestResource()
		}

		By("Delete left over workloads if exists any")
		f.CleanWorkloadLeftOvers()
	}

	AfterEach(func() {
		// delete resources for current PerconaXtraDB
		deleteTestResource()

		// old PerconaXtraDB are in garbagePerconaXtraDB list. delete their resources.
		for _, pc := range garbagePerconaXtraDB.Items {
			*perconaxtradb = pc
			deleteTestResource()
		}

		By("Delete left over workloads if exists any")
		f.CleanWorkloadLeftOvers()
	})

	JustAfterEach(func() {
		if CurrentGinkgoTestDescription().Failed {
			f.PrintDebugHelpers(perconaxtradb.Name, int(*perconaxtradb.Spec.Replicas))
		}
	})

	Describe("Test", func() {
		Context("General", func() {
			Context("-", func() {
				It("should run successfully", testGeneralBehaviour)
			})
		})

		Context("Initialize", func() {
			Context("With Script", func() {
				var initScriptConfigmap *core.ConfigMap

				BeforeEach(func() {
					initScriptConfigmap, err = f.InitScriptConfigMap()
					Expect(err).ShouldNot(HaveOccurred())
					By("Create init Script ConfigMap: " + initScriptConfigmap.Name + "\n" + initScriptConfigmap.Data["init.sql"])
					Expect(f.CreateConfigMap(initScriptConfigmap)).ShouldNot(HaveOccurred())

					perconaxtradb.Spec.Init = &api.InitSpec{
						ScriptSource: &api.ScriptSourceSpec{
							VolumeSource: core.VolumeSource{
								ConfigMap: &core.ConfigMapVolumeSource{
									LocalObjectReference: core.LocalObjectReference{
										Name: initScriptConfigmap.Name,
									},
								},
							},
						},
					}
				})

				AfterEach(func() {
					By("Deleting configMap: " + initScriptConfigmap.Name)
					Expect(f.DeleteConfigMap(initScriptConfigmap.ObjectMeta)).NotTo(HaveOccurred())
				})

				It("should run successfully", func() {
					// Create PerconaXtraDB
					createAndWaitForRunning()

					By("Checking Row Count of Table")
					f.EventuallyCountRow(perconaxtradb.ObjectMeta, dbName, 0).Should(Equal(3))
				})
			})

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

					By("Deleting RestoreSessionForCluster")
					err = f.DeleteRestoreSession(rs.ObjectMeta)
					Expect(err).NotTo(HaveOccurred())

					By("Deleting Repository")
					err = f.DeleteRepository(repo.ObjectMeta)
					Expect(err).NotTo(HaveOccurred())

					deleteTestResource()
					deleteLeftOverStuffs()
				})

				var createAndWaitForInitializing = func() {
					By("Creating PerconaXtraDB: " + perconaxtradb.Name)
					err = f.CreatePerconaXtraDB(perconaxtradb)
					Expect(err).NotTo(HaveOccurred())

					By("Wait for Initializing perconaxtradb")
					f.EventuallyPerconaXtraDBPhase(perconaxtradb.ObjectMeta).Should(Equal(api.DatabasePhaseInitializing))
				}

				var shouldInitializeFromStash = func() {
					// Create and wait for running MySQL
					createAndWaitForRunning()

					create_Database_N_Table(perconaxtradb.ObjectMeta, 0)
					insertRows(perconaxtradb.ObjectMeta, 0, 3)
					countRows(perconaxtradb.ObjectMeta, 0, 3)

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

					oldPerconaXtraDB, err := f.GetPerconaXtraDB(perconaxtradb.ObjectMeta)
					Expect(err).NotTo(HaveOccurred())

					garbagePerconaXtraDB.Items = append(garbagePerconaXtraDB.Items, *oldPerconaXtraDB)

					By("Create PerconaXtraDB for initializing from stash")
					*perconaxtradb = *f.PerconaXtraDB()
					rs = f.RestoreSessionForStandalone(perconaxtradb.ObjectMeta, oldPerconaXtraDB.ObjectMeta)
					perconaxtradb.Spec.DatabaseSecret = oldPerconaXtraDB.Spec.DatabaseSecret
					perconaxtradb.Spec.Init = &api.InitSpec{
						StashRestoreSession: &core.LocalObjectReference{
							Name: rs.Name,
						},
					}

					// Create and wait for running MySQL
					createAndWaitForInitializing()

					By("Create RestoreSessionForCluster")
					err = f.CreateRestoreSession(rs)
					Expect(err).NotTo(HaveOccurred())

					// eventually restoresession succeeded
					By("Check for Succeeded restoreSession")
					f.EventuallyRestoreSessionPhase(rs.ObjectMeta).Should(Equal(stashV1beta1.RestoreSessionSucceeded))

					By("Wait for Running perconaxtradb")
					f.EventuallyPerconaXtraDBRunning(perconaxtradb.ObjectMeta).Should(BeTrue())

					By("Wait for AppBinding to create")
					f.EventuallyAppBinding(perconaxtradb.ObjectMeta).Should(BeTrue())

					By("Check valid AppBinding Specs")
					err = f.CheckAppBindingSpec(perconaxtradb.ObjectMeta)
					Expect(err).NotTo(HaveOccurred())

					By("Waiting for database to be ready")
					f.EventuallyDatabaseReady(perconaxtradb.ObjectMeta, dbName, 0).Should(BeTrue())

					countRows(perconaxtradb.ObjectMeta, 0, 3)
				}

				Context("From GCS backend", func() {

					BeforeEach(func() {
						secret = f.SecretForGCSBackend()
						secret = f.PatchSecretForRestic(secret)
						bc = f.BackupConfiguration(perconaxtradb.ObjectMeta)
						repo = f.Repository(perconaxtradb.ObjectMeta, secret.Name)

						repo.Spec.Backend = store.Backend{
							GCS: &store.GCSSpec{
								Bucket: os.Getenv(googleBucketNameKey),
								Prefix: fmt.Sprintf("stash/%v/%v", perconaxtradb.Namespace, perconaxtradb.Name),
							},
							StorageSecretName: secret.Name,
						}
					})

					It("should run successfully", shouldInitializeFromStash)
				})
			})
		})

		Context("Resume", func() {
			Context("Super Fast User - Create-Delete-Create-Delete-Create ", func() {
				It("should resume DormantDatabase successfully", func() {
					// Create and wait for running PerconaXtraDB
					createAndWaitForRunning()

					By("Creating Table")
					f.EventuallyCreateTable(perconaxtradb.ObjectMeta, dbName, 0).Should(BeTrue())

					By("Inserting Row")
					f.EventuallyInsertRow(perconaxtradb.ObjectMeta, dbName, 0, 3).Should(BeTrue())

					By("Checking Row Count of Table")
					f.EventuallyCountRow(perconaxtradb.ObjectMeta, dbName, 0).Should(Equal(3))

					By("Delete PerconaXtraDB")
					err = f.DeletePerconaXtraDB(perconaxtradb.ObjectMeta)
					Expect(err).NotTo(HaveOccurred())

					By("Wait for perconaxtradb to be deleted")
					f.EventuallyPerconaXtraDB(perconaxtradb.ObjectMeta).Should(BeFalse())

					// Create PerconaXtraDB object again to resume it
					By("Create PerconaXtraDB: " + perconaxtradb.Name)
					err = f.CreatePerconaXtraDB(perconaxtradb)
					Expect(err).NotTo(HaveOccurred())

					// Delete without caring if DB is resumed
					By("Delete PerconaXtraDB")
					err = f.DeletePerconaXtraDB(perconaxtradb.ObjectMeta)
					Expect(err).NotTo(HaveOccurred())

					By("Wait for PerconaXtraDB to be deleted")
					f.EventuallyPerconaXtraDB(perconaxtradb.ObjectMeta).Should(BeFalse())

					// Create PerconaXtraDB object again to resume it
					By("Create PerconaXtraDB: " + perconaxtradb.Name)
					err = f.CreatePerconaXtraDB(perconaxtradb)
					Expect(err).NotTo(HaveOccurred())

					By("Wait for Running PerconaXtraDB")
					f.EventuallyPerconaXtraDBRunning(perconaxtradb.ObjectMeta).Should(BeTrue())

					By("Checking Row Count of Table")
					f.EventuallyCountRow(perconaxtradb.ObjectMeta, dbName, 0).Should(Equal(3))
				})
			})

			Context("Without Init", func() {
				It("should resume DormantDatabase successfully", func() {
					// Create and wait for running PerconaXtraDB
					createAndWaitForRunning()

					By("Creating Table")
					f.EventuallyCreateTable(perconaxtradb.ObjectMeta, dbName, 0).Should(BeTrue())

					By("Inserting Row")
					f.EventuallyInsertRow(perconaxtradb.ObjectMeta, dbName, 0, 3).Should(BeTrue())

					By("Checking Row Count of Table")
					f.EventuallyCountRow(perconaxtradb.ObjectMeta, dbName, 0).Should(Equal(3))

					By("Delete PerconaXtraDB")
					err = f.DeletePerconaXtraDB(perconaxtradb.ObjectMeta)
					Expect(err).NotTo(HaveOccurred())

					By("Wait for perconaxtradb to be deleted")
					f.EventuallyPerconaXtraDB(perconaxtradb.ObjectMeta).Should(BeFalse())

					// Create PerconaXtraDB object again to resume it
					By("Create PerconaXtraDB: " + perconaxtradb.Name)
					err = f.CreatePerconaXtraDB(perconaxtradb)
					Expect(err).NotTo(HaveOccurred())

					By("Wait for Running PerconaXtraDB")
					f.EventuallyPerconaXtraDBRunning(perconaxtradb.ObjectMeta).Should(BeTrue())

					By("Checking Row Count of Table")
					f.EventuallyCountRow(perconaxtradb.ObjectMeta, dbName, 0).Should(Equal(3))
				})
			})

			Context("with init Script", func() {
				var initScriptConfigmap *core.ConfigMap

				BeforeEach(func() {
					initScriptConfigmap, err = f.InitScriptConfigMap()
					Expect(err).ShouldNot(HaveOccurred())
					By("Create init Script ConfigMap: " + initScriptConfigmap.Name + "\n" + initScriptConfigmap.Data["init.sql"])
					Expect(f.CreateConfigMap(initScriptConfigmap)).ShouldNot(HaveOccurred())

					perconaxtradb.Spec.Init = &api.InitSpec{
						ScriptSource: &api.ScriptSourceSpec{
							VolumeSource: core.VolumeSource{
								ConfigMap: &core.ConfigMapVolumeSource{
									LocalObjectReference: core.LocalObjectReference{
										Name: initScriptConfigmap.Name,
									},
								},
							},
						},
					}
				})

				AfterEach(func() {
					By("Deleting configMap: " + initScriptConfigmap.Name)
					Expect(f.DeleteConfigMap(initScriptConfigmap.ObjectMeta)).NotTo(HaveOccurred())
				})

				It("should resume DormantDatabase successfully", func() {
					// Create and wait for running PerconaXtraDB
					createAndWaitForRunning()

					By("Checking Row Count of Table")
					f.EventuallyCountRow(perconaxtradb.ObjectMeta, dbName, 0).Should(Equal(3))

					By("Delete PerconaXtraDB")
					err = f.DeletePerconaXtraDB(perconaxtradb.ObjectMeta)
					Expect(err).NotTo(HaveOccurred())

					By("Wait for perconaxtradb to be deleted")
					f.EventuallyPerconaXtraDB(perconaxtradb.ObjectMeta).Should(BeFalse())

					// Create PerconaXtraDB object again to resume it
					By("Create PerconaXtraDB: " + perconaxtradb.Name)
					err = f.CreatePerconaXtraDB(perconaxtradb)
					Expect(err).NotTo(HaveOccurred())

					By("Wait for Running PerconaXtraDB")
					f.EventuallyPerconaXtraDBRunning(perconaxtradb.ObjectMeta).Should(BeTrue())

					By("Checking Row Count of Table")
					f.EventuallyCountRow(perconaxtradb.ObjectMeta, dbName, 0).Should(Equal(3))

					perconaxtradb, err := f.GetPerconaXtraDB(perconaxtradb.ObjectMeta)
					Expect(err).NotTo(HaveOccurred())
					Expect(perconaxtradb.Spec.Init).NotTo(BeNil())

					By("Checking PerconaXtraDB crd does not have kubedb.com/initialized annotation")
					_, err = meta_util.GetString(perconaxtradb.Annotations, api.AnnotationInitialized)
					Expect(err).To(HaveOccurred())
				})
			})

			Context("Multiple times with init", func() {
				var initScriptConfigmap *core.ConfigMap

				BeforeEach(func() {
					initScriptConfigmap, err = f.InitScriptConfigMap()
					Expect(err).ShouldNot(HaveOccurred())
					By("Create init Script ConfigMap: " + initScriptConfigmap.Name + "\n" + initScriptConfigmap.Data["init.sql"])
					Expect(f.CreateConfigMap(initScriptConfigmap)).ShouldNot(HaveOccurred())

					perconaxtradb.Spec.Init = &api.InitSpec{
						ScriptSource: &api.ScriptSourceSpec{
							VolumeSource: core.VolumeSource{
								ConfigMap: &core.ConfigMapVolumeSource{
									LocalObjectReference: core.LocalObjectReference{
										Name: initScriptConfigmap.Name,
									},
								},
							},
						},
					}
				})

				AfterEach(func() {
					By("Deleting configMap: " + initScriptConfigmap.Name)
					Expect(f.DeleteConfigMap(initScriptConfigmap.ObjectMeta)).NotTo(HaveOccurred())
				})

				It("should resume DormantDatabase successfully", func() {
					// Create and wait for running PerconaXtraDB
					createAndWaitForRunning()

					By("Checking Row Count of Table")
					f.EventuallyCountRow(perconaxtradb.ObjectMeta, dbName, 0).Should(Equal(3))

					for i := 0; i < 3; i++ {
						By(fmt.Sprintf("%v-th", i+1) + " time running.")

						By("Delete PerconaXtraDB")
						err = f.DeletePerconaXtraDB(perconaxtradb.ObjectMeta)
						Expect(err).NotTo(HaveOccurred())

						By("Wait for perconaxtradb to be deleted")
						f.EventuallyPerconaXtraDB(perconaxtradb.ObjectMeta).Should(BeFalse())

						// Create PerconaXtraDB object again to resume it
						By("Create PerconaXtraDB: " + perconaxtradb.Name)
						err = f.CreatePerconaXtraDB(perconaxtradb)
						Expect(err).NotTo(HaveOccurred())

						By("Wait for Running PerconaXtraDB")
						f.EventuallyPerconaXtraDBRunning(perconaxtradb.ObjectMeta).Should(BeTrue())

						By("Checking Row Count of Table")
						f.EventuallyCountRow(perconaxtradb.ObjectMeta, dbName, 0).Should(Equal(3))

						perconaxtradb, err := f.GetPerconaXtraDB(perconaxtradb.ObjectMeta)
						Expect(err).NotTo(HaveOccurred())
						Expect(perconaxtradb.Spec.Init).ShouldNot(BeNil())

						By("Checking PerconaXtraDB crd does not have kubedb.com/initialized annotation")
						_, err = meta_util.GetString(perconaxtradb.Annotations, api.AnnotationInitialized)
						Expect(err).To(HaveOccurred())
					}
				})
			})
		})

		Context("Termination Policy", func() {

			BeforeEach(func() {
				//skipDataChecking = false
				secret = f.SecretForGCSBackend()
				//snapshot.Spec.StorageSecretName = secret.Name
				//snapshot.Spec.GCS = &store.GCSSpec{
				//	Bucket: os.Getenv(GCS_BUCKET_NAME),
				//}
				//snapshot.Spec.DatabaseName = perconaxtradb.Name
			})

			Context("with TerminationDoNotTerminate", func() {
				BeforeEach(func() {
					//skipDataChecking = true
					perconaxtradb.Spec.TerminationPolicy = api.TerminationPolicyDoNotTerminate
				})

				It("should work successfully", func() {
					// Create and wait for running PerconaXtraDB
					createAndWaitForRunning()

					By("Delete PerconaXtraDB")
					err = f.DeletePerconaXtraDB(perconaxtradb.ObjectMeta)
					Expect(err).Should(HaveOccurred())

					By("PerconaXtraDB is not paused. Check for PerconaXtraDB")
					f.EventuallyPerconaXtraDB(perconaxtradb.ObjectMeta).Should(BeTrue())

					By("Check for Running PerconaXtraDB")
					f.EventuallyPerconaXtraDBRunning(perconaxtradb.ObjectMeta).Should(BeTrue())

					By("Update PerconaXtraDB to set spec.terminationPolicy = Pause")
					_, err := f.PatchPerconaXtraDB(perconaxtradb.ObjectMeta, func(in *api.PerconaXtraDB) *api.PerconaXtraDB {
						in.Spec.TerminationPolicy = api.TerminationPolicyHalt
						return in
					})
					Expect(err).NotTo(HaveOccurred())
				})
			})

			Context("with TerminationPolicyHalt (default)", func() {

				AfterEach(func() {
					By("Deleting secret: " + secret.Name)
					err := f.DeleteSecret(secret.ObjectMeta)
					if err != nil && !kerr.IsNotFound(err) {
						Expect(err).NotTo(HaveOccurred())
					}
				})

				It("should create DormantDatabase and resume from it", func() {
					// Run PerconaXtraDB and take snapshot
					shouldInsertData()

					By("Deleting PerconaXtraDB crd")
					err = f.DeletePerconaXtraDB(perconaxtradb.ObjectMeta)
					Expect(err).NotTo(HaveOccurred())

					By("Wait for perconaxtradb to be deleted")
					f.EventuallyPerconaXtraDB(perconaxtradb.ObjectMeta).Should(BeFalse())

					By("Checking PVC hasn't been deleted")
					f.EventuallyPVCCount(perconaxtradb.ObjectMeta).Should(Equal(1))

					By("Checking Secret hasn't been deleted")
					f.EventuallyDBSecretCount(perconaxtradb.ObjectMeta).Should(Equal(1))

					// Create PerconaXtraDB object again to resume it
					By("Create (resume) PerconaXtraDB: " + perconaxtradb.Name)
					err = f.CreatePerconaXtraDB(perconaxtradb)
					Expect(err).NotTo(HaveOccurred())

					By("Wait for Running PerconaXtraDB")
					f.EventuallyPerconaXtraDBRunning(perconaxtradb.ObjectMeta).Should(BeTrue())

					By("Checking row count of table")
					f.EventuallyCountRow(perconaxtradb.ObjectMeta, dbName, 0).Should(Equal(3))
				})
			})

			Context("with TerminationPolicyDelete", func() {

				BeforeEach(func() {
					perconaxtradb.Spec.TerminationPolicy = api.TerminationPolicyDelete
				})

				AfterEach(func() {
					By("Deleting secret: " + secret.Name)
					err := f.DeleteSecret(secret.ObjectMeta)
					if err != nil && !kerr.IsNotFound(err) {
						Expect(err).NotTo(HaveOccurred())
					}
				})

				It("should not create DormantDatabase and should not delete secret", func() {
					// Run PerconaXtraDB and take snapshot
					shouldInsertData()

					By("Delete PerconaXtraDB")
					err = f.DeletePerconaXtraDB(perconaxtradb.ObjectMeta)
					Expect(err).NotTo(HaveOccurred())

					By("wait until PerconaXtraDB is deleted")
					f.EventuallyPerconaXtraDB(perconaxtradb.ObjectMeta).Should(BeFalse())

					By("Checking PVC has been deleted")
					f.EventuallyPVCCount(perconaxtradb.ObjectMeta).Should(Equal(0))

					By("Checking Secret hasn't been deleted")
					f.EventuallyDBSecretCount(perconaxtradb.ObjectMeta).Should(Equal(1))
				})
			})

			Context("with TerminationPolicyWipeOut", func() {

				BeforeEach(func() {
					perconaxtradb.Spec.TerminationPolicy = api.TerminationPolicyWipeOut
				})

				It("should not create DormantDatabase and should wipeOut all", func() {
					// Run PerconaXtraDB and take snapshot
					shouldInsertData()

					By("Delete PerconaXtraDB")
					err = f.DeletePerconaXtraDB(perconaxtradb.ObjectMeta)
					Expect(err).NotTo(HaveOccurred())

					By("wait until PerconaXtraDB is deleted")
					f.EventuallyPerconaXtraDB(perconaxtradb.ObjectMeta).Should(BeFalse())

					By("Checking PVCs has been deleted")
					f.EventuallyPVCCount(perconaxtradb.ObjectMeta).Should(Equal(0))
				})
			})
		})

		Context("EnvVars", func() {
			Context("Database Name as EnvVar", func() {
				It("should create DB with name provided in EvnVar", func() {
					if skipMessage != "" {
						Skip(skipMessage)
					}

					dbName = f.App()
					perconaxtradb.Spec.PodTemplate.Spec.Env = []core.EnvVar{
						{
							Name:  MYSQL_DATABASE,
							Value: dbName,
						},
					}
					//test general behaviour
					testGeneralBehaviour()
				})
			})

			Context("Root Password as EnvVar", func() {
				It("should reject to create PerconaXtraDB CRD", func() {
					if skipMessage != "" {
						Skip(skipMessage)
					}

					perconaxtradb.Spec.PodTemplate.Spec.Env = []core.EnvVar{
						{
							Name:  MYSQL_ROOT_PASSWORD,
							Value: "not@secret",
						},
					}
					By("Create PerconaXtraDB: " + perconaxtradb.Name)
					err = f.CreatePerconaXtraDB(perconaxtradb)
					Expect(err).To(HaveOccurred())
				})
			})

			Context("Update EnvVar", func() {
				It("should not reject to update EvnVar", func() {
					if skipMessage != "" {
						Skip(skipMessage)
					}

					dbName = f.App()
					perconaxtradb.Spec.PodTemplate.Spec.Env = []core.EnvVar{
						{
							Name:  MYSQL_DATABASE,
							Value: dbName,
						},
					}
					//test general behaviour
					testGeneralBehaviour()

					By("Patching EnvVar")
					_, _, err = util.PatchPerconaXtraDB(f.ExtClient().KubedbV1alpha1(), perconaxtradb, func(in *api.PerconaXtraDB) *api.PerconaXtraDB {
						in.Spec.PodTemplate.Spec.Env = []core.EnvVar{
							{
								Name:  MYSQL_DATABASE,
								Value: "patched-db",
							},
						}
						return in
					})
					Expect(err).NotTo(HaveOccurred())
				})
			})
		})

		Context("Custom config", func() {

			customConfigs := []string{
				"max_connections=200",
				"read_buffer_size=1048576", // 1MB
			}

			Context("from configMap", func() {
				var userConfig *core.ConfigMap

				BeforeEach(func() {
					userConfig = f.GetCustomConfig(customConfigs)
				})

				AfterEach(func() {
					By("Deleting configMap: " + userConfig.Name)
					err := f.DeleteConfigMap(userConfig.ObjectMeta)
					Expect(err).NotTo(HaveOccurred())

				})

				It("should set configuration provided in configMap", func() {
					if skipMessage != "" {
						Skip(skipMessage)
					}

					By("Creating configMap: " + userConfig.Name)
					err := f.CreateConfigMap(userConfig)
					Expect(err).NotTo(HaveOccurred())

					perconaxtradb.Spec.ConfigSource = &core.VolumeSource{
						ConfigMap: &core.ConfigMapVolumeSource{
							LocalObjectReference: core.LocalObjectReference{
								Name: userConfig.Name,
							},
						},
					}

					// Create PerconaXtraDB
					createAndWaitForRunning()

					By("Checking PerconaXtraDB configured from provided custom configuration")
					for _, cfg := range customConfigs {
						f.EventuallyPerconaXtraDBVariable(perconaxtradb.ObjectMeta, dbName, 0, cfg).Should(matcher.UseCustomConfig(cfg))
					}
				})
			})
		})

		Context("StorageType ", func() {
			var shouldRunSuccessfully = func() {
				if skipMessage != "" {
					Skip(skipMessage)
				}

				// Create PerconaXtraDB
				createAndWaitForRunning()

				By("Creating Table")
				f.EventuallyCreateTable(perconaxtradb.ObjectMeta, dbName, 0).Should(BeTrue())

				By("Inserting Rows")
				f.EventuallyInsertRow(perconaxtradb.ObjectMeta, dbName, 0, 3).Should(BeTrue())

				By("Checking Row Count of Table")
				f.EventuallyCountRow(perconaxtradb.ObjectMeta, dbName, 0).Should(Equal(3))
			}

			Context("Ephemeral", func() {
				Context("General Behaviour", func() {
					BeforeEach(func() {
						perconaxtradb.Spec.StorageType = api.StorageTypeEphemeral
						perconaxtradb.Spec.Storage = nil
						perconaxtradb.Spec.TerminationPolicy = api.TerminationPolicyWipeOut
					})

					It("should run successfully", shouldRunSuccessfully)
				})

				Context("With TerminationPolicyHalt", func() {
					BeforeEach(func() {
						perconaxtradb.Spec.StorageType = api.StorageTypeEphemeral
						perconaxtradb.Spec.Storage = nil
						perconaxtradb.Spec.TerminationPolicy = api.TerminationPolicyHalt
					})

					It("should reject to create PerconaXtraDB object", func() {

						By("Creating PerconaXtraDB: " + perconaxtradb.Name)
						err := f.CreatePerconaXtraDB(perconaxtradb)
						Expect(err).To(HaveOccurred())
					})
				})
			})
		})
	})
})
