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

package infra

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

	infrav1alpha1 "github.com/sdncontroller/sdncontroller/api/infra/v1alpha1"
)

const bondFinalizer = "bond.sdncontroller.io/finalizer"

// BondReconciler reconciles a Bond object.
type BondReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=infra.sdncontroller.io,resources=bonds,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infra.sdncontroller.io,resources=bonds/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=infra.sdncontroller.io,resources=bonds/finalizers,verbs=update
// +kubebuilder:rbac:groups=infra.sdncontroller.io,resources=networkinterfaces,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch

func (r *BondReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var bond infrav1alpha1.Bond
	if err := r.Get(ctx, req.NamespacedName, &bond); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle deletion
	if !bond.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, &bond)
	}

	// Add finalizer
	if !controllerutil.ContainsFinalizer(&bond, bondFinalizer) {
		controllerutil.AddFinalizer(&bond, bondFinalizer)
		if err := r.Update(ctx, &bond); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Set phase to Pending if not set
	if bond.Status.Phase == "" {
		bond.Status.Phase = infrav1alpha1.BondPhasePending
		if err := r.Status().Update(ctx, &bond); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Validate node exists
	var node corev1.Node
	if err := r.Get(ctx, types.NamespacedName{Name: bond.Spec.NodeName}, &node); err != nil {
		r.setCondition(&bond, "NodeReady", metav1.ConditionFalse, "NodeNotFound",
			fmt.Sprintf("Node %q not found", bond.Spec.NodeName))
		bond.Status.Phase = infrav1alpha1.BondPhaseError
		_ = r.Status().Update(ctx, &bond)
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}
	r.setCondition(&bond, "NodeReady", metav1.ConditionTrue, "NodeExists",
		fmt.Sprintf("Node %s found", bond.Spec.NodeName))

	// Validate member interfaces exist
	var nicList infrav1alpha1.NetworkInterfaceList
	if err := r.List(ctx, &nicList); err != nil {
		return ctrl.Result{}, err
	}

	nicMap := make(map[string]bool)
	for _, nic := range nicList.Items {
		if nic.Spec.NodeName == bond.Spec.NodeName {
			nicMap[nic.Spec.Device] = true
		}
	}

	var activeCount int32
	for _, iface := range bond.Spec.Interfaces {
		if nicMap[iface] {
			activeCount++
		} else {
			r.setCondition(&bond, "InterfacesValid", metav1.ConditionFalse, "InterfaceNotFound",
				fmt.Sprintf("Interface %q not found on node %s", iface, bond.Spec.NodeName))
			bond.Status.Phase = infrav1alpha1.BondPhaseDegraded
			_ = r.Status().Update(ctx, &bond)
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}
	}
	r.setCondition(&bond, "InterfacesValid", metav1.ConditionTrue, "Valid",
		fmt.Sprintf("%d member interface(s) validated", len(bond.Spec.Interfaces)))

	// Validate mode
	validModes := map[infrav1alpha1.BondMode]bool{
		infrav1alpha1.BondModeBalanceRR:    true,
		infrav1alpha1.BondModeActiveBackup: true,
		infrav1alpha1.BondMode8023AD:       true,
		infrav1alpha1.BondModeBalanceXOR:   true,
		infrav1alpha1.BondModeBalanceTLB:   true,
		infrav1alpha1.BondModeBalanceALB:   true,
	}
	if !validModes[bond.Spec.Mode] && bond.Spec.Mode != "" {
		r.setCondition(&bond, "ModeValid", metav1.ConditionFalse, "InvalidMode",
			fmt.Sprintf("Bond mode %q is not supported", bond.Spec.Mode))
		bond.Status.Phase = infrav1alpha1.BondPhaseError
		_ = r.Status().Update(ctx, &bond)
		return ctrl.Result{}, nil
	}
	r.setCondition(&bond, "ModeValid", metav1.ConditionTrue, "Valid",
		fmt.Sprintf("Bond mode: %s", bond.Spec.Mode))

	// Set status
	bond.Status.ActiveInterfaces = activeCount
	bond.Status.Speed = fmt.Sprintf("%dGbps", activeCount*100)

	// Mark Configured
	bond.Status.Phase = infrav1alpha1.BondPhaseConfigured
	r.setCondition(&bond, "Ready", metav1.ConditionTrue, "Configured", "Bond is configured")

	if err := r.Status().Update(ctx, &bond); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Bond reconciled", "bond", bond.Name, "node", bond.Spec.NodeName, "mode", bond.Spec.Mode, "interfaces", activeCount)
	return ctrl.Result{}, nil
}

func (r *BondReconciler) reconcileDelete(ctx context.Context, bond *infrav1alpha1.Bond) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if controllerutil.ContainsFinalizer(bond, bondFinalizer) {
		controllerutil.RemoveFinalizer(bond, bondFinalizer)
		if err := r.Update(ctx, bond); err != nil {
			return ctrl.Result{}, err
		}
		log.Info("Bond deleted", "bond", bond.Name)
	}
	return ctrl.Result{}, nil
}

func (r *BondReconciler) setCondition(bond *infrav1alpha1.Bond, condType string, status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(&bond.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
	})
}

// SetupWithManager sets up the controller with the Manager.
func (r *BondReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrav1alpha1.Bond{}).
		Named("infra-bond").
		Complete(r)
}
