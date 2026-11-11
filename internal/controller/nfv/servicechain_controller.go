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
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	nfvv1alpha1 "github.com/sdncontroller/sdncontroller/api/nfv/v1alpha1"
	vpcv1alpha1 "github.com/sdncontroller/sdncontroller/api/vpc/v1alpha1"
	"github.com/sdncontroller/sdncontroller/internal/placement"
)

const serviceChainFinalizer = "servicechain.sdncontroller.io/finalizer"

// ServiceChainReconciler reconciles a ServiceChain object.
type ServiceChainReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=nfv.sdncontroller.io,resources=servicechains,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=nfv.sdncontroller.io,resources=servicechains/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=nfv.sdncontroller.io,resources=servicechains/finalizers,verbs=update
// +kubebuilder:rbac:groups=nfv.sdncontroller.io,resources=virtualnetworkfunctions,verbs=get;list;watch
// +kubebuilder:rbac:groups=vpc.sdncontroller.io,resources=vpcs,verbs=get;list;watch

func (r *ServiceChainReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var sc nfvv1alpha1.ServiceChain
	if err := r.Get(ctx, req.NamespacedName, &sc); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle deletion
	if !sc.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, &sc)
	}

	// Add finalizer
	if !controllerutil.ContainsFinalizer(&sc, serviceChainFinalizer) {
		controllerutil.AddFinalizer(&sc, serviceChainFinalizer)
		if err := r.Update(ctx, &sc); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Set phase to Pending if not set
	if sc.Status.Phase == "" {
		sc.Status.Phase = nfvv1alpha1.ServiceChainPhasePending
		if err := r.Status().Update(ctx, &sc); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Validate VPC ref exists
	var vpc vpcv1alpha1.VPC
	if err := r.Get(ctx, types.NamespacedName{Name: sc.Spec.VPCRef, Namespace: sc.Namespace}, &vpc); err != nil {
		r.setCondition(&sc, "VPCReady", metav1.ConditionFalse, "VPCNotFound", fmt.Sprintf("VPC %q not found", sc.Spec.VPCRef))
		sc.Status.Phase = nfvv1alpha1.ServiceChainPhaseFailed
		_ = r.Status().Update(ctx, &sc)
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}
	if vpc.Status.Phase != vpcv1alpha1.VPCPhaseReady {
		r.setCondition(&sc, "VPCReady", metav1.ConditionFalse, "VPCNotReady", "Parent VPC is not ready")
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}
	r.setCondition(&sc, "VPCReady", metav1.ConditionTrue, "Ready", "Parent VPC is ready")

	// Validate VNF refs exist in chain
	for _, element := range sc.Spec.Chain {
		var vnf nfvv1alpha1.VirtualNetworkFunction
		if err := r.Get(ctx, types.NamespacedName{Name: element.VNFRef, Namespace: sc.Namespace}, &vnf); err != nil {
			r.setCondition(&sc, "ChainValid", metav1.ConditionFalse, "VNFNotFound",
				fmt.Sprintf("VNF %q referenced in chain element %q not found", element.VNFRef, element.Name))
			sc.Status.Phase = nfvv1alpha1.ServiceChainPhaseFailed
			_ = r.Status().Update(ctx, &sc)
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}
		if vnf.Status.Phase != nfvv1alpha1.VNFPhaseRunning {
			r.setCondition(&sc, "ChainValid", metav1.ConditionFalse, "VNFNotReady",
				fmt.Sprintf("VNF %q is not running (phase: %s)", element.VNFRef, vnf.Status.Phase))
			sc.Status.Phase = nfvv1alpha1.ServiceChainPhaseDegraded
			_ = r.Status().Update(ctx, &sc)
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}
	}
	r.setCondition(&sc, "ChainValid", metav1.ConditionTrue, "Valid",
		fmt.Sprintf("%d VNF hop(s) validated", len(sc.Spec.Chain)))

	// Set ChainLength
	sc.Status.ChainLength = int32(len(sc.Spec.Chain))

	// Mark Active
	if nodes, err := placement.Nodes(ctx, r.Client, sc.Namespace, sc.Spec.VPCRef, sc.Spec.NodeSelector); err != nil {
		return ctrl.Result{}, err
	} else {
		sc.Status.Nodes = nodes
		if len(nodes) == 0 {
			r.setCondition(&sc, "Placed", metav1.ConditionFalse, "NoEligibleNodes", "no Ready node matches nodeSelector and VPC attachment")
		} else {
			r.setCondition(&sc, "Placed", metav1.ConditionTrue, "Placed", fmt.Sprintf("running on %v", nodes))
		}
	}
	sc.Status.Phase = nfvv1alpha1.ServiceChainPhaseActive
	r.setCondition(&sc, "Ready", metav1.ConditionTrue, "Active",
		fmt.Sprintf("Service chain active with %d hop(s)", sc.Status.ChainLength))

	if err := r.Status().Update(ctx, &sc); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("ServiceChain reconciled", "sc", sc.Name, "vpc", sc.Spec.VPCRef, "chainLength", sc.Status.ChainLength)
	return ctrl.Result{}, nil
}

func (r *ServiceChainReconciler) reconcileDelete(ctx context.Context, sc *nfvv1alpha1.ServiceChain) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if controllerutil.ContainsFinalizer(sc, serviceChainFinalizer) {
		controllerutil.RemoveFinalizer(sc, serviceChainFinalizer)
		if err := r.Update(ctx, sc); err != nil {
			return ctrl.Result{}, err
		}
		log.Info("ServiceChain deleted", "sc", sc.Name)
	}
	return ctrl.Result{}, nil
}

func (r *ServiceChainReconciler) setCondition(sc *nfvv1alpha1.ServiceChain, condType string, status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(&sc.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
	})
}

// SetupWithManager sets up the controller with the Manager.
func (r *ServiceChainReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nfvv1alpha1.ServiceChain{}).
		Watches(&vpcv1alpha1.VPC{}, placement.ByVPC(r.Client, func() client.ObjectList { return &nfvv1alpha1.ServiceChainList{} }, func(o client.Object) string { return o.(*nfvv1alpha1.ServiceChain).Spec.VPCRef })).
		Named("nfv-servicechain").
		Complete(r)
}
