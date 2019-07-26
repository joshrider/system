/*
Copyright 2018 The Knative Authors

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

package names

import (
	"fmt"

	streamingv1alpha1 "github.com/projectriff/system/pkg/apis/streaming/v1alpha1"
)

// Deployment returns the name used for the kubernetes deployment backing a riff processor.
func Deployment(p *streamingv1alpha1.Processor) string {
	return fmt.Sprintf("%s-processor", p.Name)
}

// ScaledObject returns the names used for the keda scaled-object backing a riff processor.
func ScaledObject(p *streamingv1alpha1.Processor) string {
	return fmt.Sprintf("%s-processor", p.Name)
}