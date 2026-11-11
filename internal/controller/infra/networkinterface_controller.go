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

const networkInterfaceFinalizer = "networkinterface.sdncontroller.io/finalizer"

// NetworkInterfaceReconciler reconciles a NetworkInterface object.
type NetworkInterfaceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=infra.sdncontroller.io,resources=networkinterfaces,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infra.sdncontroller.io,resources=networkinterfaces/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=infra.sdncontroller.io,resources=networkinterfaces/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch

func (r *NetworkInterfaceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var nic infrav1alpha1.NetworkInterface
	if err := r.Get(ctx, req.NamespacedName, &nic); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle deletion
	if !nic.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, &nic)
	}

	// Add finalizer
	if !controllerutil.ContainsFinalizer(&nic, networkInterfaceFinalizer) {
		controllerutil.AddFinalizer(&nic, networkInterfaceFinalizer)
		if err := r.Update(ctx, &nic); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Set phase to Pending if not set
	if nic.Status.Phase == "" {
		nic.Status.Phase = infrav1alpha1.NICPhasePending
		if err := r.Status().Update(ctx, &nic); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Validate node exists
	var node corev1.Node
	if err := r.Get(ctx, types.NamespacedName{Name: nic.Spec.NodeName}, &node); err != nil {
		r.setCondition(&nic, "NodeReady", metav1.ConditionFalse, "NodeNotFound",
			fmt.Sprintf("Node %q not found", nic.Spec.NodeName))
		nic.Status.Phase = infrav1alpha1.NICPhaseError
		_ = r.Status().Update(ctx, &nic)
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}
	r.setCondition(&nic, "NodeReady", metav1.ConditionTrue, "NodeExists",
		fmt.Sprintf("Node %s found", nic.Spec.NodeName))

	// Detect hardware capabilities
	offloadCapable := false
	offloadActive := false
	driver := "unknown"

	if nic.Spec.Type == infrav1alpha1.NICTypePhysical || nic.Spec.Type == infrav1alpha1.NICTypeVF {
		offloadCapable = true
		driver = "mlx5_core"
	} else if nic.Spec.Type == infrav1alpha1.NICTypeVirtual {
		driver = "virtio_net"
	} else if nic.Spec.Type == infrav1alpha1.NICTypeBond {
		driver = "bonding"
	}

	if nic.Spec.Offload != nil && nic.Spec.Offload.Enabled && offloadCapable {
		offloadActive = true
		if nic.Spec.Offload.Driver != "" {
			driver = nic.Spec.Offload.Driver
		}
	}

	nic.Status.OffloadCapable = offloadCapable
	nic.Status.OffloadActive = offloadActive
	nic.Status.Driver = driver

	// Set MAC address (simulated based on device name)
	if nic.Status.MACAddress == "" {
		nic.Status.MACAddress = fmt.Sprintf("00:00:00:%02x:%02x:%02x",
			len(nic.Spec.NodeName)%256, len(nic.Spec.Device)%256, len(nic.Name)%256)
	}

	// Set speed based on type
	switch nic.Spec.Type {
	case infrav1alpha1.NICTypePhysical, infrav1alpha1.NICTypeVF:
		nic.Status.Speed = "100Gbps"
	case infrav1alpha1.NICTypeVirtual:
		nic.Status.Speed = "10Gbps"
	case infrav1alpha1.NICTypeBond:
		nic.Status.Speed = "200Gbps"
	default:
		nic.Status.Speed = "10Gbps"
	}

	r.setCondition(&nic, "HardwareDetected", metav1.ConditionTrue, "Detected",
		fmt.Sprintf("Driver: %s, Speed: %s, Offload: %v", driver, nic.Status.Speed, offloadActive))

	// Mark Configured
	nic.Status.Phase = infrav1alpha1.NICPhaseConfigured
	r.setCondition(&nic, "Ready", metav1.ConditionTrue, "Configured", "NetworkInterface is configured")

	if err := r.Status().Update(ctx, &nic); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("NetworkInterface reconciled", "nic", nic.Name, "node", nic.Spec.NodeName, "device", nic.Spec.Device)
	return ctrl.Result{}, nil
}

func (r *NetworkInterfaceReconciler) reconcileDelete(ctx context.Context, nic *infrav1alpha1.NetworkInterface) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if controllerutil.ContainsFinalizer(nic, networkInterfaceFinalizer) {
		controllerutil.RemoveFinalizer(nic, networkInterfaceFinalizer)
		if err := r.Update(ctx, nic); err != nil {
			return ctrl.Result{}, err
		}
		log.Info("NetworkInterface deleted", "nic", nic.Name)
	}
	return ctrl.Result{}, nil
}

func (r *NetworkInterfaceReconciler) setCondition(nic *infrav1alpha1.NetworkInterface, condType string, status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(&nic.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
	})
}

// SetupWithManager sets up the controller with the Manager.
func (r *NetworkInterfaceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrav1alpha1.NetworkInterface{}).
		Named("infra-networkinterface").
		Complete(r)
}
