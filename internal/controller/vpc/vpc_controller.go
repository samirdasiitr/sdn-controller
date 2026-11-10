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

package vpc

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	vpcv1alpha1 "github.com/sdncontroller/sdncontroller/api/vpc/v1alpha1"
	"github.com/sdncontroller/sdncontroller/pkg/netutil"
)

const vpcFinalizer = "vpc.sdncontroller.io/finalizer"

// VPCReconciler reconciles a VPC object.
type VPCReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	VNIAllocator *netutil.VNIAllocator
}

// +kubebuilder:rbac:groups=vpc.sdncontroller.io,resources=vpcs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vpc.sdncontroller.io,resources=vpcs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=vpc.sdncontroller.io,resources=vpcs/finalizers,verbs=update
// +kubebuilder:rbac:groups=vpc.sdncontroller.io,resources=subnets,verbs=get;list;watch

func (r *VPCReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var vpc vpcv1alpha1.VPC
	if err := r.Get(ctx, req.NamespacedName, &vpc); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle deletion
	if !vpc.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, &vpc)
	}

	// Add finalizer
	if !controllerutil.ContainsFinalizer(&vpc, vpcFinalizer) {
		controllerutil.AddFinalizer(&vpc, vpcFinalizer)
		if err := r.Update(ctx, &vpc); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Set phase locally (will be persisted with the final status update)
	if vpc.Status.Phase == "" || vpc.Status.Phase == vpcv1alpha1.VPCPhasePending {
		vpc.Status.Phase = vpcv1alpha1.VPCPhaseProvisioning
	}

	// Validate CIDR
	_, err := netutil.ParseCIDR(vpc.Spec.CIDR)
	if err != nil {
		log.Error(err, "invalid VPC CIDR")
		r.setCondition(&vpc, "CIDRValid", metav1.ConditionFalse, "InvalidCIDR", err.Error())
		vpc.Status.Phase = vpcv1alpha1.VPCPhaseFailed
		_ = r.Status().Update(ctx, &vpc)
		return ctrl.Result{}, nil
	}
	r.setCondition(&vpc, "CIDRValid", metav1.ConditionTrue, "Valid", "CIDR is valid")

	// Check for CIDR overlap with other VPCs
	var vpcList vpcv1alpha1.VPCList
	if err := r.List(ctx, &vpcList); err != nil {
		return ctrl.Result{}, err
	}
	for _, other := range vpcList.Items {
		if other.Name == vpc.Name {
			continue
		}
		otherNet, err := netutil.ParseCIDR(other.Spec.CIDR)
		if err != nil {
			continue
		}
		vpcNet, _ := netutil.ParseCIDR(vpc.Spec.CIDR)
		if netutil.OverlapsCIDR(vpcNet, otherNet) {
			msg := fmt.Sprintf("CIDR %s overlaps with VPC %s (%s)", vpc.Spec.CIDR, other.Name, other.Spec.CIDR)
			r.setCondition(&vpc, "CIDRValid", metav1.ConditionFalse, "CIDROverlap", msg)
			vpc.Status.Phase = vpcv1alpha1.VPCPhaseFailed
			_ = r.Status().Update(ctx, &vpc)
			return ctrl.Result{}, nil
		}
	}

	// Allocate VNI (idempotent: reuse existing allocation from status)
	var vni int32
	if vpc.Status.AllocatedVNI > 0 {
		// Restore in-memory state from persisted status
		r.VNIAllocator.Restore(vpc.Status.AllocatedVNI, vpc.Name)
		vni = vpc.Status.AllocatedVNI
	} else {
		var requestedVNI int32
		if vpc.Spec.VNI != nil {
			requestedVNI = *vpc.Spec.VNI
		}
		var allocErr error
		vni, allocErr = r.VNIAllocator.Allocate(vpc.Name, requestedVNI)
		if allocErr != nil {
			log.Error(allocErr, "failed to allocate VNI")
			r.setCondition(&vpc, "VNIAllocated", metav1.ConditionFalse, "AllocationFailed", allocErr.Error())
			vpc.Status.Phase = vpcv1alpha1.VPCPhaseFailed
			_ = r.Status().Update(ctx, &vpc)
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}
	}
	vpc.Status.AllocatedVNI = vni
	r.setCondition(&vpc, "VNIAllocated", metav1.ConditionTrue, "Allocated", fmt.Sprintf("VNI %d allocated", vni))

	// Count subnets
	var subnetList vpcv1alpha1.SubnetList
	if err := r.List(ctx, &subnetList); err != nil {
		return ctrl.Result{}, err
	}
	var subnetCount int32
	for _, sn := range subnetList.Items {
		if sn.Spec.VPCRef == vpc.Name {
			subnetCount++
		}
	}
	vpc.Status.AllocatedSubnets = subnetCount

	// Mark Ready
	vpc.Status.Phase = vpcv1alpha1.VPCPhaseReady
	r.setCondition(&vpc, "Ready", metav1.ConditionTrue, "Provisioned", "VPC is ready")

	if err := r.Status().Update(ctx, &vpc); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("VPC reconciled", "vpc", vpc.Name, "vni", vni, "cidr", vpc.Spec.CIDR)
	return ctrl.Result{RequeueAfter: 60 * time.Second}, nil
}

func (r *VPCReconciler) reconcileDelete(ctx context.Context, vpc *vpcv1alpha1.VPC) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if controllerutil.ContainsFinalizer(vpc, vpcFinalizer) {
		// Check for remaining subnets
		var subnetList vpcv1alpha1.SubnetList
		if err := r.List(ctx, &subnetList); err != nil {
			return ctrl.Result{}, err
		}
		for _, sn := range subnetList.Items {
			if sn.Spec.VPCRef == vpc.Name {
				log.Info("VPC still has subnets, waiting", "subnet", sn.Name)
				return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
			}
		}

		// Release VNI
		if vpc.Status.AllocatedVNI > 0 {
			r.VNIAllocator.Release(vpc.Status.AllocatedVNI)
		}

		controllerutil.RemoveFinalizer(vpc, vpcFinalizer)
		if err := r.Update(ctx, vpc); err != nil {
			return ctrl.Result{}, err
		}
		log.Info("VPC deleted", "vpc", vpc.Name)
	}
	return ctrl.Result{}, nil
}

func (r *VPCReconciler) setCondition(vpc *vpcv1alpha1.VPC, condType string, status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(&vpc.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
	})
}

// SetupWithManager sets up the controller with the Manager.
func (r *VPCReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&vpcv1alpha1.VPC{}).
		Named("vpc-vpc").
		Complete(r)
}
