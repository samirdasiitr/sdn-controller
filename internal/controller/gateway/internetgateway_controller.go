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

package gateway

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	gatewayv1alpha1 "github.com/sdncontroller/sdncontroller/api/gateway/v1alpha1"
	vpcv1alpha1 "github.com/sdncontroller/sdncontroller/api/vpc/v1alpha1"
)

const internetGatewayFinalizer = "internetgateway.sdncontroller.io/finalizer"

// InternetGatewayReconciler reconciles a InternetGateway object.
type InternetGatewayReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=gateway.sdncontroller.io,resources=internetgateways,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.sdncontroller.io,resources=internetgateways/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=gateway.sdncontroller.io,resources=internetgateways/finalizers,verbs=update
// +kubebuilder:rbac:groups=vpc.sdncontroller.io,resources=vpcs,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch

func (r *InternetGatewayReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var igw gatewayv1alpha1.InternetGateway
	if err := r.Get(ctx, req.NamespacedName, &igw); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle deletion
	if !igw.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, &igw)
	}

	// Add finalizer
	if !controllerutil.ContainsFinalizer(&igw, internetGatewayFinalizer) {
		controllerutil.AddFinalizer(&igw, internetGatewayFinalizer)
		if err := r.Update(ctx, &igw); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Set phase to Provisioning
	if igw.Status.Phase == "" || igw.Status.Phase == gatewayv1alpha1.GatewayPhasePending {
		igw.Status.Phase = gatewayv1alpha1.GatewayPhaseProvisioning
		if err := r.Status().Update(ctx, &igw); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Validate VPC ref exists
	var vpc vpcv1alpha1.VPC
	if err := r.Get(ctx, types.NamespacedName{Name: igw.Spec.VPCRef, Namespace: igw.Namespace}, &vpc); err != nil {
		r.setCondition(&igw, "VPCReady", metav1.ConditionFalse, "VPCNotFound", fmt.Sprintf("VPC %q not found", igw.Spec.VPCRef))
		igw.Status.Phase = gatewayv1alpha1.GatewayPhaseFailed
		_ = r.Status().Update(ctx, &igw)
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}
	if vpc.Status.Phase != vpcv1alpha1.VPCPhaseReady {
		r.setCondition(&igw, "VPCReady", metav1.ConditionFalse, "VPCNotReady", "Parent VPC is not ready")
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}
	r.setCondition(&igw, "VPCReady", metav1.ConditionTrue, "Ready", "Parent VPC is ready")

	// Select active/standby nodes based on nodeSelector
	var nodeList corev1.NodeList
	if igw.Spec.NodeSelector != nil {
		selector, err := metav1.LabelSelectorAsSelector(igw.Spec.NodeSelector)
		if err != nil {
			r.setCondition(&igw, "NodesSelected", metav1.ConditionFalse, "InvalidSelector", err.Error())
			igw.Status.Phase = gatewayv1alpha1.GatewayPhaseFailed
			_ = r.Status().Update(ctx, &igw)
			return ctrl.Result{}, nil
		}
		if err := r.List(ctx, &nodeList, &client.ListOptions{LabelSelector: selector}); err != nil {
			return ctrl.Result{}, err
		}
	} else {
		if err := r.List(ctx, &nodeList); err != nil {
			return ctrl.Result{}, err
		}
	}

	readyNodes := filterReadyNodes(nodeList.Items)
	if len(readyNodes) == 0 {
		r.setCondition(&igw, "NodesSelected", metav1.ConditionFalse, "NoReadyNodes", "No ready nodes matching selector")
		igw.Status.Phase = gatewayv1alpha1.GatewayPhaseFailed
		_ = r.Status().Update(ctx, &igw)
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	igw.Status.ActiveNode = readyNodes[0].Name
	if igw.Spec.HA != nil && igw.Spec.HA.Enabled && len(readyNodes) > 1 {
		igw.Status.StandbyNode = readyNodes[1].Name
	} else {
		igw.Status.StandbyNode = ""
	}
	r.setCondition(&igw, "NodesSelected", metav1.ConditionTrue, "NodesAssigned",
		fmt.Sprintf("Active: %s", igw.Status.ActiveNode))

	// Mark Ready
	igw.Status.Phase = gatewayv1alpha1.GatewayPhaseReady
	r.setCondition(&igw, "Ready", metav1.ConditionTrue, "Provisioned", "InternetGateway is ready")

	if err := r.Status().Update(ctx, &igw); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("InternetGateway reconciled", "igw", igw.Name, "vpc", igw.Spec.VPCRef, "activeNode", igw.Status.ActiveNode)
	return ctrl.Result{}, nil
}

func (r *InternetGatewayReconciler) reconcileDelete(ctx context.Context, igw *gatewayv1alpha1.InternetGateway) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if controllerutil.ContainsFinalizer(igw, internetGatewayFinalizer) {
		controllerutil.RemoveFinalizer(igw, internetGatewayFinalizer)
		if err := r.Update(ctx, igw); err != nil {
			return ctrl.Result{}, err
		}
		log.Info("InternetGateway deleted", "igw", igw.Name)
	}
	return ctrl.Result{}, nil
}

func (r *InternetGatewayReconciler) setCondition(igw *gatewayv1alpha1.InternetGateway, condType string, status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(&igw.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
	})
}

// filterReadyNodes returns nodes that have the Ready condition True.
func filterReadyNodes(nodes []corev1.Node) []corev1.Node {
	var ready []corev1.Node
	for _, n := range nodes {
		for _, c := range n.Status.Conditions {
			if c.Type == corev1.NodeReady && c.Status == corev1.ConditionTrue {
				ready = append(ready, n)
				break
			}
		}
	}
	return ready
}

// SetupWithManager sets up the controller with the Manager.
func (r *InternetGatewayReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&gatewayv1alpha1.InternetGateway{}).
		Named("gateway-internetgateway").
		Complete(r)
}


