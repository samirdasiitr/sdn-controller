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

const nodeNetworkConfigFinalizer = "nodenetworkconfig.sdncontroller.io/finalizer"

// NodeNetworkConfigReconciler reconciles a NodeNetworkConfig object.
type NodeNetworkConfigReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=infra.sdncontroller.io,resources=nodenetworkconfigs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infra.sdncontroller.io,resources=nodenetworkconfigs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=infra.sdncontroller.io,resources=nodenetworkconfigs/finalizers,verbs=update
// +kubebuilder:rbac:groups=infra.sdncontroller.io,resources=tunnels,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch

func (r *NodeNetworkConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var nnc infrav1alpha1.NodeNetworkConfig
	if err := r.Get(ctx, req.NamespacedName, &nnc); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle deletion
	if !nnc.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, &nnc)
	}

	// Add finalizer
	if !controllerutil.ContainsFinalizer(&nnc, nodeNetworkConfigFinalizer) {
		controllerutil.AddFinalizer(&nnc, nodeNetworkConfigFinalizer)
		if err := r.Update(ctx, &nnc); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Set phase to Pending if not set
	if nnc.Status.Phase == "" {
		nnc.Status.Phase = infrav1alpha1.NodeNetworkPhasePending
		if err := r.Status().Update(ctx, &nnc); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Validate node exists
	var node corev1.Node
	if err := r.Get(ctx, types.NamespacedName{Name: nnc.Spec.NodeName}, &node); err != nil {
		r.setCondition(&nnc, "NodeReady", metav1.ConditionFalse, "NodeNotFound",
			fmt.Sprintf("Node %q not found", nnc.Spec.NodeName))
		nnc.Status.Phase = infrav1alpha1.NodeNetworkPhaseError
		_ = r.Status().Update(ctx, &nnc)
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}
	r.setCondition(&nnc, "NodeReady", metav1.ConditionTrue, "NodeExists",
		fmt.Sprintf("Node %s found", nnc.Spec.NodeName))

	// Manage OVS bridge configs
	if len(nnc.Spec.OVSBridges) > 0 {
		for _, bridge := range nnc.Spec.OVSBridges {
			if bridge.Name == "" {
				r.setCondition(&nnc, "OVSBridgesValid", metav1.ConditionFalse, "InvalidBridge",
					"OVS bridge name cannot be empty")
				nnc.Status.Phase = infrav1alpha1.NodeNetworkPhaseError
				_ = r.Status().Update(ctx, &nnc)
				return ctrl.Result{}, nil
			}
		}
		r.setCondition(&nnc, "OVSBridgesValid", metav1.ConditionTrue, "Valid",
			fmt.Sprintf("%d OVS bridge(s) configured", len(nnc.Spec.OVSBridges)))
	} else {
		r.setCondition(&nnc, "OVSBridgesValid", metav1.ConditionTrue, "Valid", "No OVS bridges configured")
	}

	// Count tunnels established for this node
	var tunnelList infrav1alpha1.TunnelList
	if err := r.List(ctx, &tunnelList); err != nil {
		return ctrl.Result{}, err
	}
	var tunnelCount int32
	for _, t := range tunnelList.Items {
		if t.Spec.LocalEndpoint.NodeName == nnc.Spec.NodeName &&
			t.Status.Phase == infrav1alpha1.TunnelPhaseEstablished {
			tunnelCount++
		}
	}
	nnc.Status.TunnelsEstablished = tunnelCount

	// Set OVS/Agent version (simulated)
	if nnc.Status.OVSVersion == "" {
		nnc.Status.OVSVersion = "3.3.0"
	}
	if nnc.Status.AgentVersion == "" {
		nnc.Status.AgentVersion = "1.0.0"
	}

	// Set offload status
	if nnc.Spec.DPUEnabled {
		nnc.Status.OffloadStatus = "DPU-Active"
	} else if nnc.Spec.Hardware != nil {
		hasOffload := false
		for _, n := range nnc.Spec.Hardware.NICs {
			if n.OffloadCapable {
				hasOffload = true
				break
			}
		}
		if hasOffload {
			nnc.Status.OffloadStatus = "NIC-Offload-Available"
		} else {
			nnc.Status.OffloadStatus = "Software"
		}
	} else {
		nnc.Status.OffloadStatus = "Software"
	}

	// Mark Configured
	nnc.Status.Phase = infrav1alpha1.NodeNetworkPhaseConfigured
	r.setCondition(&nnc, "Ready", metav1.ConditionTrue, "Configured", "NodeNetworkConfig is configured")

	if err := r.Status().Update(ctx, &nnc); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("NodeNetworkConfig reconciled", "nnc", nnc.Name, "node", nnc.Spec.NodeName,
		"tunnels", tunnelCount, "offload", nnc.Status.OffloadStatus)
	return ctrl.Result{}, nil
}

func (r *NodeNetworkConfigReconciler) reconcileDelete(ctx context.Context, nnc *infrav1alpha1.NodeNetworkConfig) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if controllerutil.ContainsFinalizer(nnc, nodeNetworkConfigFinalizer) {
		controllerutil.RemoveFinalizer(nnc, nodeNetworkConfigFinalizer)
		if err := r.Update(ctx, nnc); err != nil {
			return ctrl.Result{}, err
		}
		log.Info("NodeNetworkConfig deleted", "nnc", nnc.Name)
	}
	return ctrl.Result{}, nil
}

func (r *NodeNetworkConfigReconciler) setCondition(nnc *infrav1alpha1.NodeNetworkConfig, condType string, status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(&nnc.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
	})
}

// SetupWithManager sets up the controller with the Manager.
func (r *NodeNetworkConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrav1alpha1.NodeNetworkConfig{}).
		Named("infra-nodenetworkconfig").
		Complete(r)
}
