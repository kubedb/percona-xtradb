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

// Code generated by client-gen. DO NOT EDIT.

package fake

import (
	v1alpha1 "kubedb.dev/apimachinery/apis/catalog/v1alpha1"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
)

// FakePerconaXtraDBVersions implements PerconaXtraDBVersionInterface
type FakePerconaXtraDBVersions struct {
	Fake *FakeCatalogV1alpha1
}

var perconaxtradbversionsResource = schema.GroupVersionResource{Group: "catalog.kubedb.com", Version: "v1alpha1", Resource: "perconaxtradbversions"}

var perconaxtradbversionsKind = schema.GroupVersionKind{Group: "catalog.kubedb.com", Version: "v1alpha1", Kind: "PerconaXtraDBVersion"}

// Get takes name of the perconaXtraDBVersion, and returns the corresponding perconaXtraDBVersion object, and an error if there is any.
func (c *FakePerconaXtraDBVersions) Get(name string, options v1.GetOptions) (result *v1alpha1.PerconaXtraDBVersion, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootGetAction(perconaxtradbversionsResource, name), &v1alpha1.PerconaXtraDBVersion{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.PerconaXtraDBVersion), err
}

// List takes label and field selectors, and returns the list of PerconaXtraDBVersions that match those selectors.
func (c *FakePerconaXtraDBVersions) List(opts v1.ListOptions) (result *v1alpha1.PerconaXtraDBVersionList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootListAction(perconaxtradbversionsResource, perconaxtradbversionsKind, opts), &v1alpha1.PerconaXtraDBVersionList{})
	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1alpha1.PerconaXtraDBVersionList{ListMeta: obj.(*v1alpha1.PerconaXtraDBVersionList).ListMeta}
	for _, item := range obj.(*v1alpha1.PerconaXtraDBVersionList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested perconaXtraDBVersions.
func (c *FakePerconaXtraDBVersions) Watch(opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewRootWatchAction(perconaxtradbversionsResource, opts))
}

// Create takes the representation of a perconaXtraDBVersion and creates it.  Returns the server's representation of the perconaXtraDBVersion, and an error, if there is any.
func (c *FakePerconaXtraDBVersions) Create(perconaXtraDBVersion *v1alpha1.PerconaXtraDBVersion) (result *v1alpha1.PerconaXtraDBVersion, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootCreateAction(perconaxtradbversionsResource, perconaXtraDBVersion), &v1alpha1.PerconaXtraDBVersion{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.PerconaXtraDBVersion), err
}

// Update takes the representation of a perconaXtraDBVersion and updates it. Returns the server's representation of the perconaXtraDBVersion, and an error, if there is any.
func (c *FakePerconaXtraDBVersions) Update(perconaXtraDBVersion *v1alpha1.PerconaXtraDBVersion) (result *v1alpha1.PerconaXtraDBVersion, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootUpdateAction(perconaxtradbversionsResource, perconaXtraDBVersion), &v1alpha1.PerconaXtraDBVersion{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.PerconaXtraDBVersion), err
}

// Delete takes name of the perconaXtraDBVersion and deletes it. Returns an error if one occurs.
func (c *FakePerconaXtraDBVersions) Delete(name string, options *v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewRootDeleteAction(perconaxtradbversionsResource, name), &v1alpha1.PerconaXtraDBVersion{})
	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakePerconaXtraDBVersions) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	action := testing.NewRootDeleteCollectionAction(perconaxtradbversionsResource, listOptions)

	_, err := c.Fake.Invokes(action, &v1alpha1.PerconaXtraDBVersionList{})
	return err
}

// Patch applies the patch and returns the patched perconaXtraDBVersion.
func (c *FakePerconaXtraDBVersions) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.PerconaXtraDBVersion, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootPatchSubresourceAction(perconaxtradbversionsResource, name, pt, data, subresources...), &v1alpha1.PerconaXtraDBVersion{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.PerconaXtraDBVersion), err
}
