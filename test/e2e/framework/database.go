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
package framework

import (
	"fmt"
	"strings"
	"time"

	api "kubedb.dev/apimachinery/apis/kubedb/v1alpha1"

	_ "github.com/go-sql-driver/mysql"
	"github.com/go-xorm/xorm"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"kmodules.xyz/client-go/tools/portforward"
)

type KubedbTable struct {
	Id      int64
	PodName string
}

func (f *Framework) forwardPort(meta metav1.ObjectMeta, clientPodIndex, remotePort int) (*portforward.Tunnel, error) {
	clientPodName := fmt.Sprintf("%v-%d", meta.Name, clientPodIndex)
	tunnel := portforward.NewTunnel(
		f.kubeClient.CoreV1().RESTClient(),
		f.restConfig,
		meta.Namespace,
		clientPodName,
		remotePort,
	)

	if err := tunnel.ForwardPort(); err != nil {
		return nil, err
	}
	return tunnel, nil
}

func (f *Framework) getPerconaXtraDBClient(meta metav1.ObjectMeta, tunnel *portforward.Tunnel, dbName string) (*xorm.Engine, error) {
	var user, pass string
	var userErr, passErr error

	px, err := f.GetPerconaXtraDB(meta)
	if err != nil {
		return nil, err
	}
	secretMeta := metav1.ObjectMeta{
		Name:      px.Spec.DatabaseSecret.SecretName,
		Namespace: px.Namespace,
	}

	user, userErr = f.GetSecretCred(secretMeta, api.MySQLUserKey)
	if userErr != nil {
		return nil, userErr
	}

	pass, passErr = f.GetSecretCred(secretMeta, api.MySQLPasswordKey)
	if passErr != nil {
		return nil, passErr
	}

	cnnstr := fmt.Sprintf("%v:%v@tcp(127.0.0.1:%v)/%s", user, pass, tunnel.Local, dbName)
	return xorm.NewEngine("mysql", cnnstr)
}

func (f *Framework) EventuallyDatabaseReady(meta metav1.ObjectMeta, dbName string, podIndex int) GomegaAsyncAssertion {
	return Eventually(
		func() bool {
			tunnel, en, err := f.GetEngine(meta, dbName, podIndex)
			if err != nil {
				return false
			}
			defer tunnel.Close()
			defer en.Close()

			return true
		},
		time.Minute*10,
		time.Second*20,
	)
}

func (f *Framework) GetEngine(
	meta metav1.ObjectMeta,
	dbName string, forwardingPodIndex int) (*portforward.Tunnel, *xorm.Engine, error) {

	var (
		tunnel *portforward.Tunnel
		en     *xorm.Engine
		err    error
		port   int
	)

	port = 3306
	By(fmt.Sprintf("Name: %v, Namespace: %v, Port: %v", meta.Name, meta.Namespace, port))

	tunnel, err = f.forwardPort(meta, forwardingPodIndex, port)
	if err != nil {
		return nil, nil, err
	}

	en, err = f.getPerconaXtraDBClient(meta, tunnel, dbName)
	if err != nil {
		return nil, nil, err
	}

	if err = en.Ping(); err != nil {
		return nil, nil, err
	}

	return tunnel, en, nil
}

func (f *Framework) EventuallyCreateDatabase(meta metav1.ObjectMeta, dbName string, podIndex int) GomegaAsyncAssertion {
	return Eventually(
		func() bool {
			tunnel, en, err := f.GetEngine(meta, dbName, podIndex)
			if err != nil {
				return false
			}
			defer tunnel.Close()
			defer en.Close()

			_, err = en.Exec("CREATE DATABASE kubedb")

			return err == nil
		},
		time.Minute*10,
		time.Second*20,
	)
}

func (f *Framework) EventuallyCreateTable(meta metav1.ObjectMeta, dbName string, podIndex int) GomegaAsyncAssertion {
	return Eventually(
		func() bool {
			tunnel, en, err := f.GetEngine(meta, dbName, podIndex)
			if err != nil {
				return false
			}
			defer tunnel.Close()
			defer en.Close()

			err = en.Sync(new(KubedbTable))
			if err != nil {
				fmt.Println("creation error", err)
				return false
			}
			return true
		},
		time.Minute*10,
		time.Second*20,
	)
}

func (f *Framework) EventuallyInsertRow(meta metav1.ObjectMeta, dbName string, podIndex, rowsCntToInsert int) GomegaAsyncAssertion {
	count := 0
	return Eventually(
		func() bool {
			tunnel, en, err := f.GetEngine(meta, dbName, podIndex)
			if err != nil {
				return false
			}
			defer tunnel.Close()
			defer en.Close()

			for i := count; i < rowsCntToInsert; i++ {
				if _, err := en.Insert(&KubedbTable{
					//Id:      int64(i),
					PodName: fmt.Sprintf("%s-%v", meta.Name, podIndex),
				}); err != nil {
					return false
				}
				count++
			}
			return true
		},
		time.Minute*10,
		time.Second*10,
	)
}

func (f *Framework) EventuallyCountRow(meta metav1.ObjectMeta, dbName string, podIndex int) GomegaAsyncAssertion {
	return Eventually(
		func() int {
			tunnel, en, err := f.GetEngine(meta, dbName, podIndex)
			if err != nil {
				return -1
			}
			defer tunnel.Close()
			defer en.Close()

			kubedb := new(KubedbTable)
			total, err := en.Count(kubedb)
			if err != nil {
				return -1
			}
			return int(total)
		},
		time.Minute*10,
		time.Second*20,
	)
}

func (f *Framework) EventuallyPerconaXtraDBVariable(meta metav1.ObjectMeta, dbName string, podIndex int, config string) GomegaAsyncAssertion {
	configPair := strings.Split(config, "=")
	sql := fmt.Sprintf("SHOW VARIABLES LIKE '%s';", configPair[0])
	return Eventually(
		func() []map[string][]byte {
			tunnel, en, err := f.GetEngine(meta, dbName, podIndex)
			if err != nil {
				return nil
			}
			defer tunnel.Close()
			defer en.Close()

			results, err := en.Query(sql)
			if err != nil {
				return nil
			}
			return results
		},
		time.Minute*5,
		time.Second*5,
	)
}
