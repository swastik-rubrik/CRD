/*
Copyright 2026.

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

package controller

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	webappv1 "example.com/webapp-operator/api/v1"
)

// WebAppReconciler reconciles a WebApp object
type WebAppReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=apps.example.com,resources=webapps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.example.com,resources=webapps/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps.example.com,resources=webapps/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete

// Reconcile brings the cluster state in line with the WebApp spec: it ensures a
// Deployment exists that runs spec.Image with spec.Replicas replicas, and records
// progress on the WebApp's status conditions.
func (r *WebAppReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the WebApp. If it's gone, the owned Deployment is garbage-collected
	// automatically via the owner reference, so there's nothing to do.
	var webapp webappv1.WebApp
	if err := r.Get(ctx, req.NamespacedName, &webapp); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Build the desired Deployment and create-or-update it. controllerutil.CreateOrUpdate
	// makes this idempotent: it creates the Deployment if missing, otherwise patches the
	// fields we manage below.
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      webapp.Name,
			Namespace: webapp.Namespace,
		},
	}

	replicas := webapp.Spec.Replicas
	labels := map[string]string{"app": webapp.Name}

	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, deploy, func() error {
		deploy.Labels = labels
		deploy.Spec.Replicas = &replicas
		deploy.Spec.Selector = &metav1.LabelSelector{MatchLabels: labels}
		deploy.Spec.Template.ObjectMeta.Labels = labels
		deploy.Spec.Template.Spec.Containers = []corev1.Container{
			{
				Name:  "webapp",
				Image: webapp.Spec.Image,
				Env: []corev1.EnvVar{
					{Name: "GREETING", Value: webapp.Spec.Greeting},
				},
			},
		}
		// Set the WebApp as the owner so the Deployment is garbage-collected when the
		// WebApp is deleted, and so changes to it re-trigger reconciliation (via Owns).
		return controllerutil.SetControllerReference(&webapp, deploy, r.Scheme)
	})
	if err != nil {
		log.Error(err, "unable to create or update Deployment")
		meta.SetStatusCondition(&webapp.Status.Conditions, metav1.Condition{
			Type:    "Available",
			Status:  metav1.ConditionFalse,
			Reason:  "DeploymentReconcileFailed",
			Message: fmt.Sprintf("Failed to reconcile Deployment: %v", err),
		})
		_ = r.Status().Update(ctx, &webapp)
		return ctrl.Result{}, err
	}
	log.Info("reconciled Deployment", "operation", op, "name", deploy.Name)

	// Report readiness based on how many replicas are actually available.
	available := deploy.Status.AvailableReplicas == replicas
	condition := metav1.Condition{
		Type:    "Available",
		Status:  metav1.ConditionFalse,
		Reason:  "DeploymentNotReady",
		Message: fmt.Sprintf("%d/%d replicas available", deploy.Status.AvailableReplicas, replicas),
	}
	if available {
		condition.Status = metav1.ConditionTrue
		condition.Reason = "DeploymentReady"
		condition.Message = fmt.Sprintf("Deployment has %d/%d replicas available", deploy.Status.AvailableReplicas, replicas)
	}
	meta.SetStatusCondition(&webapp.Status.Conditions, condition)
	if err := r.Status().Update(ctx, &webapp); err != nil {
		if apierrors.IsConflict(err) {
			// The object was updated concurrently; requeue to retry with a fresh copy.
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *WebAppReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&webappv1.WebApp{}).
		Owns(&appsv1.Deployment{}).
		Named("webapp").
		Complete(r)
}
