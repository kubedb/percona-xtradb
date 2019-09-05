/*
Copyright 2019 The KubeDB Authors.

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

// Code generated by informer-gen. DO NOT EDIT.

package externalversions

import (
	"fmt"

	schema "k8s.io/apimachinery/pkg/runtime/schema"
	cache "k8s.io/client-go/tools/cache"
	v1alpha1 "kubedb.dev/apimachinery/apis/catalog/v1alpha1"
	kubedbv1alpha1 "kubedb.dev/apimachinery/apis/kubedb/v1alpha1"
)

// GenericInformer is type of SharedIndexInformer which will locate and delegate to other
// sharedInformers based on type
type GenericInformer interface {
	Informer() cache.SharedIndexInformer
	Lister() cache.GenericLister
}

type genericInformer struct {
	informer cache.SharedIndexInformer
	resource schema.GroupResource
}

// Informer returns the SharedIndexInformer.
func (f *genericInformer) Informer() cache.SharedIndexInformer {
	return f.informer
}

// Lister returns the GenericLister.
func (f *genericInformer) Lister() cache.GenericLister {
	return cache.NewGenericLister(f.Informer().GetIndexer(), f.resource)
}

// ForResource gives generic access to a shared informer of the matching type
// TODO extend this to unknown resources with a client pool
func (f *sharedInformerFactory) ForResource(resource schema.GroupVersionResource) (GenericInformer, error) {
	switch resource {
	// Group=catalog.kubedb.com, Version=v1alpha1
	case v1alpha1.SchemeGroupVersion.WithResource("elasticsearchversions"):
		return &genericInformer{resource: resource.GroupResource(), informer: f.Catalog().V1alpha1().ElasticsearchVersions().Informer()}, nil
	case v1alpha1.SchemeGroupVersion.WithResource("etcdversions"):
		return &genericInformer{resource: resource.GroupResource(), informer: f.Catalog().V1alpha1().EtcdVersions().Informer()}, nil
	case v1alpha1.SchemeGroupVersion.WithResource("memcachedversions"):
		return &genericInformer{resource: resource.GroupResource(), informer: f.Catalog().V1alpha1().MemcachedVersions().Informer()}, nil
	case v1alpha1.SchemeGroupVersion.WithResource("mongodbversions"):
		return &genericInformer{resource: resource.GroupResource(), informer: f.Catalog().V1alpha1().MongoDBVersions().Informer()}, nil
	case v1alpha1.SchemeGroupVersion.WithResource("mysqlversions"):
		return &genericInformer{resource: resource.GroupResource(), informer: f.Catalog().V1alpha1().MySQLVersions().Informer()}, nil
	case v1alpha1.SchemeGroupVersion.WithResource("perconaxtradbversions"):
		return &genericInformer{resource: resource.GroupResource(), informer: f.Catalog().V1alpha1().PerconaXtraDBVersions().Informer()}, nil
	case v1alpha1.SchemeGroupVersion.WithResource("postgresversions"):
		return &genericInformer{resource: resource.GroupResource(), informer: f.Catalog().V1alpha1().PostgresVersions().Informer()}, nil
	case v1alpha1.SchemeGroupVersion.WithResource("proxysqlversions"):
		return &genericInformer{resource: resource.GroupResource(), informer: f.Catalog().V1alpha1().ProxySQLVersions().Informer()}, nil
	case v1alpha1.SchemeGroupVersion.WithResource("redisversions"):
		return &genericInformer{resource: resource.GroupResource(), informer: f.Catalog().V1alpha1().RedisVersions().Informer()}, nil

		// Group=kubedb.com, Version=v1alpha1
	case kubedbv1alpha1.SchemeGroupVersion.WithResource("dormantdatabases"):
		return &genericInformer{resource: resource.GroupResource(), informer: f.Kubedb().V1alpha1().DormantDatabases().Informer()}, nil
	case kubedbv1alpha1.SchemeGroupVersion.WithResource("elasticsearches"):
		return &genericInformer{resource: resource.GroupResource(), informer: f.Kubedb().V1alpha1().Elasticsearches().Informer()}, nil
	case kubedbv1alpha1.SchemeGroupVersion.WithResource("etcds"):
		return &genericInformer{resource: resource.GroupResource(), informer: f.Kubedb().V1alpha1().Etcds().Informer()}, nil
	case kubedbv1alpha1.SchemeGroupVersion.WithResource("mariadbs"):
		return &genericInformer{resource: resource.GroupResource(), informer: f.Kubedb().V1alpha1().MariaDBs().Informer()}, nil
	case kubedbv1alpha1.SchemeGroupVersion.WithResource("memcacheds"):
		return &genericInformer{resource: resource.GroupResource(), informer: f.Kubedb().V1alpha1().Memcacheds().Informer()}, nil
	case kubedbv1alpha1.SchemeGroupVersion.WithResource("mongodbs"):
		return &genericInformer{resource: resource.GroupResource(), informer: f.Kubedb().V1alpha1().MongoDBs().Informer()}, nil
	case kubedbv1alpha1.SchemeGroupVersion.WithResource("mysqls"):
		return &genericInformer{resource: resource.GroupResource(), informer: f.Kubedb().V1alpha1().MySQLs().Informer()}, nil
	case kubedbv1alpha1.SchemeGroupVersion.WithResource("perconaxtradbs"):
		return &genericInformer{resource: resource.GroupResource(), informer: f.Kubedb().V1alpha1().PerconaXtraDBs().Informer()}, nil
	case kubedbv1alpha1.SchemeGroupVersion.WithResource("postgreses"):
		return &genericInformer{resource: resource.GroupResource(), informer: f.Kubedb().V1alpha1().Postgreses().Informer()}, nil
	case kubedbv1alpha1.SchemeGroupVersion.WithResource("proxysqls"):
		return &genericInformer{resource: resource.GroupResource(), informer: f.Kubedb().V1alpha1().ProxySQLs().Informer()}, nil
	case kubedbv1alpha1.SchemeGroupVersion.WithResource("redises"):
		return &genericInformer{resource: resource.GroupResource(), informer: f.Kubedb().V1alpha1().Redises().Informer()}, nil
	case kubedbv1alpha1.SchemeGroupVersion.WithResource("snapshots"):
		return &genericInformer{resource: resource.GroupResource(), informer: f.Kubedb().V1alpha1().Snapshots().Informer()}, nil

	}

	return nil, fmt.Errorf("no informer found for %v", resource)
}
