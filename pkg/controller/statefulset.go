package controller

import (
	"fmt"
	"strings"

	"github.com/appscode/go/log"
	"github.com/appscode/go/types"
	"github.com/fatih/structs"
	api "github.com/kubedb/apimachinery/apis/kubedb/v1alpha1"
	"github.com/kubedb/apimachinery/pkg/eventer"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientsetscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/reference"
	kutil "kmodules.xyz/client-go"
	app_util "kmodules.xyz/client-go/apps/v1"
	core_util "kmodules.xyz/client-go/core/v1"
	mona "kmodules.xyz/monitoring-agent-api/api/v1"
)

func (c *Controller) ensureStatefulSet(pxc *api.Percona) (kutil.VerbType, error) {
	if err := c.checkStatefulSet(pxc); err != nil {
		return kutil.VerbUnchanged, err
	}

	// Create statefulSet for Percona
	statefulSet, vt, err := c.createStatefulSet(pxc)
	if err != nil {
		return kutil.VerbUnchanged, err
	}

	// Check StatefulSet Pod status
	if vt != kutil.VerbUnchanged {
		if err := c.checkStatefulSetPodStatus(statefulSet); err != nil {
			return kutil.VerbUnchanged, err
		}
		c.recorder.Eventf(
			pxc,
			core.EventTypeNormal,
			eventer.EventReasonSuccessful,
			"Successfully %v StatefulSet",
			vt,
		)
	}
	return vt, nil
}

func (c *Controller) checkStatefulSet(pxc *api.Percona) error {
	// SatatefulSet for Percona
	statefulSet, err := c.Client.AppsV1().StatefulSets(pxc.Namespace).Get(pxc.OffshootName(), metav1.GetOptions{})
	if err != nil {
		if kerr.IsNotFound(err) {
			return nil
		}
		return err
	}

	if statefulSet.Labels[api.LabelDatabaseKind] != api.ResourceKindPercona ||
		statefulSet.Labels[api.LabelDatabaseName] != pxc.Name {
		return fmt.Errorf(`intended statefulSet "%v/%v" already exists`, pxc.Namespace, pxc.OffshootName())
	}

	return nil
}

func (c *Controller) createStatefulSet(pxc *api.Percona) (*apps.StatefulSet, kutil.VerbType, error) {
	statefulSetMeta := metav1.ObjectMeta{
		Name:      pxc.OffshootName(),
		Namespace: pxc.Namespace,
	}

	ref, rerr := reference.GetReference(clientsetscheme.Scheme, pxc)
	if rerr != nil {
		return nil, kutil.VerbUnchanged, rerr
	}

	pxcVersion, err := c.ExtClient.CatalogV1alpha1().PerconaVersions().Get(string(pxc.Spec.Version), metav1.GetOptions{})
	if err != nil {
		return nil, kutil.VerbUnchanged, rerr
	}

	return app_util.CreateOrPatchStatefulSet(c.Client, statefulSetMeta, func(in *apps.StatefulSet) *apps.StatefulSet {
		in.Labels = pxc.OffshootLabels()
		in.Annotations = pxc.Spec.PodTemplate.Controller.Annotations
		core_util.EnsureOwnerReference(&in.ObjectMeta, ref)

		in.Spec.Replicas = pxc.Spec.Replicas
		in.Spec.ServiceName = c.GoverningService
		in.Spec.Selector = &metav1.LabelSelector{
			MatchLabels: pxc.OffshootSelectors(),
		}
		in.Spec.Template.Labels = pxc.OffshootSelectors()
		in.Spec.Template.Annotations = pxc.Spec.PodTemplate.Annotations
		in.Spec.Template.Spec.InitContainers = core_util.UpsertContainers(
			in.Spec.Template.Spec.InitContainers,
			append(
				[]core.Container{
					{
						Name:            "remove-lost-found",
						Image:           pxcVersion.Spec.InitContainer.Image,
						ImagePullPolicy: core.PullIfNotPresent,
						Command: []string{
							"rm",
							"-rf",
							"/var/lib/mysql/lost+found",
						},
						VolumeMounts: []core.VolumeMount{
							{
								Name:      "data",
								MountPath: "/var/lib/mysql",
							},
						},
						Resources: pxc.Spec.PodTemplate.Spec.Resources,
					},
				},
				pxc.Spec.PodTemplate.Spec.InitContainers...,
			),
		)

		//entryPointArgs := strings.Join(pxc.Spec.PodTemplate.Spec.Args, " ")
		container := core.Container{
			Name:            api.ResourceSingularPercona,
			Image:           pxcVersion.Spec.DB.Image,
			ImagePullPolicy: core.PullIfNotPresent,
			Args:            pxc.Spec.PodTemplate.Spec.Args,
			Resources:       pxc.Spec.PodTemplate.Spec.Resources,
			LivenessProbe:   pxc.Spec.PodTemplate.Spec.LivenessProbe,
			ReadinessProbe:  pxc.Spec.PodTemplate.Spec.ReadinessProbe,
			Lifecycle:       pxc.Spec.PodTemplate.Spec.Lifecycle,
			Ports: []core.ContainerPort{
				{
					Name:          "db",
					ContainerPort: api.MySQLNodePort,
					Protocol:      core.ProtocolTCP,
				},
				//{
				//	Name:          "com1",
				//	ContainerPort: 4567,
				//},
				//{
				//	Name:          "com2",
				//	ContainerPort: 4568,
				//},
			},
			//
			//			Command: []string{
			//				"/bin/bash", "-c",
			//				fmt.Sprintf(`
			//index=$(cat /etc/hostname | grep -o '[^-]*$')
			//NAMESPACE=$(cat /var/run/secrets/kubernetes.io/serviceaccount/namespace)
			//
			//CLUSTER_JOIN=
			//if [[ "${index}" -gt "0" ]]; then
			//  CLUSTER_JOIN=%[1]s-0.%[2]s.$NAMESPACE.svc.cluster.local
			//
			//  while true; do
			//    out=$(ping $CLUSTER_JOIN -c 1 2>/dev/null)
			//    if [[ -n "$out" ]]; then
			//      break
			//    fi
			//
			//    sleep 1
			//  done
			//
			//  for i in {60..0}; do
			//    echo .
			//    sleep 1
			//  done
			//fi
			//
			//# export CLUSTER_NAME=%[3]s
			//export CLUSTER_JOIN
			//
			//echo
			//echo "================================="
			//echo "CLUSTER_JOIN=${CLUSTER_JOIN}"
			//echo "CLUSTER_NAME=${CLUSTER_NAME}"
			//echo "================================="
			//echo
			//
			//echo
			//
			//. /entrypoint.sh
			//# . /entrypoint.sh --wsrep_node_address=%[1]s-${index}.%[2]s.$NAMESPACE
			//# . /entrypoint.sh --wsrep_node_address=%[1]s-${index}.%[2]s.$NAMESPACE.svc.cluster.local --bind-address=0.0.0.0
			//`, pxc.Name, c.GoverningService),
			//			},
		}

		if pxc.Spec.PXC != nil {
			container.Command = []string{
				"peer-finder",
			}
			userProvidedArgs := strings.Join(pxc.Spec.PodTemplate.Spec.Args, " ")
			container.Args = []string{
				fmt.Sprintf("-service=%s", c.GoverningService),
				fmt.Sprintf("-on-start=/on-start.sh %s", userProvidedArgs),
			}
			container.Ports = append(container.Ports, []core.ContainerPort{
				{
					Name:          "com1",
					ContainerPort: 4567,
				},
				{
					Name:          "com2",
					ContainerPort: 4568,
				},
			}...)
		}
		if container.LivenessProbe != nil && structs.IsZero(*container.LivenessProbe) {
			container.LivenessProbe = nil
		}
		if container.ReadinessProbe != nil && structs.IsZero(*container.ReadinessProbe) {
			container.ReadinessProbe = nil
		}

		in.Spec.Template.Spec.Containers = core_util.UpsertContainer(in.Spec.Template.Spec.Containers, container)

		if pxc.GetMonitoringVendor() == mona.VendorPrometheus {
			in.Spec.Template.Spec.Containers = core_util.UpsertContainer(in.Spec.Template.Spec.Containers, core.Container{
				Name: "exporter",
				Command: []string{
					"/bin/sh",
				},
				Args: []string{
					"-c",
					// DATA_SOURCE_NAME=user:password@tcp(localhost:5555)/dbname
					// ref: https://github.com/prometheus/mysqld_exporter#setting-the-mysql-servers-data-source-name
					fmt.Sprintf(`export DATA_SOURCE_NAME="${MYSQL_ROOT_USERNAME:-}:${MYSQL_ROOT_PASSWORD:-}@(127.0.0.1:3306)/"
						/bin/mysqld_exporter --web.listen-address=:%v --web.telemetry-path=%v %v`,
						pxc.Spec.Monitor.Prometheus.Port, pxc.StatsService().Path(), strings.Join(pxc.Spec.Monitor.Args, " ")),
				},
				Image: pxcVersion.Spec.Exporter.Image,
				Ports: []core.ContainerPort{
					{
						Name:          api.PrometheusExporterPortName,
						Protocol:      core.ProtocolTCP,
						ContainerPort: pxc.Spec.Monitor.Prometheus.Port,
					},
				},
				Env:             pxc.Spec.Monitor.Env,
				Resources:       pxc.Spec.Monitor.Resources,
				SecurityContext: pxc.Spec.Monitor.SecurityContext,
			})
		}
		// Set Admin Secret as MYSQL_ROOT_PASSWORD env variable
		in = upsertEnv(in, pxc)
		in = upsertDataVolume(in, pxc)
		in = upsertCustomConfig(in, pxc)

		if pxc.Spec.Init != nil && pxc.Spec.Init.ScriptSource != nil {
			in = upsertInitScript(in, pxc.Spec.Init.ScriptSource.VolumeSource)
		}

		in.Spec.Template.Spec.NodeSelector = pxc.Spec.PodTemplate.Spec.NodeSelector
		in.Spec.Template.Spec.Affinity = pxc.Spec.PodTemplate.Spec.Affinity
		if pxc.Spec.PodTemplate.Spec.SchedulerName != "" {
			in.Spec.Template.Spec.SchedulerName = pxc.Spec.PodTemplate.Spec.SchedulerName
		}
		in.Spec.Template.Spec.Tolerations = pxc.Spec.PodTemplate.Spec.Tolerations
		in.Spec.Template.Spec.ImagePullSecrets = pxc.Spec.PodTemplate.Spec.ImagePullSecrets
		in.Spec.Template.Spec.PriorityClassName = pxc.Spec.PodTemplate.Spec.PriorityClassName
		in.Spec.Template.Spec.Priority = pxc.Spec.PodTemplate.Spec.Priority
		in.Spec.Template.Spec.SecurityContext = pxc.Spec.PodTemplate.Spec.SecurityContext
		//in.Spec.Template.Spec.SecurityContext = pxc.Spec.PodTemplate.Spec.SecurityContext

		if c.EnableRBAC {
			in.Spec.Template.Spec.ServiceAccountName = pxc.OffshootName()
		}

		in.Spec.UpdateStrategy = pxc.Spec.UpdateStrategy
		in = upsertUserEnv(in, pxc)

		return in
	})
}

func upsertDataVolume(statefulSet *apps.StatefulSet, pxc *api.Percona) *apps.StatefulSet {
	for i, container := range statefulSet.Spec.Template.Spec.Containers {
		if container.Name == api.ResourceSingularPercona {
			volumeMount := core.VolumeMount{
				Name:      "data",
				MountPath: "/var/lib/mysql",
			}
			volumeMounts := container.VolumeMounts
			volumeMounts = core_util.UpsertVolumeMount(volumeMounts, volumeMount)
			statefulSet.Spec.Template.Spec.Containers[i].VolumeMounts = volumeMounts

			pvcSpec := pxc.Spec.Storage
			if pxc.Spec.StorageType == api.StorageTypeEphemeral {
				ed := core.EmptyDirVolumeSource{}
				if pvcSpec != nil {
					if sz, found := pvcSpec.Resources.Requests[core.ResourceStorage]; found {
						ed.SizeLimit = &sz
					}
				}
				statefulSet.Spec.Template.Spec.Volumes = core_util.UpsertVolume(
					statefulSet.Spec.Template.Spec.Volumes,
					core.Volume{
						Name: "data",
						VolumeSource: core.VolumeSource{
							EmptyDir: &ed,
						},
					})
			} else {
				if len(pvcSpec.AccessModes) == 0 {
					pvcSpec.AccessModes = []core.PersistentVolumeAccessMode{
						core.ReadWriteOnce,
					}
					log.Infof(`Using "%v" as AccessModes in percona.Spec.Storage`, core.ReadWriteOnce)
				}

				claim := core.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name: "data",
					},
					Spec: *pvcSpec,
				}
				if pvcSpec.StorageClassName != nil {
					claim.Annotations = map[string]string{
						"volume.beta.kubernetes.io/storage-class": *pvcSpec.StorageClassName,
					}
				}
				statefulSet.Spec.VolumeClaimTemplates = core_util.UpsertVolumeClaim(statefulSet.Spec.VolumeClaimTemplates, claim)
			}
			break
		}
	}
	return statefulSet
}

func upsertEnv(statefulSet *apps.StatefulSet, pxc *api.Percona) *apps.StatefulSet {
	for i, container := range statefulSet.Spec.Template.Spec.Containers {
		if container.Name == api.ResourceSingularPercona || container.Name == "exporter" {
			envs := []core.EnvVar{
				{
					Name: "MYSQL_ROOT_PASSWORD",
					ValueFrom: &core.EnvVarSource{
						SecretKeyRef: &core.SecretKeySelector{
							LocalObjectReference: core.LocalObjectReference{
								Name: pxc.Spec.DatabaseSecret.SecretName,
							},
							Key: KeyPerconaPassword,
						},
					},
				},
				{
					Name: "MYSQL_ROOT_USERNAME",
					ValueFrom: &core.EnvVarSource{
						SecretKeyRef: &core.SecretKeySelector{
							LocalObjectReference: core.LocalObjectReference{
								Name: pxc.Spec.DatabaseSecret.SecretName,
							},
							Key: KeyPerconaUser,
						},
					},
				},
			}

			if pxc.Spec.PXC != nil {
				envs = append(envs, core.EnvVar{
					Name:  "CLUSTER_NAME",
					Value: pxc.Spec.PXC.ClusterName,
				})
				//envs = append(envs, core.EnvVar{
				//	Name:  "GOV_SVC",
				//	Value: statefulSet.Spec.ServiceName,
				//})
			}

			statefulSet.Spec.Template.Spec.Containers[i].Env = core_util.UpsertEnvVars(container.Env, envs...)
		}
	}

	return statefulSet
}

// upsertUserEnv add/overwrite env from user provided env in crd spec
func upsertUserEnv(statefulSet *apps.StatefulSet, pxc *api.Percona) *apps.StatefulSet {
	for i, container := range statefulSet.Spec.Template.Spec.Containers {
		if container.Name == api.ResourceSingularPercona {
			statefulSet.Spec.Template.Spec.Containers[i].Env = core_util.UpsertEnvVars(container.Env, pxc.Spec.PodTemplate.Spec.Env...)
			return statefulSet
		}
	}
	return statefulSet
}

func upsertInitScript(statefulSet *apps.StatefulSet, script core.VolumeSource) *apps.StatefulSet {
	for i, container := range statefulSet.Spec.Template.Spec.Containers {
		if container.Name == api.ResourceSingularPercona {
			volumeMount := core.VolumeMount{
				Name:      "initial-script",
				MountPath: "/docker-entrypoint-initdb.d",
			}
			statefulSet.Spec.Template.Spec.Containers[i].VolumeMounts = core_util.UpsertVolumeMount(
				container.VolumeMounts,
				volumeMount,
			)

			volume := core.Volume{
				Name:         "initial-script",
				VolumeSource: script,
			}
			statefulSet.Spec.Template.Spec.Volumes = core_util.UpsertVolume(
				statefulSet.Spec.Template.Spec.Volumes,
				volume,
			)
			return statefulSet
		}
	}
	return statefulSet
}

func (c *Controller) checkStatefulSetPodStatus(statefulSet *apps.StatefulSet) error {
	err := core_util.WaitUntilPodRunningBySelector(
		c.Client,
		statefulSet.Namespace,
		statefulSet.Spec.Selector,
		int(types.Int32(statefulSet.Spec.Replicas)),
	)
	if err != nil {
		return err
	}
	return nil
}

func upsertCustomConfig(statefulSet *apps.StatefulSet, pxc *api.Percona) *apps.StatefulSet {
	if pxc.Spec.ConfigSource != nil {
		for i, container := range statefulSet.Spec.Template.Spec.Containers {
			if container.Name == api.ResourceSingularPercona {
				configVolumeMount := core.VolumeMount{
					Name:      "custom-config",
					MountPath: "/etc/mysql/conf.d",
				}
				volumeMounts := container.VolumeMounts
				volumeMounts = core_util.UpsertVolumeMount(volumeMounts, configVolumeMount)
				statefulSet.Spec.Template.Spec.Containers[i].VolumeMounts = volumeMounts

				configVolume := core.Volume{
					Name:         "custom-config",
					VolumeSource: *pxc.Spec.ConfigSource,
				}

				volumes := statefulSet.Spec.Template.Spec.Volumes
				volumes = core_util.UpsertVolume(volumes, configVolume)
				statefulSet.Spec.Template.Spec.Volumes = volumes
				break
			}
		}
	}
	return statefulSet
}
