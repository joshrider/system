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

package streaming

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/source"

	streamingv1alpha1 "github.com/projectriff/system/pkg/apis/streaming/v1alpha1"
	"github.com/projectriff/system/pkg/controllers"
	"github.com/projectriff/system/pkg/refs"
	"github.com/projectriff/system/pkg/tracker"
)

const (
	kafkaProviderDeploymentIndexField = ".metadata.kafkaProviderDeploymentController"
	kafkaProviderServiceIndexField    = ".metadata.kafkaProviderServiceController"
)

// KafkaProviderReconciler reconciles a KafkaProvider object
type KafkaProviderReconciler struct {
	client.Client
	Recorder  record.EventRecorder
	Log       logr.Logger
	Scheme    *runtime.Scheme
	Tracker   tracker.Tracker
	Namespace string
}

// +kubebuilder:rbac:groups=streaming.projectriff.io,resources=kafkaproviders,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=streaming.projectriff.io,resources=kafkaproviders/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=events,verbs=get;list;watch;create;update;patch;delete

func (r *KafkaProviderReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("kafkaprovider", req.NamespacedName)

	// your logic here
	var kafkaProvider streamingv1alpha1.KafkaProvider
	if err := r.Get(ctx, req.NamespacedName, &kafkaProvider); err != nil {
		log.Error(err, "unable to fetch KafkaProvider")
		// we'll ignore not-found errors, since they can't be fixed by an immediate
		// requeue (we'll need to wait for a new notification), and we can get them
		// on deleted requests.
		return ctrl.Result{}, ignoreNotFound(err)
	}

	originalKafkaProvider := kafkaProvider.DeepCopy()
	kafkaProvider.Default()
	kafkaProvider.Status.InitializeConditions()

	result, err := r.reconcile(ctx, log, &kafkaProvider)

	// check if status has changed before updating, unless requeued
	if !result.Requeue && !equality.Semantic.DeepEqual(kafkaProvider.Status, originalKafkaProvider.Status) {
		// update status
		log.Info("updating kafka provider status", "diff", cmp.Diff(originalKafkaProvider.Status, kafkaProvider.Status))
		if updateErr := r.Status().Update(ctx, &kafkaProvider); updateErr != nil {
			log.Error(updateErr, "unable to update KafkaProvider status", "kafkaprovider", kafkaProvider)
			r.Recorder.Eventf(&kafkaProvider, corev1.EventTypeWarning, "StatusUpdateFailed",
				"Failed to update status: %v", updateErr)
			return ctrl.Result{Requeue: true}, updateErr
		}
		r.Recorder.Eventf(&kafkaProvider, corev1.EventTypeNormal, "StatusUpdated",
			"Updated status")
	}

	return result, err
}

func (r *KafkaProviderReconciler) reconcile(ctx context.Context, log logr.Logger, kafkaProvider *streamingv1alpha1.KafkaProvider) (ctrl.Result, error) {

	// Lookup and track configMap to know which images to use
	cm := corev1.ConfigMap{}
	cmKey := types.NamespacedName{Namespace: r.Namespace, Name: kafkaProviderImages}
	// track config map for new images
	r.Tracker.Track(
		tracker.NewKey(schema.GroupVersionKind{Version: "v1", Kind: "ConfigMap"}, cmKey),
		types.NamespacedName{Namespace: kafkaProvider.GetNamespace(), Name: kafkaProvider.GetName()},
	)
	if err := r.Get(ctx, cmKey, &cm); err != nil {
		log.Error(err, "unable to lookup images configMap")
		return ctrl.Result{}, err
	}

	// Reconcile deployment for gateway
	gatewayDeployment, err := r.reconcileGatewayDeployment(ctx, log, kafkaProvider, &cm)
	if err != nil {
		log.Error(err, "unable to reconcile gateway Deployment", "kafkaprovider", kafkaProvider)
		return ctrl.Result{}, err
	}
	kafkaProvider.Status.GatewayDeploymentRef = refs.NewTypedLocalObjectReferenceForObject(gatewayDeployment, r.Scheme)
	kafkaProvider.Status.PropagateGatewayDeploymentStatus(&gatewayDeployment.Status)

	// Reconcile service for gateway
	gatewayService, err := r.reconcileGatewayService(ctx, log, kafkaProvider)
	if err != nil {
		log.Error(err, "unable to reconcile gateway Service", "kafkaprovider", kafkaProvider)
		return ctrl.Result{}, err
	}
	kafkaProvider.Status.GatewayServiceRef = refs.NewTypedLocalObjectReferenceForObject(gatewayService, r.Scheme)
	kafkaProvider.Status.PropagateGatewayServiceStatus(&gatewayService.Status)

	// Reconcile deployment for provisioner
	provisionerDeployment, err := r.reconcileProvisionerDeployment(ctx, log, kafkaProvider, &cm)
	if err != nil {
		log.Error(err, "unable to reconcile provisioner Deployment", "kafkaprovider", kafkaProvider)
		return ctrl.Result{}, err
	}
	kafkaProvider.Status.ProvisionerDeploymentRef = refs.NewTypedLocalObjectReferenceForObject(provisionerDeployment, r.Scheme)
	kafkaProvider.Status.PropagateProvisionerDeploymentStatus(&provisionerDeployment.Status)

	// Reconcile service for provisioner
	provisionerService, err := r.reconcileProvisionerService(ctx, log, kafkaProvider)
	if err != nil {
		log.Error(err, "unable to reconcile provisioner Service", "kafkaprovider", kafkaProvider)
		return ctrl.Result{}, err
	}
	kafkaProvider.Status.ProvisionerServiceRef = refs.NewTypedLocalObjectReferenceForObject(provisionerService, r.Scheme)
	kafkaProvider.Status.PropagateProvisionerServiceStatus(&provisionerService.Status)

	return ctrl.Result{}, nil

}

func (r *KafkaProviderReconciler) reconcileGatewayDeployment(ctx context.Context, log logr.Logger, kafkaProvider *streamingv1alpha1.KafkaProvider, cm *corev1.ConfigMap) (*appsv1.Deployment, error) {
	var actualDeployment appsv1.Deployment
	var childDeployments appsv1.DeploymentList
	if err := r.List(ctx, &childDeployments,
		client.InNamespace(kafkaProvider.Namespace),
		client.MatchingLabels(map[string]string{streamingv1alpha1.KafkaProviderGatewayLabelKey: kafkaProvider.Name}),
		client.MatchingField(kafkaProviderDeploymentIndexField, kafkaProvider.Name)); err != nil {
		return nil, err
	}
	// TODO do we need to remove resources pending deletion?
	if len(childDeployments.Items) == 1 {
		actualDeployment = childDeployments.Items[0]
	} else if len(childDeployments.Items) > 1 {
		// this shouldn't happen, delete everything to a clean slate
		for _, extraDeployment := range childDeployments.Items {
			log.Info("deleting extra gateway deployment", "deployment", extraDeployment)
			if err := r.Delete(ctx, &extraDeployment); err != nil {
				r.Recorder.Eventf(kafkaProvider, corev1.EventTypeWarning, "DeleteFailed",
					"Failed to delete gateway Deployment %q: %v", extraDeployment.Name, err)
				return nil, err
			}
			r.Recorder.Eventf(kafkaProvider, corev1.EventTypeNormal, "Deleted",
				"Deleted gateway Deployment %q", extraDeployment.Name)
		}
	}

	gatewayImg := cm.Data[gatewayImageKey]
	if gatewayImg == "" {
		return nil, fmt.Errorf("missing gateway image configuration")
	}

	desiredDeployment, err := r.constructGatewayDeploymentForKafkaProvider(kafkaProvider, gatewayImg)
	if err != nil {
		return nil, err
	}

	// delete deployment if no longer needed
	if desiredDeployment == nil {
		if err := r.Delete(ctx, &actualDeployment); err != nil {
			log.Error(err, "unable to delete Deployment for KafkaProvider", "deployment", actualDeployment)
			r.Recorder.Eventf(kafkaProvider, corev1.EventTypeWarning, "DeleteFailed",
				"Failed to delete gateway Deployment %q: %v", actualDeployment.Name, err)
			return nil, err
		}
		r.Recorder.Eventf(kafkaProvider, corev1.EventTypeNormal, "Deleted",
			"Deleted gateway Deployment %q", actualDeployment.Name)
		return nil, nil
	}

	// create deployment if it doesn't exist
	if actualDeployment.Name == "" {
		log.Info("creating gateway deployment", "spec", desiredDeployment.Spec)
		if err := r.Create(ctx, desiredDeployment); err != nil {
			log.Error(err, "unable to create Deployment for KafkaProvider", "deployment", desiredDeployment)
			r.Recorder.Eventf(kafkaProvider, corev1.EventTypeWarning, "CreationFailed",
				"Failed to create gateway Deployment %q: %v", desiredDeployment.Name, err)
			return nil, err
		}
		r.Recorder.Eventf(kafkaProvider, corev1.EventTypeNormal, "Created",
			"Created gateway Deployment %q", desiredDeployment.Name)
		return desiredDeployment, nil
	}

	// overwrite fields that should not be mutated
	desiredDeployment.Spec.Replicas = actualDeployment.Spec.Replicas

	if r.deploymentSemanticEquals(desiredDeployment, &actualDeployment) {
		// deployment is unchanged
		return &actualDeployment, nil
	}

	// update deployment with desired changes

	deployment := actualDeployment.DeepCopy()
	deployment.ObjectMeta.Labels = desiredDeployment.ObjectMeta.Labels
	deployment.Spec = desiredDeployment.Spec
	log.Info("reconciling gateway deployment", "diff", cmp.Diff(actualDeployment.Spec, deployment.Spec))
	if err := r.Update(ctx, deployment); err != nil {
		log.Error(err, "unable to update Deployment for KafkaProvider", "deployment", deployment)
		r.Recorder.Eventf(kafkaProvider, corev1.EventTypeWarning, "UpdateFailed",
			"Failed to update gateway Deployment %q: %v", deployment.Name, err)
		return nil, err
	}
	r.Recorder.Eventf(kafkaProvider, corev1.EventTypeNormal, "Updated",
		"Updated gateway Deployment %q", deployment.Name)

	return deployment, nil
}

func (r *KafkaProviderReconciler) constructGatewayDeploymentForKafkaProvider(kafkaProvider *streamingv1alpha1.KafkaProvider, gatewayImg string) (*appsv1.Deployment, error) {
	labels := r.constructGatewayLabelsForKafkaProvider(kafkaProvider)

	env, err := r.gatewayEnvironmentForKafkaProvider(kafkaProvider)
	if err != nil {
		return nil, err
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Labels:       labels,
			Annotations:  make(map[string]string),
			GenerateName: fmt.Sprintf("%s-kafka-gateway-", kafkaProvider.Name),
			Namespace:    kafkaProvider.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					streamingv1alpha1.KafkaProviderGatewayLabelKey: kafkaProvider.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "gateway",
							Image:           gatewayImg,
							ImagePullPolicy: corev1.PullAlways,
							Env:             env,
						},
					},
				},
			},
		},
	}
	if err := ctrl.SetControllerReference(kafkaProvider, deployment, r.Scheme); err != nil {
		return nil, err
	}

	return deployment, nil
}

func (r *KafkaProviderReconciler) gatewayEnvironmentForKafkaProvider(kafkaProvider *streamingv1alpha1.KafkaProvider) ([]corev1.EnvVar, error) {
	return []corev1.EnvVar{
		{Name: "kafka_bootstrapServers", Value: kafkaProvider.Spec.BootstrapServers},
		{Name: "storage_positions_type", Value: "MEMORY"},
		{Name: "storage_records_type", Value: "KAFKA"},
	}, nil
}

func (r *KafkaProviderReconciler) constructGatewayLabelsForKafkaProvider(kafkaProvider *streamingv1alpha1.KafkaProvider) map[string]string {
	labels := make(map[string]string, len(kafkaProvider.ObjectMeta.Labels)+1)
	// pass through existing labels
	for k, v := range kafkaProvider.ObjectMeta.Labels {
		labels[k] = v
	}

	labels[streamingv1alpha1.KafkaProviderLabelKey] = kafkaProvider.Name
	labels[streamingv1alpha1.KafkaProviderGatewayLabelKey] = kafkaProvider.Name

	return labels
}

func (r *KafkaProviderReconciler) deploymentSemanticEquals(desiredDeployment, deployment *appsv1.Deployment) bool {
	return equality.Semantic.DeepEqual(desiredDeployment.Spec, deployment.Spec) &&
		equality.Semantic.DeepEqual(desiredDeployment.ObjectMeta.Labels, deployment.ObjectMeta.Labels)
}

func (r *KafkaProviderReconciler) reconcileGatewayService(ctx context.Context, log logr.Logger, kafkaProvider *streamingv1alpha1.KafkaProvider) (*corev1.Service, error) {
	var actualService corev1.Service
	var childServices corev1.ServiceList
	if err := r.List(ctx, &childServices,
		client.InNamespace(kafkaProvider.Namespace),
		client.MatchingLabels(map[string]string{streamingv1alpha1.KafkaProviderGatewayLabelKey: kafkaProvider.Name}),
		client.MatchingField(kafkaProviderServiceIndexField, kafkaProvider.Name)); err != nil {
		return nil, err
	}
	// TODO do we need to remove resources pending deletion?
	if len(childServices.Items) == 1 {
		actualService = childServices.Items[0]
	} else if len(childServices.Items) > 1 {
		// this shouldn't happen, delete everything to a clean slate
		for _, extraService := range childServices.Items {
			log.Info("deleting extra gateway service", "service", extraService)
			if err := r.Delete(ctx, &extraService); err != nil {
				r.Recorder.Eventf(kafkaProvider, corev1.EventTypeWarning, "DeleteFailed",
					"Failed to delete gateway Service %q: %v", extraService.Name, err)
				return nil, err
			}
			r.Recorder.Eventf(kafkaProvider, corev1.EventTypeNormal, "Deleted",
				"Deleted gateway service %q", extraService.Name)
		}
	}

	desiredService, err := r.constructGatewayServiceForKafkaProvider(kafkaProvider)
	if err != nil {
		return nil, err
	}

	// delete service if no longer needed
	if desiredService == nil {
		if err := r.Delete(ctx, &actualService); err != nil {
			log.Error(err, "unable to delete gateway Service for KafkaProvider", "service", actualService)
			r.Recorder.Eventf(kafkaProvider, corev1.EventTypeWarning, "DeleteFailed",
				"Failed to delete gateway Service %q: %v", actualService.Name, err)
			return nil, err
		}
		r.Recorder.Eventf(kafkaProvider, corev1.EventTypeNormal, "Deleted",
			"Deleted gateway Service %q", actualService.Name)
		return nil, nil
	}

	// create service if it doesn't exist
	if actualService.Name == "" {
		log.Info("creating gateway service", "spec", desiredService.Spec)
		if err := r.Create(ctx, desiredService); err != nil {
			log.Error(err, "unable to create gateway Service for KafkaProvider", "service", desiredService)
			r.Recorder.Eventf(kafkaProvider, corev1.EventTypeWarning, "CreationFailed",
				"Failed to create gateway Service %q: %v", desiredService.Name, err)
			return nil, err
		}
		r.Recorder.Eventf(kafkaProvider, corev1.EventTypeNormal, "Created",
			"Created gateway Service %q", desiredService.Name)
		return desiredService, nil
	}

	// overwrite fields that should not be mutated
	desiredService.Spec.ClusterIP = actualService.Spec.ClusterIP

	if r.serviceSemanticEquals(desiredService, &actualService) {
		// service is unchanged
		return &actualService, nil
	}

	// update service with desired changes
	service := actualService.DeepCopy()
	service.ObjectMeta.Labels = desiredService.ObjectMeta.Labels
	service.Spec = desiredService.Spec
	log.Info("reconciling gateway service", "diff", cmp.Diff(actualService.Spec, service.Spec))
	if err := r.Update(ctx, service); err != nil {
		log.Error(err, "unable to update gateway Service for KafkaProvider", "service", service)
		r.Recorder.Eventf(kafkaProvider, corev1.EventTypeWarning, "UpdateFailed",
			"Failed to update gateway Service %q: %v", service.Name, err)
		return nil, err
	}
	r.Recorder.Eventf(kafkaProvider, corev1.EventTypeNormal, "Updated",
		"Updated gateway Service %q", service.Name)

	return service, nil
}

func (r *KafkaProviderReconciler) serviceSemanticEquals(desiredService, service *corev1.Service) bool {
	return equality.Semantic.DeepEqual(desiredService.Spec, service.Spec) &&
		equality.Semantic.DeepEqual(desiredService.ObjectMeta.Labels, service.ObjectMeta.Labels)
}

func (r *KafkaProviderReconciler) constructGatewayServiceForKafkaProvider(kafkaProvider *streamingv1alpha1.KafkaProvider) (*corev1.Service, error) {
	labels := r.constructGatewayLabelsForKafkaProvider(kafkaProvider)

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Labels:       labels,
			Annotations:  make(map[string]string),
			GenerateName: fmt.Sprintf("%s-kafka-gateway-", kafkaProvider.Name),
			Namespace:    kafkaProvider.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{Name: "gateway", Port: 6565},
			},
			Selector: map[string]string{
				streamingv1alpha1.KafkaProviderGatewayLabelKey: kafkaProvider.Name,
			},
		},
	}
	if err := ctrl.SetControllerReference(kafkaProvider, service, r.Scheme); err != nil {
		return nil, err
	}

	return service, nil
}

func (r *KafkaProviderReconciler) reconcileProvisionerDeployment(ctx context.Context, log logr.Logger, kafkaProvider *streamingv1alpha1.KafkaProvider, cm *corev1.ConfigMap) (*appsv1.Deployment, error) {
	var actualDeployment appsv1.Deployment
	var childDeployments appsv1.DeploymentList
	if err := r.List(ctx, &childDeployments,
		client.InNamespace(kafkaProvider.Namespace),
		client.MatchingLabels(map[string]string{streamingv1alpha1.KafkaProviderProvisionerLabelKey: kafkaProvider.Name}),
		client.MatchingField(kafkaProviderDeploymentIndexField, kafkaProvider.Name)); err != nil {
		return nil, err
	}
	// TODO do we need to remove resources pending deletion?
	if len(childDeployments.Items) == 1 {
		actualDeployment = childDeployments.Items[0]
	} else if len(childDeployments.Items) > 1 {
		// this shouldn't happen, delete everything to a clean slate
		for _, extraDeployment := range childDeployments.Items {
			log.Info("deleting extra provisioner deployment", "deployment", extraDeployment)
			if err := r.Delete(ctx, &extraDeployment); err != nil {
				r.Recorder.Eventf(kafkaProvider, corev1.EventTypeWarning, "DeleteFailed",
					"Failed to delete provisioner Deployment %q: %v", extraDeployment.Name, err)
				return nil, err
			}
			r.Recorder.Eventf(kafkaProvider, corev1.EventTypeNormal, "Deleted",
				"Deleted provisioner Deployment %q", extraDeployment.Name)
		}
	}

	provisionerImg := cm.Data[provisionerImageKey]
	if provisionerImg == "" {
		return nil, fmt.Errorf("missing provisioner image configuration")
	}

	desiredDeployment, err := r.constructProvisionerDeploymentForKafkaProvider(kafkaProvider, provisionerImg)
	if err != nil {
		return nil, err
	}

	// delete deployment if no longer needed
	if desiredDeployment == nil {
		if err := r.Delete(ctx, &actualDeployment); err != nil {
			log.Error(err, "unable to delete Deployment for KafkaProvider", "deployment", actualDeployment)
			r.Recorder.Eventf(kafkaProvider, corev1.EventTypeWarning, "DeleteFailed",
				"Failed to delete provisioner Deployment %q: %v", actualDeployment.Name, err)
			return nil, err
		}
		r.Recorder.Eventf(kafkaProvider, corev1.EventTypeNormal, "Deleted",
			"Deleted provisioner Deployment %q", actualDeployment.Name)
		return nil, nil
	}

	// create deployment if it doesn't exist
	if actualDeployment.Name == "" {
		log.Info("creating provisioner deployment", "spec", desiredDeployment.Spec)
		if err := r.Create(ctx, desiredDeployment); err != nil {
			log.Error(err, "unable to create Deployment for KafkaProvider", "deployment", desiredDeployment)
			r.Recorder.Eventf(kafkaProvider, corev1.EventTypeWarning, "CreationFailed",
				"Failed to create provisioner Deployment %q: %v", desiredDeployment.Name, err)
			return nil, err
		}
		r.Recorder.Eventf(kafkaProvider, corev1.EventTypeNormal, "Created",
			"Created provisioner Deployment %q", desiredDeployment.Name)
		return desiredDeployment, nil
	}

	// overwrite fields that should not be mutated
	desiredDeployment.Spec.Replicas = actualDeployment.Spec.Replicas

	if r.deploymentSemanticEquals(desiredDeployment, &actualDeployment) {
		// deployment is unchanged
		return &actualDeployment, nil
	}

	// update deployment with desired changes

	deployment := actualDeployment.DeepCopy()
	deployment.ObjectMeta.Labels = desiredDeployment.ObjectMeta.Labels
	deployment.Spec = desiredDeployment.Spec
	log.Info("reconciling provisioner deployment", "diff", cmp.Diff(actualDeployment.Spec, deployment.Spec))
	if err := r.Update(ctx, deployment); err != nil {
		log.Error(err, "unable to update Deployment for KafkaProvider", "deployment", deployment)
		r.Recorder.Eventf(kafkaProvider, corev1.EventTypeWarning, "UpdateFailed",
			"Failed to update provisioner Deployment %q: %v", deployment.Name, err)
		return nil, err
	}
	r.Recorder.Eventf(kafkaProvider, corev1.EventTypeNormal, "Updated",
		"Updated provisioner Deployment %q", deployment.Name)

	return deployment, nil
}

func (r *KafkaProviderReconciler) constructProvisionerDeploymentForKafkaProvider(kafkaProvider *streamingv1alpha1.KafkaProvider, provisionerImg string) (*appsv1.Deployment, error) {
	labels := r.constructProvisionerLabelsForKafkaProvider(kafkaProvider)

	env := []corev1.EnvVar{
		{Name: "GATEWAY", Value: fmt.Sprintf("%s.%s:6565", kafkaProvider.Status.GatewayServiceRef.Name, kafkaProvider.Namespace)}, // TODO get port number from svc lookup?
		{Name: "BROKER", Value: kafkaProvider.Spec.BootstrapServers},
	}
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Labels:       labels,
			Annotations:  make(map[string]string),
			GenerateName: fmt.Sprintf("%s-kafka-provisioner-", kafkaProvider.Name),
			Namespace:    kafkaProvider.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					streamingv1alpha1.KafkaProviderProvisionerLabelKey: kafkaProvider.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "main",
							Image:           provisionerImg,
							ImagePullPolicy: corev1.PullAlways,
							Env:             env,
						},
					},
				},
			},
		},
	}
	if err := ctrl.SetControllerReference(kafkaProvider, deployment, r.Scheme); err != nil {
		return nil, err
	}

	return deployment, nil
}

func (r *KafkaProviderReconciler) reconcileProvisionerService(ctx context.Context, log logr.Logger, kafkaProvider *streamingv1alpha1.KafkaProvider) (*corev1.Service, error) {
	var actualService corev1.Service
	var childServices corev1.ServiceList
	if err := r.List(ctx, &childServices,
		client.InNamespace(kafkaProvider.Namespace),
		client.MatchingLabels(map[string]string{streamingv1alpha1.KafkaProviderProvisionerLabelKey: kafkaProvider.Name}),
		client.MatchingField(kafkaProviderServiceIndexField, kafkaProvider.Name)); err != nil {
		return nil, err
	}
	// TODO do we need to remove resources pending deletion?
	if len(childServices.Items) == 1 {
		actualService = childServices.Items[0]
	} else if len(childServices.Items) > 1 {
		// this shouldn't happen, delete everything to a clean slate
		for _, extraService := range childServices.Items {
			log.Info("deleting extra provisioner service", "service", extraService)
			if err := r.Delete(ctx, &extraService); err != nil {
				r.Recorder.Eventf(kafkaProvider, corev1.EventTypeWarning, "DeleteFailed",
					"Failed to delete provisioner Service %q: %v", extraService.Name, err)
				return nil, err
			}
			r.Recorder.Eventf(kafkaProvider, corev1.EventTypeNormal, "Deleted",
				"Deleted provionser Service %q", extraService.Name)
		}
	}

	desiredService, err := r.constructProvisionerServiceForKafkaProvider(kafkaProvider)
	if err != nil {
		return nil, err
	}

	// delete service if no longer needed
	if desiredService == nil {
		if err := r.Delete(ctx, &actualService); err != nil {
			log.Error(err, "unable to delete provisioner Service for KafkaProvider", "service", actualService)
			r.Recorder.Eventf(kafkaProvider, corev1.EventTypeWarning, "DeleteFailed",
				"Failed to delete provisioner Service %q: %v", actualService.Name, err)
			return nil, err
		}
		r.Recorder.Eventf(kafkaProvider, corev1.EventTypeNormal, "Deleted",
			"Deleted provionser Service %q", actualService.Name)
		return nil, nil
	}

	// create service if it doesn't exist
	if actualService.Name == "" {
		log.Info("creating provisioner service", "spec", desiredService.Spec)
		if err := r.Create(ctx, desiredService); err != nil {
			log.Error(err, "unable to create provisioner Service for KafkaProvider", "service", desiredService)
			r.Recorder.Eventf(kafkaProvider, corev1.EventTypeWarning, "CreationFailed",
				"Failed to create provisioner Service %q: %v", desiredService.Name, err)
			return nil, err
		}
		r.Recorder.Eventf(kafkaProvider, corev1.EventTypeNormal, "Created",
			"Created provisioner Service %q", desiredService.Name)
		return desiredService, nil
	}

	// overwrite fields that should not be mutated
	desiredService.Spec.ClusterIP = actualService.Spec.ClusterIP

	if r.serviceSemanticEquals(desiredService, &actualService) {
		// service is unchanged
		return &actualService, nil
	}

	// update service with desired changes
	service := actualService.DeepCopy()
	service.ObjectMeta.Labels = desiredService.ObjectMeta.Labels
	service.Spec = desiredService.Spec
	log.Info("reconciling provisioner service", "diff", cmp.Diff(actualService.Spec, service.Spec))
	if err := r.Update(ctx, service); err != nil {
		log.Error(err, "unable to update provisioner Service for KafkaProvider", "service", service)
		r.Recorder.Eventf(kafkaProvider, corev1.EventTypeWarning, "UpdateFailed",
			"Failed to update provisioner Service %q: %v", service.Name, err)
		return nil, err
	}
	r.Recorder.Eventf(kafkaProvider, corev1.EventTypeNormal, "Updated",
		"Updated provisioner Service %q", service.Name)

	return service, nil
}

func (r *KafkaProviderReconciler) constructProvisionerServiceForKafkaProvider(kafkaProvider *streamingv1alpha1.KafkaProvider) (*corev1.Service, error) {
	labels := r.constructProvisionerLabelsForKafkaProvider(kafkaProvider)

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      labels,
			Annotations: make(map[string]string),
			Name:        fmt.Sprintf("%s-kafka-provisioner", kafkaProvider.Name),
			Namespace:   kafkaProvider.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{Name: "http", Port: 80, TargetPort: intstr.FromInt(8080)},
			},
			Selector: map[string]string{
				streamingv1alpha1.KafkaProviderProvisionerLabelKey: kafkaProvider.Name,
			},
		},
	}
	if err := ctrl.SetControllerReference(kafkaProvider, service, r.Scheme); err != nil {
		return nil, err
	}

	return service, nil
}

func (r *KafkaProviderReconciler) constructProvisionerLabelsForKafkaProvider(kafkaProvider *streamingv1alpha1.KafkaProvider) map[string]string {
	labels := make(map[string]string, len(kafkaProvider.ObjectMeta.Labels)+2)
	// pass through existing labels
	for k, v := range kafkaProvider.ObjectMeta.Labels {
		labels[k] = v
	}

	labels[streamingv1alpha1.KafkaProviderLabelKey] = kafkaProvider.Name
	labels[streamingv1alpha1.KafkaProviderProvisionerLabelKey] = kafkaProvider.Name
	labels[streamingv1alpha1.ProvisionerLabelKey] = streamingv1alpha1.KafkaProvisioner

	return labels
}

func (r *KafkaProviderReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := controllers.IndexControllersOfType(mgr, kafkaProviderDeploymentIndexField, &streamingv1alpha1.KafkaProvider{}, &appsv1.Deployment{}, r.Scheme); err != nil {
		return err
	}
	if err := controllers.IndexControllersOfType(mgr, kafkaProviderServiceIndexField, &streamingv1alpha1.KafkaProvider{}, &corev1.Service{}, r.Scheme); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&streamingv1alpha1.KafkaProvider{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Watches(&source.Kind{Type: &corev1.ConfigMap{}}, controllers.EnqueueTracked(&corev1.ConfigMap{}, r.Tracker, r.Scheme)).
		Complete(r)
}
