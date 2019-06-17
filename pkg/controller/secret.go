package controller

import (
	"fmt"

	"github.com/appscode/go/crypto/rand"
	api "github.com/kubedb/apimachinery/apis/kubedb/v1alpha1"
	"github.com/kubedb/apimachinery/client/clientset/versioned/typed/kubedb/v1alpha1/util"
	core "k8s.io/api/core/v1"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	core_util "kmodules.xyz/client-go/core/v1"
)

const (
	mysqlUser = "root"

	KeyPerconaUser     = "username"
	KeyPerconaPassword = "password"
)

func (c *Controller) ensureDatabaseSecret(pxc *api.Percona) error {
	if pxc.Spec.DatabaseSecret == nil {
		secretVolumeSource, err := c.createDatabaseSecret(pxc)
		if err != nil {
			return err
		}

		per, _, err := util.PatchPercona(c.ExtClient.KubedbV1alpha1(), pxc, func(in *api.Percona) *api.Percona {
			in.Spec.DatabaseSecret = secretVolumeSource
			return in
		})
		if err != nil {
			return err
		}
		pxc.Spec.DatabaseSecret = per.Spec.DatabaseSecret
		return nil
	}
	return c.upgradeDatabaseSecret(pxc)
}

func (c *Controller) createDatabaseSecret(pxc *api.Percona) (*core.SecretVolumeSource, error) {
	authSecretName := pxc.Name + "-auth"

	sc, err := c.checkSecret(authSecretName, pxc)
	if err != nil {
		return nil, err
	}
	if sc == nil {
		randPassword := ""

		// if the password starts with "-", it will cause error in bash scripts (in percona-tools)
		for randPassword = rand.GeneratePassword(); randPassword[0] == '-'; {
		}

		secret := &core.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:   authSecretName,
				Labels: pxc.OffshootSelectors(),
			},
			Type: core.SecretTypeOpaque,
			StringData: map[string]string{
				KeyPerconaUser:     mysqlUser,
				KeyPerconaPassword: randPassword,
			},
		}

		if pxc.Spec.PXC != nil {
			randProxysqlPassword := ""

			// if the password starts with "-", it will cause error in bash scripts (in percona-tools)
			for randProxysqlPassword = rand.GeneratePassword(); randProxysqlPassword[0] == '-'; {
			}

			secret.StringData[api.ProxysqlUser] = "proxysql"
			secret.StringData[api.ProxysqlPassword] = randProxysqlPassword
		}

		if _, err := c.Client.CoreV1().Secrets(pxc.Namespace).Create(secret); err != nil {
			return nil, err
		}
	}
	return &core.SecretVolumeSource{
		SecretName: authSecretName,
	}, nil
}

// This is done to fix 0.8.0 -> 0.9.0 upgrade due to
// https://github.com/kubedb/percona/pull/115/files#diff-10ddaf307bbebafda149db10a28b9c24R17 commit
func (c *Controller) upgradeDatabaseSecret(pxc *api.Percona) error {
	meta := metav1.ObjectMeta{
		Name:      pxc.Spec.DatabaseSecret.SecretName,
		Namespace: pxc.Namespace,
	}

	_, _, err := core_util.CreateOrPatchSecret(c.Client, meta, func(in *core.Secret) *core.Secret {
		if _, ok := in.Data[KeyPerconaUser]; !ok {
			if val, ok2 := in.Data["user"]; ok2 {
				in.StringData = map[string]string{KeyPerconaUser: string(val)}
			}
		}
		return in
	})
	return err
}

func (c *Controller) checkSecret(secretName string, pxc *api.Percona) (*core.Secret, error) {
	secret, err := c.Client.CoreV1().Secrets(pxc.Namespace).Get(secretName, metav1.GetOptions{})
	if err != nil {
		if kerr.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	if secret.Labels[api.LabelDatabaseKind] != api.ResourceKindPercona ||
		secret.Labels[api.LabelDatabaseName] != pxc.Name {
		return nil, fmt.Errorf(`intended secret "%v/%v" already exists`, pxc.Namespace, secretName)
	}
	return secret, nil
}
