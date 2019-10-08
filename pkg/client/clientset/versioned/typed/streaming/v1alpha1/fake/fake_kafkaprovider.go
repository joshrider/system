/*
Copyright 2019 the original author or authors.

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
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"

	v1alpha1 "github.com/projectriff/system/pkg/apis/streaming/v1alpha1"
)

// FakeKafkaProviders implements KafkaProviderInterface
type FakeKafkaProviders struct {
	Fake *FakeStreamingV1alpha1
	ns   string
}

var kafkaprovidersResource = schema.GroupVersionResource{Group: "streaming.projectriff.io", Version: "v1alpha1", Resource: "kafkaproviders"}

var kafkaprovidersKind = schema.GroupVersionKind{Group: "streaming.projectriff.io", Version: "v1alpha1", Kind: "KafkaProvider"}

// Get takes name of the kafkaProvider, and returns the corresponding kafkaProvider object, and an error if there is any.
func (c *FakeKafkaProviders) Get(name string, options v1.GetOptions) (result *v1alpha1.KafkaProvider, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(kafkaprovidersResource, c.ns, name), &v1alpha1.KafkaProvider{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.KafkaProvider), err
}

// List takes label and field selectors, and returns the list of KafkaProviders that match those selectors.
func (c *FakeKafkaProviders) List(opts v1.ListOptions) (result *v1alpha1.KafkaProviderList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(kafkaprovidersResource, kafkaprovidersKind, c.ns, opts), &v1alpha1.KafkaProviderList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1alpha1.KafkaProviderList{ListMeta: obj.(*v1alpha1.KafkaProviderList).ListMeta}
	for _, item := range obj.(*v1alpha1.KafkaProviderList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested kafkaProviders.
func (c *FakeKafkaProviders) Watch(opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(kafkaprovidersResource, c.ns, opts))

}

// Create takes the representation of a kafkaProvider and creates it.  Returns the server's representation of the kafkaProvider, and an error, if there is any.
func (c *FakeKafkaProviders) Create(kafkaProvider *v1alpha1.KafkaProvider) (result *v1alpha1.KafkaProvider, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(kafkaprovidersResource, c.ns, kafkaProvider), &v1alpha1.KafkaProvider{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.KafkaProvider), err
}

// Update takes the representation of a kafkaProvider and updates it. Returns the server's representation of the kafkaProvider, and an error, if there is any.
func (c *FakeKafkaProviders) Update(kafkaProvider *v1alpha1.KafkaProvider) (result *v1alpha1.KafkaProvider, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(kafkaprovidersResource, c.ns, kafkaProvider), &v1alpha1.KafkaProvider{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.KafkaProvider), err
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *FakeKafkaProviders) UpdateStatus(kafkaProvider *v1alpha1.KafkaProvider) (*v1alpha1.KafkaProvider, error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateSubresourceAction(kafkaprovidersResource, "status", c.ns, kafkaProvider), &v1alpha1.KafkaProvider{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.KafkaProvider), err
}

// Delete takes name of the kafkaProvider and deletes it. Returns an error if one occurs.
func (c *FakeKafkaProviders) Delete(name string, options *v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteAction(kafkaprovidersResource, c.ns, name), &v1alpha1.KafkaProvider{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeKafkaProviders) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(kafkaprovidersResource, c.ns, listOptions)

	_, err := c.Fake.Invokes(action, &v1alpha1.KafkaProviderList{})
	return err
}

// Patch applies the patch and returns the patched kafkaProvider.
func (c *FakeKafkaProviders) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.KafkaProvider, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(kafkaprovidersResource, c.ns, name, pt, data, subresources...), &v1alpha1.KafkaProvider{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.KafkaProvider), err
}