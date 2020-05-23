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
	"context"

	v1alpha1 "kubedb.dev/apimachinery/apis/ops/v1alpha1"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
)

// FakeRedisOpsRequests implements RedisOpsRequestInterface
type FakeRedisOpsRequests struct {
	Fake *FakeOpsV1alpha1
	ns   string
}

var redisopsrequestsResource = schema.GroupVersionResource{Group: "ops.kubedb.com", Version: "v1alpha1", Resource: "redisopsrequests"}

var redisopsrequestsKind = schema.GroupVersionKind{Group: "ops.kubedb.com", Version: "v1alpha1", Kind: "RedisOpsRequest"}

// Get takes name of the redisOpsRequest, and returns the corresponding redisOpsRequest object, and an error if there is any.
func (c *FakeRedisOpsRequests) Get(ctx context.Context, name string, options v1.GetOptions) (result *v1alpha1.RedisOpsRequest, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(redisopsrequestsResource, c.ns, name), &v1alpha1.RedisOpsRequest{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.RedisOpsRequest), err
}

// List takes label and field selectors, and returns the list of RedisOpsRequests that match those selectors.
func (c *FakeRedisOpsRequests) List(ctx context.Context, opts v1.ListOptions) (result *v1alpha1.RedisOpsRequestList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(redisopsrequestsResource, redisopsrequestsKind, c.ns, opts), &v1alpha1.RedisOpsRequestList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1alpha1.RedisOpsRequestList{ListMeta: obj.(*v1alpha1.RedisOpsRequestList).ListMeta}
	for _, item := range obj.(*v1alpha1.RedisOpsRequestList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested redisOpsRequests.
func (c *FakeRedisOpsRequests) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(redisopsrequestsResource, c.ns, opts))

}

// Create takes the representation of a redisOpsRequest and creates it.  Returns the server's representation of the redisOpsRequest, and an error, if there is any.
func (c *FakeRedisOpsRequests) Create(ctx context.Context, redisOpsRequest *v1alpha1.RedisOpsRequest, opts v1.CreateOptions) (result *v1alpha1.RedisOpsRequest, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(redisopsrequestsResource, c.ns, redisOpsRequest), &v1alpha1.RedisOpsRequest{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.RedisOpsRequest), err
}

// Update takes the representation of a redisOpsRequest and updates it. Returns the server's representation of the redisOpsRequest, and an error, if there is any.
func (c *FakeRedisOpsRequests) Update(ctx context.Context, redisOpsRequest *v1alpha1.RedisOpsRequest, opts v1.UpdateOptions) (result *v1alpha1.RedisOpsRequest, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(redisopsrequestsResource, c.ns, redisOpsRequest), &v1alpha1.RedisOpsRequest{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.RedisOpsRequest), err
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *FakeRedisOpsRequests) UpdateStatus(ctx context.Context, redisOpsRequest *v1alpha1.RedisOpsRequest, opts v1.UpdateOptions) (*v1alpha1.RedisOpsRequest, error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateSubresourceAction(redisopsrequestsResource, "status", c.ns, redisOpsRequest), &v1alpha1.RedisOpsRequest{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.RedisOpsRequest), err
}

// Delete takes name of the redisOpsRequest and deletes it. Returns an error if one occurs.
func (c *FakeRedisOpsRequests) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteAction(redisopsrequestsResource, c.ns, name), &v1alpha1.RedisOpsRequest{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeRedisOpsRequests) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(redisopsrequestsResource, c.ns, listOpts)

	_, err := c.Fake.Invokes(action, &v1alpha1.RedisOpsRequestList{})
	return err
}

// Patch applies the patch and returns the patched redisOpsRequest.
func (c *FakeRedisOpsRequests) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1alpha1.RedisOpsRequest, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(redisopsrequestsResource, c.ns, name, pt, data, subresources...), &v1alpha1.RedisOpsRequest{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.RedisOpsRequest), err
}
