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

package nfv

import (
	"context"
	"fmt"
	corev1 "k8s.io/api/core/v1"
	"time"

	"github.com/sdncontroller/sdncontroller/internal/deps"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	nfvv1alpha1 "github.com/sdncontroller/sdncontroller/api/nfv/v1alpha1"
	"github.com/sdncontroller/sdncontroller/internal/placement"
)

const virtualNetworkFunctionFinalizer = "virtualnetworkfunction.sdncontroller.io/finalizer"

// VirtualNetworkFunctionReconciler reconciles a VirtualNetworkFunction object.
type VirtualNetworkFunctionReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=nfv.sdncontroller.io,resources=virtualnetworkfunctions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=nfv.sdncontroller.io,resources=virtualnetworkfunctions/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=nfv.sdncontroller.io,resources=virtualnetworkfunctions/finalizers,verbs=update

func (r *VirtualNetworkFunctionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var vnf nfvv1alpha1.VirtualNetworkFunction
	if err := r.Get(ctx, req.NamespacedName, &vnf); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle deletion
	if !vnf.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, &vnf)
	}

	// Add finalizer
	if !controllerutil.ContainsFinalizer(&vnf, virtualNetworkFunctionFinalizer) {
		controllerutil.AddFinalizer(&vnf, virtualNetworkFunctionFinalizer)
		if err := r.Update(ctx, &vnf); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Set phase to Creating if not set
	if vnf.Status.Phase == "" || vnf.Status.Phase == nfvv1alpha1.VNFPhasePending {
		vnf.Status.Phase = nfvv1alpha1.VNFPhaseCreating
		if err := r.Status().Update(ctx, &vnf); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Manage VNF lifecycle based on type
	switch vnf.Spec.Type {
	case nfvv1alpha1.VNFTypeContainer:
		if vnf.Spec.Image == "" {
			r.setCondition(&vnf, "ImageValid", metav1.ConditionFalse, "NoImage",
				"Container VNF requires an image")
			vnf.Status.Phase = nfvv1alpha1.VNFPhaseFailed
			_ = r.Status().Update(ctx, &vnf)
			return ctrl.Result{}, nil
		}
		r.setCondition(&vnf, "ImageValid", metav1.ConditionTrue, "Valid",
			fmt.Sprintf("Container image: %s", vnf.Spec.Image))

	case nfvv1alpha1.VNFTypeVM:
		if vnf.Spec.VMConfig == nil {
			r.setCondition(&vnf, "VMConfigValid", metav1.ConditionFalse, "NoVMConfig",
				"VM VNF requires vmConfig")
			vnf.Status.Phase = nfvv1alpha1.VNFPhaseFailed
			_ = r.Status().Update(ctx, &vnf)
			return ctrl.Result{}, nil
		}
		r.setCondition(&vnf, "VMConfigValid", metav1.ConditionTrue, "Valid",
			fmt.Sprintf("VM config: %d vCPUs, %dMB memory", vnf.Spec.VMConfig.VCPUs, vnf.Spec.VMConfig.MemoryMB))

	default:
		r.setCondition(&vnf, "TypeValid", metav1.ConditionFalse, "InvalidType",
			fmt.Sprintf("Unknown VNF type: %s", vnf.Spec.Type))
		vnf.Status.Phase = nfvv1alpha1.VNFPhaseFailed
		_ = r.Status().Update(ctx, &vnf)
		return ctrl.Result{}, nil
	}

	// Validate interfaces
	if len(vnf.Spec.Interfaces) == 0 {
		r.setCondition(&vnf, "InterfacesValid", metav1.ConditionFalse, "NoInterfaces",
			"VNF requires at least one interface")
		vnf.Status.Phase = nfvv1alpha1.VNFPhaseFailed
		_ = r.Status().Update(ctx, &vnf)
		return ctrl.Result{}, nil
	}
	r.setCondition(&vnf, "InterfacesValid", metav1.ConditionTrue, "Valid",
		fmt.Sprintf("%d interface(s) configured", len(vnf.Spec.Interfaces)))

	// Set ReadyReplicas (simulated - assume all desired replicas are ready)
	vnf.Status.ReadyReplicas = vnf.Spec.Replicas

	// Mark Running
	if nodes, err := placement.Nodes(ctx, r.Client, vnf.Namespace, "", vnf.Spec.NodeSelector); err != nil {
		return ctrl.Result{}, err
	} else {
		vnf.Status.Nodes = nodes
		if len(nodes) == 0 {
			r.setCondition(&vnf, "Placed", metav1.ConditionFalse, "NoEligibleNodes", "no Ready node matches nodeSelector and VPC attachment")
		} else {
			r.setCondition(&vnf, "Placed", metav1.ConditionTrue, "Placed", fmt.Sprintf("running on %v", nodes))
		}
	}
	vnf.Status.Phase = nfvv1alpha1.VNFPhaseRunning
	r.setCondition(&vnf, "Ready", metav1.ConditionTrue, "Running",
		fmt.Sprintf("VNF running: %d/%d replicas ready", vnf.Status.ReadyReplicas, vnf.Spec.Replicas))

	if err := r.Status().Update(ctx, &vnf); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("VirtualNetworkFunction reconciled", "vnf", vnf.Name, "type", vnf.Spec.Type,
		"replicas", vnf.Spec.Replicas, "ready", vnf.Status.ReadyReplicas)
	return ctrl.Result{}, nil
}

func (r *VirtualNetworkFunctionReconciler) reconcileDelete(ctx context.Context, vnf *nfvv1alpha1.VirtualNetworkFunction) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if controllerutil.ContainsFinalizer(vnf, virtualNetworkFunctionFinalizer) {
		used, err := deps.VNFDependents(ctx, r.Client, vnf.Namespace, vnf.Name)
		if err != nil {
			return ctrl.Result{}, err
		}
		if len(used) > 0 {
			log.Info("VNF is in use, deletion blocked", "usedBy", used)
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}
		controllerutil.RemoveFinalizer(vnf, virtualNetworkFunctionFinalizer)
		if err := r.Update(ctx, vnf); err != nil {
			return ctrl.Result{}, err
		}
		log.Info("VirtualNetworkFunction deleted", "vnf", vnf.Name)
	}
	return ctrl.Result{}, nil
}

func (r *VirtualNetworkFunctionReconciler) setCondition(vnf *nfvv1alpha1.VirtualNetworkFunction, condType string, status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(&vnf.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
	})
}

// SetupWithManager sets up the controller with the Manager.
func (r *VirtualNetworkFunctionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nfvv1alpha1.VirtualNetworkFunction{}).
		Watches(&corev1.Node{}, placement.All(r.Client, func() client.ObjectList { return &nfvv1alpha1.VirtualNetworkFunctionList{} })).
		Named("nfv-virtualnetworkfunction").
		Complete(r)
}
