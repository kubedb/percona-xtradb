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

package framework

import (
	"bytes"
	"context"
	"strings"

	shell "github.com/codeskyblue/go-sh"
	core "k8s.io/api/core/v1"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	meta_util "kmodules.xyz/client-go/meta"
)

func (f *Framework) InitScriptConfigMap() (*core.ConfigMap, error) {
	sh := shell.NewSession()

	var execOut bytes.Buffer
	sh.Stdout = &execOut
	if err := sh.Command("which", "curl").Run(); err != nil {
		return nil, err
	}

	curlLoc := strings.TrimSpace(execOut.String())
	execOut.Reset()

	sh.ShowCMD = true
	if err := sh.Command(curlLoc, "-fsSL", "https://github.com/kubedb/percona-xtradb-init-scripts/raw/master/init.sql").Run(); err != nil {
		return nil, err
	}

	return &core.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-init-script",
			Namespace: f.namespace,
		},
		Data: map[string]string{
			"init.sql": execOut.String(),
		},
	}, nil
}

func (f *Invocation) GetCustomConfig(configs []string) *core.Secret {
	configs = append([]string{"[mysqld]"}, configs...)
	return &core.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      f.app,
			Namespace: f.namespace,
		},
		StringData: map[string]string{
			"my-custom.cnf": strings.Join(configs, "\n"),
		},
	}
}

func (f *Invocation) CreateConfigMap(obj *core.ConfigMap) error {
	_, err := f.kubeClient.CoreV1().ConfigMaps(obj.Namespace).Create(context.TODO(), obj, metav1.CreateOptions{})
	return err
}

func (f *Framework) DeleteConfigMap(meta metav1.ObjectMeta) error {
	err := f.kubeClient.CoreV1().ConfigMaps(meta.Namespace).Delete(context.TODO(), meta.Name, meta_util.DeleteInForeground())
	if !kerr.IsNotFound(err) {
		return err
	}
	return nil
}
