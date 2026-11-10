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
	"net"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	infrav1alpha1 "github.com/sdncontroller/sdncontroller/api/infra/v1alpha1"
	"github.com/sdncontroller/sdncontroller/pkg/netutil"
)

const tunnelFinalizer = "tunnel.sdncontroller.io/finalizer"

// TunnelReconciler reconciles a Tunnel object.
type TunnelReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=infra.sdncontroller.io,resources=tunnels,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infra.sdncontroller.io,resources=tunnels/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=infra.sdncontroller.io,resources=tunnels/finalizers,verbs=update

func (r *TunnelReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var tunnel infrav1alpha1.Tunnel
	if err := r.Get(ctx, req.NamespacedName, &tunnel); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle deletion
	if !tunnel.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, &tunnel)
	}

	// Add finalizer
	if !controllerutil.ContainsFinalizer(&tunnel, tunnelFinalizer) {
		controllerutil.AddFinalizer(&tunnel, tunnelFinalizer)
		if err := r.Update(ctx, &tunnel); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Set phase to Establishing
	if tunnel.Status.Phase == "" || tunnel.Status.Phase == infrav1alpha1.TunnelPhasePending {
		tunnel.Status.Phase = infrav1alpha1.TunnelPhaseEstablishing
		if err := r.Status().Update(ctx, &tunnel); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Validate endpoints
	if ip := net.ParseIP(tunnel.Spec.LocalEndpoint.IP); ip == nil {
		r.setCondition(&tunnel, "EndpointsValid", metav1.ConditionFalse, "InvalidLocalIP",
			fmt.Sprintf("Local endpoint IP %q is not valid", tunnel.Spec.LocalEndpoint.IP))
		tunnel.Status.Phase = infrav1alpha1.TunnelPhaseError
		_ = r.Status().Update(ctx, &tunnel)
		return ctrl.Result{}, nil
	}
	if ip := net.ParseIP(tunnel.Spec.RemoteEndpoint.IP); ip == nil {
		r.setCondition(&tunnel, "EndpointsValid", metav1.ConditionFalse, "InvalidRemoteIP",
			fmt.Sprintf("Remote endpoint IP %q is not valid", tunnel.Spec.RemoteEndpoint.IP))
		tunnel.Status.Phase = infrav1alpha1.TunnelPhaseError
		_ = r.Status().Update(ctx, &tunnel)
		return ctrl.Result{}, nil
	}
	if tunnel.Spec.LocalEndpoint.IP == tunnel.Spec.RemoteEndpoint.IP {
		r.setCondition(&tunnel, "EndpointsValid", metav1.ConditionFalse, "SameEndpoint",
			"Local and remote endpoints cannot be the same IP")
		tunnel.Status.Phase = infrav1alpha1.TunnelPhaseError
		_ = r.Status().Update(ctx, &tunnel)
		return ctrl.Result{}, nil
	}
	r.setCondition(&tunnel, "EndpointsValid", metav1.ConditionTrue, "Valid",
		fmt.Sprintf("Tunnel %s -> %s", tunnel.Spec.LocalEndpoint.IP, tunnel.Spec.RemoteEndpoint.IP))

	// Calculate MTU with overhead using netutil.InnerMTU
	tunnelType := string(tunnel.Spec.Type)
	if tunnelType == "" {
		tunnelType = "VXLAN"
	}

	physicalMTU := netutil.DefaultMTU
	if tunnel.Spec.MTU != nil {
		physicalMTU = int(*tunnel.Spec.MTU)
	}
	innerMTU := netutil.InnerMTU(physicalMTU, tunnelType)
	r.setCondition(&tunnel, "MTUCalculated", metav1.ConditionTrue, "Calculated",
		fmt.Sprintf("Inner MTU: %d (physical: %d, type: %s)", innerMTU, physicalMTU, tunnelType))

	// Set offload status
	offloadActive := false
	if tunnel.Spec.Offload != nil && tunnel.Spec.Offload.Enabled {
		offloadActive = true
	}
	tunnel.Status.OffloadActive = offloadActive

	// Mark Established
	tunnel.Status.Phase = infrav1alpha1.TunnelPhaseEstablished
	r.setCondition(&tunnel, "Ready", metav1.ConditionTrue, "Established", "Tunnel is established")

	if err := r.Status().Update(ctx, &tunnel); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Tunnel reconciled", "tunnel", tunnel.Name, "type", tunnelType,
		"local", tunnel.Spec.LocalEndpoint.IP, "remote", tunnel.Spec.RemoteEndpoint.IP, "innerMTU", innerMTU)
	return ctrl.Result{RequeueAfter: 60 * time.Second}, nil
}

func (r *TunnelReconciler) reconcileDelete(ctx context.Context, tunnel *infrav1alpha1.Tunnel) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if controllerutil.ContainsFinalizer(tunnel, tunnelFinalizer) {
		controllerutil.RemoveFinalizer(tunnel, tunnelFinalizer)
		if err := r.Update(ctx, tunnel); err != nil {
			return ctrl.Result{}, err
		}
		log.Info("Tunnel deleted", "tunnel", tunnel.Name)
	}
	return ctrl.Result{}, nil
}

func (r *TunnelReconciler) setCondition(tunnel *infrav1alpha1.Tunnel, condType string, status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(&tunnel.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
	})
}

// SetupWithManager sets up the controller with the Manager.
func (r *TunnelReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrav1alpha1.Tunnel{}).
		Named("infra-tunnel").
		Complete(r)
}
