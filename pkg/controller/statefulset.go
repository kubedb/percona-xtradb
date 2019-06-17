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
	ofst "kmodules.xyz/offshoot-api/api/v1"
)

type workloadOptions struct {
	// App level options
	stsName   string
	labels    map[string]string
	selectors map[string]string

	// db container options
	conatainerName string
	image          string
	cmd            []string // cmd of `percona` container
	args           []string // args of `percona` container
	ports          []core.ContainerPort
	envList        []core.EnvVar // envList of `percona` container
	volumeMount    []core.VolumeMount
	configSource   *core.VolumeSource

	// monitor container
	monitorContainer *core.Container

	// pod Template level options
	replicas       *int32
	gvrSvcName     string
	podTemplate    *ofst.PodTemplateSpec
	pvcSpec        *core.PersistentVolumeClaimSpec
	initContainers []core.Container
	volume         []core.Volume // volumes to mount on stsPodTemplate
}

func (c *Controller) ensurePerconaXtraDBNode(pxc *api.Percona) (kutil.VerbType, error) {
	if pxc.Spec.PXC != nil {
		vt1, err := c.ensurePerconaXtraDB(pxc)
		if err != nil {
			return vt1, err
		}

		vt2, err := c.ensureProxysql(pxc)
		if err != nil {
			return vt2, err
		}

		if vt1 == kutil.VerbCreated && vt2 == kutil.VerbCreated {
			return kutil.VerbCreated, nil
		} else if vt1 != kutil.VerbUnchanged || vt2 != kutil.VerbUnchanged {
			return kutil.VerbPatched, nil
		}
	}

	return kutil.VerbUnchanged, nil
}

func (c *Controller) ensurePerconaXtraDB(pxc *api.Percona) (kutil.VerbType, error) {
	pxcVersion, err := c.ExtClient.CatalogV1alpha1().PerconaVersions().Get(string(pxc.Spec.Version), metav1.GetOptions{})
	if err != nil {
		return kutil.VerbUnchanged, err
	}

	initContainers := append([]core.Container{
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
					Name: "data",
					MountPath: api.PerconaDataMountPath,
				},
			},
			Resources: pxc.Spec.PodTemplate.Spec.Resources,
		},
	})

	var cmds, args []string
	var ports = []core.ContainerPort{
		{
			Name:          "mysql",
			ContainerPort: api.MySQLNodePort,
			Protocol:      core.ProtocolTCP,
		},
	}
	if pxc.Spec.PXC != nil {
		cmds = []string{
			"peer-finder",
		}
		userProvidedArgs := strings.Join(pxc.Spec.PodTemplate.Spec.Args, " ")
		args = []string{
			fmt.Sprintf("-service=%s", c.GoverningService),
			fmt.Sprintf("-on-start=/on-start.sh %s", userProvidedArgs),
		}
		ports = append(ports, []core.ContainerPort{
			{
				Name:          "sst",
				ContainerPort: 4567,
			},
			{
				Name:          "replication",
				ContainerPort: 4568,
			},
		}...)
	}

	var volumes []core.Volume
	var volumeMounts []core.VolumeMount

	if pxc.Spec.Init != nil && pxc.Spec.Init.ScriptSource != nil {
		volumes = append(volumes, core.Volume{
			Name:         "initial-script",
			VolumeSource: pxc.Spec.Init.ScriptSource.VolumeSource,
		})
		volumeMounts = append(volumeMounts, core.VolumeMount{
			Name: "initial-script",
			MountPath: api.PerconaInitDBMountPath,
		})
	}
	pxc.Spec.PodTemplate.Spec.ServiceAccountName = pxc.OffshootName()

	var envList []core.EnvVar
	if pxc.Spec.PXC != nil {
		envList = []core.EnvVar{
			{
				Name: "MYSQL_USER",
				ValueFrom: &core.EnvVarSource{
					SecretKeyRef: &core.SecretKeySelector{
						LocalObjectReference: core.LocalObjectReference{
							Name: pxc.Spec.DatabaseSecret.SecretName,
						},
						Key: api.ProxysqlUser,
					},
				},
			},
			{
				Name: "MYSQL_PASSWORD",
				ValueFrom: &core.EnvVarSource{
					SecretKeyRef: &core.SecretKeySelector{
						LocalObjectReference: core.LocalObjectReference{
							Name: pxc.Spec.DatabaseSecret.SecretName,
						},
						Key: api.ProxysqlPassword,
					},
				},
			},
			{
				Name:  "CLUSTER_NAME",
				Value: pxc.Spec.PXC.ClusterName,
			},
		}
	}

	var monitorContainer core.Container
	if pxc.GetMonitoringVendor() == mona.VendorPrometheus {
		monitorContainer = core.Container{
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
		}
	}

	opts := workloadOptions{
		stsName:          pxc.OffshootName(),
		labels:           pxc.XtraDBLabels(),
		selectors:        pxc.XtraDBSelectors(),
		conatainerName:   api.ResourceSingularPercona,
		image:            pxcVersion.Spec.DB.Image,
		args:             args,
		cmd:              cmds,
		ports:            ports,
		envList:          envList,
		initContainers:   initContainers,
		gvrSvcName:       c.GoverningService,
		podTemplate:      &pxc.Spec.PodTemplate,
		configSource:     pxc.Spec.ConfigSource,
		pvcSpec:          pxc.Spec.Storage,
		replicas:         pxc.Spec.Replicas,
		volume:           volumes,
		volumeMount:      volumeMounts,
		monitorContainer: &monitorContainer,
	}

	return c.ensureStatefulSet(pxc, pxc.Spec.UpdateStrategy, opts)
}

func (c *Controller) ensureProxysql(pxc *api.Percona) (kutil.VerbType, error) {
	pxcVersion, err := c.ExtClient.CatalogV1alpha1().PerconaVersions().Get(string(pxc.Spec.Version), metav1.GetOptions{})
	if err != nil {
		return kutil.VerbUnchanged, err
	}

	var ports = []core.ContainerPort{
		{
			Name:          "mysql",
			ContainerPort: api.ProxysqlMySQLNodePort,
			Protocol:      core.ProtocolTCP,
		},
		{
			Name:          api.ProxysqlAdminPortName,
			ContainerPort: api.ProxysqlAdminPort,
			Protocol:      core.ProtocolTCP,
		},
	}

	var volumes []core.Volume
	var volumeMounts []core.VolumeMount

	volumeMounts = append(volumeMounts, core.VolumeMount{
		Name: "data",
		MountPath: api.ProxysqlDataMountPath,
	})
	volumes = append(volumes, core.Volume{
		Name: "data",
		VolumeSource: core.VolumeSource{
			EmptyDir: &core.EmptyDirVolumeSource{},
		},
	})

	pxc.Spec.PodTemplate.Spec.ServiceAccountName = pxc.OffshootName()
	proxysqlServiceName, err := c.createProxysqlService(pxc)
	if err != nil {
		return kutil.VerbUnchanged, err
	}

	var envList []core.EnvVar
	var peers []string
	for i := 0; i < int(*pxc.Spec.Replicas); i += 1 {
		peers = append(peers, pxc.PeerName(i))
	}
	envList = append(envList, []core.EnvVar{
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
			Name: "MYSQL_PROXY_USER",
			ValueFrom: &core.EnvVarSource{
				SecretKeyRef: &core.SecretKeySelector{
					LocalObjectReference: core.LocalObjectReference{
						Name: pxc.Spec.DatabaseSecret.SecretName,
					},
					Key: api.ProxysqlUser,
				},
			},
		},
		{
			Name: "MYSQL_PROXY_PASSWORD",
			ValueFrom: &core.EnvVarSource{
				SecretKeyRef: &core.SecretKeySelector{
					LocalObjectReference: core.LocalObjectReference{
						Name: pxc.Spec.DatabaseSecret.SecretName,
					},
					Key: api.ProxysqlPassword,
				},
			},
		},
		{
			Name:  "PEERS",
			Value: strings.Join(peers, ","),
		},
	}...)

	opts := workloadOptions{
		stsName:        pxc.ProxysqlName(),
		labels:         pxc.ProxysqlLabels(),
		selectors:      pxc.ProxysqlSelectors(),
		conatainerName: "proxysql",
		image:          pxcVersion.Spec.Proxysql.Image,
		args:           nil,
		cmd:            nil,
		ports:          ports,
		envList:        envList,
		initContainers: nil,
		gvrSvcName:     proxysqlServiceName,
		podTemplate:    &pxc.Spec.PXC.Proxysql.PodTemplate,
		configSource:   nil,
		pvcSpec:        nil,
		replicas:       pxc.Spec.PXC.Proxysql.Replicas,
		volume:         volumes,
		volumeMount:    volumeMounts,
	}

	return c.ensureStatefulSet(pxc, pxc.Spec.UpdateStrategy, opts)
}

func (c *Controller) checkStatefulSet(pxc *api.Percona, stsName string) error {
	// StatefulSet for MongoDB database
	statefulSet, err := c.Client.AppsV1().StatefulSets(pxc.Namespace).Get(stsName, metav1.GetOptions{})
	if err != nil {
		if kerr.IsNotFound(err) {
			return nil
		}
		return err
	}

	if statefulSet.Labels[api.LabelDatabaseKind] != api.ResourceKindPercona ||
		statefulSet.Labels[api.LabelDatabaseName] != pxc.Name {
		return fmt.Errorf(`intended statefulSet "%v/%v" already exists`, pxc.Namespace, stsName)
	}

	return nil
}

func upsertCustomConfig(template core.PodTemplateSpec, configSource *core.VolumeSource) core.PodTemplateSpec {
	for i, container := range template.Spec.Containers {
		if container.Name == api.ResourceSingularPercona {
			configVolumeMount := core.VolumeMount{
				Name: "custom-config",
				MountPath: api.PerconaCustomConfigMountPath,
			}
			volumeMounts := container.VolumeMounts
			volumeMounts = core_util.UpsertVolumeMount(volumeMounts, configVolumeMount)
			template.Spec.Containers[i].VolumeMounts = volumeMounts

			configVolume := core.Volume{
				Name:         "custom-config",
				VolumeSource: *configSource,
			}

			volumes := template.Spec.Volumes
			volumes = core_util.UpsertVolume(volumes, configVolume)
			template.Spec.Volumes = volumes
			break
		}
	}

	return template
}

func (c *Controller) ensureStatefulSet(
	pxc *api.Percona,
	updateStrategy apps.StatefulSetUpdateStrategy,
	opts workloadOptions) (kutil.VerbType, error) {
	// Take value of podTemplate
	var pt ofst.PodTemplateSpec
	if opts.podTemplate != nil {
		pt = *opts.podTemplate
	}
	if err := c.checkStatefulSet(pxc, opts.stsName); err != nil {
		return kutil.VerbUnchanged, err
	}

	// Create statefulSet for Percona database
	statefulSetMeta := metav1.ObjectMeta{
		Name:      opts.stsName,
		Namespace: pxc.Namespace,
	}

	ref, rerr := reference.GetReference(clientsetscheme.Scheme, pxc)
	if rerr != nil {
		return kutil.VerbUnchanged, rerr
	}

	readinessProbe := pt.Spec.ReadinessProbe
	if readinessProbe != nil && structs.IsZero(*readinessProbe) {
		readinessProbe = nil
	}
	livenessProbe := pt.Spec.LivenessProbe
	if livenessProbe != nil && structs.IsZero(*livenessProbe) {
		livenessProbe = nil
	}

	statefulSet, vt, err := app_util.CreateOrPatchStatefulSet(c.Client, statefulSetMeta, func(in *apps.StatefulSet) *apps.StatefulSet {
		in.Labels = opts.labels
		in.Annotations = pt.Controller.Annotations
		core_util.EnsureOwnerReference(&in.ObjectMeta, ref)

		in.Spec.Replicas = opts.replicas
		in.Spec.ServiceName = opts.gvrSvcName
		in.Spec.Selector = &metav1.LabelSelector{
			MatchLabels: opts.selectors,
		}
		in.Spec.Template.Labels = opts.selectors
		in.Spec.Template.Annotations = pt.Annotations
		in.Spec.Template.Spec.InitContainers = core_util.UpsertContainers(
			in.Spec.Template.Spec.InitContainers,
			pt.Spec.InitContainers,
		)
		in.Spec.Template.Spec.Containers = core_util.UpsertContainer(
			in.Spec.Template.Spec.Containers,
			core.Container{
				Name:            opts.conatainerName,
				Image:           opts.image,
				ImagePullPolicy: core.PullIfNotPresent,
				Command:         opts.cmd,
				Args:            opts.args,
				Ports:           opts.ports,
				Env:             core_util.UpsertEnvVars(opts.envList, pt.Spec.Env...),
				Resources:       pt.Spec.Resources,
				Lifecycle:       pt.Spec.Lifecycle,
				LivenessProbe:   livenessProbe,
				ReadinessProbe:  readinessProbe,
				VolumeMounts:    opts.volumeMount,
			})

		in.Spec.Template.Spec.InitContainers = core_util.UpsertContainers(
			in.Spec.Template.Spec.InitContainers,
			opts.initContainers,
		)

		if opts.monitorContainer != nil && pxc.GetMonitoringVendor() == mona.VendorPrometheus {
			in.Spec.Template.Spec.Containers = core_util.UpsertContainer(
				in.Spec.Template.Spec.Containers, *opts.monitorContainer)
		}

		in.Spec.Template.Spec.Volumes = core_util.UpsertVolume(in.Spec.Template.Spec.Volumes, opts.volume...)

		in = upsertEnv(in, pxc)
		in = upsertDataVolume(in, pxc)

		if opts.configSource != nil {
			in.Spec.Template = upsertCustomConfig(in.Spec.Template, opts.configSource)
		}

		in.Spec.Template.Spec.NodeSelector = pt.Spec.NodeSelector
		in.Spec.Template.Spec.Affinity = pt.Spec.Affinity
		if pt.Spec.SchedulerName != "" {
			in.Spec.Template.Spec.SchedulerName = pt.Spec.SchedulerName
		}
		in.Spec.Template.Spec.Tolerations = pt.Spec.Tolerations
		in.Spec.Template.Spec.ImagePullSecrets = pt.Spec.ImagePullSecrets
		in.Spec.Template.Spec.PriorityClassName = pt.Spec.PriorityClassName
		in.Spec.Template.Spec.Priority = pt.Spec.Priority
		in.Spec.Template.Spec.SecurityContext = pt.Spec.SecurityContext

		if c.EnableRBAC {
			in.Spec.Template.Spec.ServiceAccountName = pt.Spec.ServiceAccountName
		}

		in.Spec.UpdateStrategy = updateStrategy
		return in
	})

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
			"Successfully %v StatefulSet %v/%v",
			vt, pxc.Namespace, opts.stsName,
		)
	}

	return vt, nil
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
