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

package factories

import (
	"fmt"

	"github.com/projectriff/system/pkg/apis"
	knativev1alpha1 "github.com/projectriff/system/pkg/apis/knative/v1alpha1"
)

type adapterKnative struct {
	target *knativev1alpha1.Adapter
}

func AdapterKnative(seed ...*knativev1alpha1.Adapter) *adapterKnative {
	var target *knativev1alpha1.Adapter
	switch len(seed) {
	case 0:
		target = &knativev1alpha1.Adapter{}
	case 1:
		target = seed[0]
	default:
		panic(fmt.Errorf("expected exactly zero or one seed, got %v", seed))
	}
	return &adapterKnative{
		target: target,
	}
}

func (f *adapterKnative) deepCopy() *adapterKnative {
	return AdapterKnative(f.target.DeepCopy())
}

func (f *adapterKnative) Get() *knativev1alpha1.Adapter {
	return f.deepCopy().target
}

func (f *adapterKnative) Mutate(m func(*knativev1alpha1.Adapter)) *adapterKnative {
	f = f.deepCopy()
	m(f.target)
	return f
}

func (f *adapterKnative) NamespaceName(namespace, name string) *adapterKnative {
	return f.Mutate(func(adapter *knativev1alpha1.Adapter) {
		adapter.ObjectMeta.Namespace = namespace
		adapter.ObjectMeta.Name = name
	})
}

func (f *adapterKnative) ObjectMeta(nf func(ObjectMeta)) *adapterKnative {
	return f.Mutate(func(adapter *knativev1alpha1.Adapter) {
		omf := objectMeta(adapter.ObjectMeta)
		nf(omf)
		adapter.ObjectMeta = omf.Get()
	})
}

func (f *adapterKnative) ApplicationRef(format string, a ...interface{}) *adapterKnative {
	return f.Mutate(func(adapter *knativev1alpha1.Adapter) {
		adapter.Spec.Build = knativev1alpha1.Build{
			ApplicationRef: fmt.Sprintf(format, a...),
		}
	})
}

func (f *adapterKnative) ContainerRef(format string, a ...interface{}) *adapterKnative {
	return f.Mutate(func(adapter *knativev1alpha1.Adapter) {
		adapter.Spec.Build = knativev1alpha1.Build{
			ContainerRef: fmt.Sprintf(format, a...),
		}
	})
}

func (f *adapterKnative) FunctionRef(format string, a ...interface{}) *adapterKnative {
	return f.Mutate(func(adapter *knativev1alpha1.Adapter) {
		adapter.Spec.Build = knativev1alpha1.Build{
			FunctionRef: fmt.Sprintf(format, a...),
		}
	})
}

func (f *adapterKnative) ConfigurationRef(format string, a ...interface{}) *adapterKnative {
	return f.Mutate(func(adapter *knativev1alpha1.Adapter) {
		adapter.Spec.Target = knativev1alpha1.AdapterTarget{
			ConfigurationRef: fmt.Sprintf(format, a...),
		}
	})
}

func (f *adapterKnative) ServiceRef(format string, a ...interface{}) *adapterKnative {
	return f.Mutate(func(adapter *knativev1alpha1.Adapter) {
		adapter.Spec.Target = knativev1alpha1.AdapterTarget{
			ServiceRef: fmt.Sprintf(format, a...),
		}
	})
}

func (f *adapterKnative) StatusConditions(conditions ...apis.Condition) *adapterKnative {
	return f.Mutate(func(adapter *knativev1alpha1.Adapter) {
		adapter.Status.Conditions = conditions
	})
}

func (f *adapterKnative) StatusObservedGeneration(generation int64) *adapterKnative {
	return f.Mutate(func(adapter *knativev1alpha1.Adapter) {
		adapter.Status.ObservedGeneration = generation
	})
}

func (f *adapterKnative) StatusLatestImage(format string, a ...interface{}) *adapterKnative {
	return f.Mutate(func(adapter *knativev1alpha1.Adapter) {
		adapter.Status.LatestImage = fmt.Sprintf(format, a...)
	})
}