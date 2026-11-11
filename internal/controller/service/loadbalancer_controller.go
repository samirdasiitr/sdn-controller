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

package service

import (
	"context"
	"fmt"
	"net"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	servicev1alpha1 "github.com/sdncontroller/sdncontroller/api/service/v1alpha1"
	vpcv1alpha1 "github.com/sdncontroller/sdncontroller/api/vpc/v1alpha1"
	"github.com/sdncontroller/sdncontroller/internal/placement"
)

const loadBalancerFinalizer = "loadbalancer.sdncontroller.io/finalizer"

// LoadBalancerReconciler reconciles a LoadBalancer object.
type LoadBalancerReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=service.sdncontroller.io,resources=loadbalancers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=service.sdncontroller.io,resources=loadbalancers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=service.sdncontroller.io,resources=loadbalancers/finalizers,verbs=update
// +kubebuilder:rbac:groups=vpc.sdncontroller.io,resources=vpcs,verbs=get;list;watch

func (r *LoadBalancerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var lb servicev1alpha1.LoadBalancer
	if err := r.Get(ctx, req.NamespacedName, &lb); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle deletion
	if !lb.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, &lb)
	}

	// Add finalizer
	if !controllerutil.ContainsFinalizer(&lb, loadBalancerFinalizer) {
		controllerutil.AddFinalizer(&lb, loadBalancerFinalizer)
		if err := r.Update(ctx, &lb); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Set phase to Provisioning
	if lb.Status.Phase == "" || lb.Status.Phase == servicev1alpha1.LBPhasePending {
		lb.Status.Phase = servicev1alpha1.LBPhaseProvisioning
		if err := r.Status().Update(ctx, &lb); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Validate VPC ref exists
	var vpc vpcv1alpha1.VPC
	if err := r.Get(ctx, types.NamespacedName{Name: lb.Spec.VPCRef, Namespace: lb.Namespace}, &vpc); err != nil {
		r.setCondition(&lb, "VPCReady", metav1.ConditionFalse, "VPCNotFound", fmt.Sprintf("VPC %q not found", lb.Spec.VPCRef))
		lb.Status.Phase = servicev1alpha1.LBPhaseFailed
		_ = r.Status().Update(ctx, &lb)
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}
	if vpc.Status.Phase != vpcv1alpha1.VPCPhaseReady {
		r.setCondition(&lb, "VPCReady", metav1.ConditionFalse, "VPCNotReady", "Parent VPC is not ready")
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}
	r.setCondition(&lb, "VPCReady", metav1.ConditionTrue, "Ready", "Parent VPC is ready")

	// Validate frontend IP
	if ip := net.ParseIP(lb.Spec.Frontend.IP); ip == nil {
		r.setCondition(&lb, "FrontendValid", metav1.ConditionFalse, "InvalidIP",
			fmt.Sprintf("Frontend IP %q is not a valid IP address", lb.Spec.Frontend.IP))
		lb.Status.Phase = servicev1alpha1.LBPhaseFailed
		_ = r.Status().Update(ctx, &lb)
		return ctrl.Result{}, nil
	}
	r.setCondition(&lb, "FrontendValid", metav1.ConditionTrue, "Valid",
		fmt.Sprintf("Frontend VIP %s with %d port(s)", lb.Spec.Frontend.IP, len(lb.Spec.Frontend.Ports)))

	// Validate backends
	if len(lb.Spec.Backends) == 0 {
		r.setCondition(&lb, "BackendsValid", metav1.ConditionFalse, "NoBackends", "No backends configured")
		lb.Status.Phase = servicev1alpha1.LBPhaseFailed
		_ = r.Status().Update(ctx, &lb)
		return ctrl.Result{}, nil
	}
	for _, backend := range lb.Spec.Backends {
		if ip := net.ParseIP(backend.IP); ip == nil {
			r.setCondition(&lb, "BackendsValid", metav1.ConditionFalse, "InvalidBackendIP",
				fmt.Sprintf("Backend IP %q is not a valid IP address", backend.IP))
			lb.Status.Phase = servicev1alpha1.LBPhaseFailed
			_ = r.Status().Update(ctx, &lb)
			return ctrl.Result{}, nil
		}
	}
	lb.Status.ActiveBackends = int32(len(lb.Spec.Backends))
	r.setCondition(&lb, "BackendsValid", metav1.ConditionTrue, "Valid",
		fmt.Sprintf("%d backend(s) validated", lb.Status.ActiveBackends))

	// Mark Ready
	if nodes, err := placement.Nodes(ctx, r.Client, lb.Namespace, lb.Spec.VPCRef, lb.Spec.NodeSelector); err != nil {
		return ctrl.Result{}, err
	} else {
		lb.Status.Nodes = nodes
		if len(nodes) == 0 {
			r.setCondition(&lb, "Placed", metav1.ConditionFalse, "NoEligibleNodes", "no Ready node matches nodeSelector and VPC attachment")
		} else {
			r.setCondition(&lb, "Placed", metav1.ConditionTrue, "Placed", fmt.Sprintf("running on %v", nodes))
		}
	}
	lb.Status.Phase = servicev1alpha1.LBPhaseReady
	r.setCondition(&lb, "Ready", metav1.ConditionTrue, "Provisioned", "LoadBalancer is ready")

	if err := r.Status().Update(ctx, &lb); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("LoadBalancer reconciled", "lb", lb.Name, "vpc", lb.Spec.VPCRef, "vip", lb.Spec.Frontend.IP, "backends", lb.Status.ActiveBackends)
	return ctrl.Result{}, nil
}

func (r *LoadBalancerReconciler) reconcileDelete(ctx context.Context, lb *servicev1alpha1.LoadBalancer) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if controllerutil.ContainsFinalizer(lb, loadBalancerFinalizer) {
		controllerutil.RemoveFinalizer(lb, loadBalancerFinalizer)
		if err := r.Update(ctx, lb); err != nil {
			return ctrl.Result{}, err
		}
		log.Info("LoadBalancer deleted", "lb", lb.Name)
	}
	return ctrl.Result{}, nil
}

func (r *LoadBalancerReconciler) setCondition(lb *servicev1alpha1.LoadBalancer, condType string, status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(&lb.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
	})
}

// SetupWithManager sets up the controller with the Manager.
func (r *LoadBalancerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&servicev1alpha1.LoadBalancer{}).
		Watches(&vpcv1alpha1.VPC{}, placement.ByVPC(r.Client, func() client.ObjectList { return &servicev1alpha1.LoadBalancerList{} }, func(o client.Object) string { return o.(*servicev1alpha1.LoadBalancer).Spec.VPCRef })).
		Named("service-loadbalancer").
		Complete(r)
}
