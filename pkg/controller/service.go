package controller

import (
	"fmt"

	"github.com/appscode/go/log"
	core "k8s.io/api/core/v1"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	clientsetscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/reference"
	kutil "kmodules.xyz/client-go"
	core_util "kmodules.xyz/client-go/core/v1"
	mona "kmodules.xyz/monitoring-agent-api/api/v1"
	ofst "kmodules.xyz/offshoot-api/api/v1"
	api "kubedb.dev/apimachinery/apis/kubedb/v1alpha1"
	"kubedb.dev/apimachinery/pkg/eventer"
)

var defaultDBPort = core.ServicePort{
	Name:       "db",
	Protocol:   core.ProtocolTCP,
	Port:       api.MySQLNodePort,
	TargetPort: intstr.FromInt(api.MySQLNodePort),
}

func (c *Controller) ensureService(px *api.PerconaXtraDB) (kutil.VerbType, error) {
	// Check if service name exists
	if err := c.checkService(px, px.ServiceName()); err != nil {
		return kutil.VerbUnchanged, err
	}

	// create database Service
	vt, err := c.createService(px)
	if err != nil {
		return kutil.VerbUnchanged, err
	} else if vt != kutil.VerbUnchanged {
		c.recorder.Eventf(
			px,
			core.EventTypeNormal,
			eventer.EventReasonSuccessful,
			"Successfully %s Service",
			vt,
		)
	}
	return vt, nil
}

func (c *Controller) checkService(px *api.PerconaXtraDB, serviceName string) error {
	service, err := c.Client.CoreV1().Services(px.Namespace).Get(serviceName, metav1.GetOptions{})
	if err != nil {
		if kerr.IsNotFound(err) {
			return nil
		}
		return err
	}

	if service.Labels[api.LabelDatabaseKind] != api.ResourceKindPerconaXtraDB ||
		service.Labels[api.LabelDatabaseName] != px.Name {
		return fmt.Errorf(`intended service "%v/%v" already exists`, px.Namespace, serviceName)
	}

	return nil
}

func (c *Controller) createService(px *api.PerconaXtraDB) (kutil.VerbType, error) {
	meta := metav1.ObjectMeta{
		Name:      px.OffshootName(),
		Namespace: px.Namespace,
	}

	ref, rerr := reference.GetReference(clientsetscheme.Scheme, px)
	if rerr != nil {
		return kutil.VerbUnchanged, rerr
	}

	_, ok, err := core_util.CreateOrPatchService(c.Client, meta, func(in *core.Service) *core.Service {
		core_util.EnsureOwnerReference(&in.ObjectMeta, ref)
		in.Labels = px.OffshootLabels()
		in.Annotations = px.Spec.ServiceTemplate.Annotations

		in.Spec.Selector = px.OffshootSelectors()
		in.Spec.Ports = ofst.MergeServicePorts(
			core_util.MergeServicePorts(in.Spec.Ports, []core.ServicePort{defaultDBPort}),
			px.Spec.ServiceTemplate.Spec.Ports,
		)

		if px.Spec.ServiceTemplate.Spec.ClusterIP != "" {
			in.Spec.ClusterIP = px.Spec.ServiceTemplate.Spec.ClusterIP
		}
		if px.Spec.ServiceTemplate.Spec.Type != "" {
			in.Spec.Type = px.Spec.ServiceTemplate.Spec.Type
		}
		in.Spec.ExternalIPs = px.Spec.ServiceTemplate.Spec.ExternalIPs
		in.Spec.LoadBalancerIP = px.Spec.ServiceTemplate.Spec.LoadBalancerIP
		in.Spec.LoadBalancerSourceRanges = px.Spec.ServiceTemplate.Spec.LoadBalancerSourceRanges
		in.Spec.ExternalTrafficPolicy = px.Spec.ServiceTemplate.Spec.ExternalTrafficPolicy
		if px.Spec.ServiceTemplate.Spec.HealthCheckNodePort > 0 {
			in.Spec.HealthCheckNodePort = px.Spec.ServiceTemplate.Spec.HealthCheckNodePort
		}
		return in
	})
	return ok, err
}

func (c *Controller) ensureStatsService(px *api.PerconaXtraDB) (kutil.VerbType, error) {
	// return if monitoring is not prometheus
	if px.GetMonitoringVendor() != mona.VendorPrometheus {
		log.Infoln("spec.monitor.agent is not coreos-operator or builtin.")
		return kutil.VerbUnchanged, nil
	}

	// Check if statsService name exists
	if err := c.checkService(px, px.StatsService().ServiceName()); err != nil {
		return kutil.VerbUnchanged, err
	}

	ref, rerr := reference.GetReference(clientsetscheme.Scheme, px)
	if rerr != nil {
		return kutil.VerbUnchanged, rerr
	}

	// reconcile stats Service
	meta := metav1.ObjectMeta{
		Name:      px.StatsService().ServiceName(),
		Namespace: px.Namespace,
	}
	_, vt, err := core_util.CreateOrPatchService(c.Client, meta, func(in *core.Service) *core.Service {
		core_util.EnsureOwnerReference(&in.ObjectMeta, ref)
		in.Labels = px.StatsServiceLabels()
		in.Spec.Selector = px.OffshootSelectors()
		in.Spec.Ports = core_util.MergeServicePorts(in.Spec.Ports, []core.ServicePort{
			{
				Name:       api.PrometheusExporterPortName,
				Protocol:   core.ProtocolTCP,
				Port:       px.Spec.Monitor.Prometheus.Port,
				TargetPort: intstr.FromString(api.PrometheusExporterPortName),
			},
		})
		return in
	})
	if err != nil {
		return kutil.VerbUnchanged, err
	} else if vt != kutil.VerbUnchanged {
		c.recorder.Eventf(
			px,
			core.EventTypeNormal,
			eventer.EventReasonSuccessful,
			"Successfully %s stats service",
			vt,
		)
	}
	return vt, nil
}

func (c *Controller) createPerconaXtraDBGoverningService(px *api.PerconaXtraDB) (string, error) {
	ref, rerr := reference.GetReference(clientsetscheme.Scheme, px)
	if rerr != nil {
		return "", rerr
	}

	service := &core.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      px.GoverningServiceName(),
			Namespace: px.Namespace,
			Labels:    px.OffshootLabels(),
			// 'tolerate-unready-endpoints' annotation is deprecated.
			// ref: https://github.com/kubernetes/kubernetes/pull/63742
			Annotations: map[string]string{
				"service.alpha.kubernetes.io/tolerate-unready-endpoints": "true",
			},
		},
		Spec: core.ServiceSpec{
			Type:                     core.ServiceTypeClusterIP,
			ClusterIP:                core.ClusterIPNone,
			PublishNotReadyAddresses: true,
			Ports: []core.ServicePort{
				{
					Name: "db",
					Port: api.MySQLNodePort,
				},
			},
			Selector: px.OffshootSelectors(),
		},
	}
	core_util.EnsureOwnerReference(&service.ObjectMeta, ref)

	_, err := c.Client.CoreV1().Services(px.Namespace).Create(service)
	if err != nil && !kerr.IsAlreadyExists(err) {
		return "", err
	}
	return service.Name, nil
}
