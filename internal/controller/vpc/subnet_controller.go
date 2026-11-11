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
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	vpcv1alpha1 "github.com/sdncontroller/sdncontroller/api/vpc/v1alpha1"
	"github.com/sdncontroller/sdncontroller/internal/deps"
	"github.com/sdncontroller/sdncontroller/internal/ipam"
	"github.com/sdncontroller/sdncontroller/pkg/netutil"
)

const subnetFinalizer = "subnet.sdncontroller.io/finalizer"

// SubnetReconciler reconciles a Subnet object.
type SubnetReconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	IPAMManager *ipam.IPAMManager
}

// +kubebuilder:rbac:groups=vpc.sdncontroller.io,resources=subnets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vpc.sdncontroller.io,resources=subnets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=vpc.sdncontroller.io,resources=subnets/finalizers,verbs=update
// +kubebuilder:rbac:groups=vpc.sdncontroller.io,resources=vpcs,verbs=get;list;watch

func (r *SubnetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var subnet vpcv1alpha1.Subnet
	if err := r.Get(ctx, req.NamespacedName, &subnet); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !subnet.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, &subnet)
	}

	if !controllerutil.ContainsFinalizer(&subnet, subnetFinalizer) {
		controllerutil.AddFinalizer(&subnet, subnetFinalizer)
		if err := r.Update(ctx, &subnet); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Set phase locally (will be persisted with the final status update)
	if subnet.Status.Phase == "" || subnet.Status.Phase == vpcv1alpha1.SubnetPhasePending {
		subnet.Status.Phase = vpcv1alpha1.SubnetPhaseProvisioning
	}

	// Validate parent VPC exists and is ready
	var parentVPC vpcv1alpha1.VPC
	if err := r.Get(ctx, types.NamespacedName{Name: subnet.Spec.VPCRef, Namespace: subnet.Namespace}, &parentVPC); err != nil {
		r.setCondition(&subnet, "VPCReady", metav1.ConditionFalse, "VPCNotFound", fmt.Sprintf("VPC %q not found", subnet.Spec.VPCRef))
		subnet.Status.Phase = vpcv1alpha1.SubnetPhaseFailed
		_ = r.Status().Update(ctx, &subnet)
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}
	if parentVPC.Status.Phase != vpcv1alpha1.VPCPhaseReady {
		r.setCondition(&subnet, "VPCReady", metav1.ConditionFalse, "VPCNotReady", "Parent VPC is not ready")
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}
	r.setCondition(&subnet, "VPCReady", metav1.ConditionTrue, "Ready", "Parent VPC is ready")

	// Validate subnet CIDR is within VPC CIDR
	subnetNet, err := netutil.ParseCIDR(subnet.Spec.CIDR)
	if err != nil {
		r.setCondition(&subnet, "CIDRValid", metav1.ConditionFalse, "InvalidCIDR", err.Error())
		subnet.Status.Phase = vpcv1alpha1.SubnetPhaseFailed
		_ = r.Status().Update(ctx, &subnet)
		return ctrl.Result{}, nil
	}
	vpcNet, _ := netutil.ParseCIDR(parentVPC.Spec.CIDR)
	if !netutil.ContainsCIDR(vpcNet, subnetNet) {
		msg := fmt.Sprintf("Subnet CIDR %s is not within VPC CIDR %s", subnet.Spec.CIDR, parentVPC.Spec.CIDR)
		r.setCondition(&subnet, "CIDRValid", metav1.ConditionFalse, "CIDROutOfRange", msg)
		subnet.Status.Phase = vpcv1alpha1.SubnetPhaseFailed
		_ = r.Status().Update(ctx, &subnet)
		return ctrl.Result{}, nil
	}

	// Check overlap with sibling subnets
	var subnetList vpcv1alpha1.SubnetList
	if err := r.List(ctx, &subnetList); err != nil {
		return ctrl.Result{}, err
	}
	for _, other := range subnetList.Items {
		if other.Name == subnet.Name || other.Spec.VPCRef != subnet.Spec.VPCRef {
			continue
		}
		// Skip subnets already in Failed phase - they were rejected and shouldn't block valid subnets
		if other.Status.Phase == vpcv1alpha1.SubnetPhaseFailed {
			continue
		}
		otherNet, err := netutil.ParseCIDR(other.Spec.CIDR)
		if err != nil {
			continue
		}
		if netutil.OverlapsCIDR(subnetNet, otherNet) {
			msg := fmt.Sprintf("CIDR overlaps with subnet %s (%s)", other.Name, other.Spec.CIDR)
			r.setCondition(&subnet, "CIDRValid", metav1.ConditionFalse, "CIDROverlap", msg)
			subnet.Status.Phase = vpcv1alpha1.SubnetPhaseFailed
			_ = r.Status().Update(ctx, &subnet)
			return ctrl.Result{}, nil
		}
	}
	r.setCondition(&subnet, "CIDRValid", metav1.ConditionTrue, "Valid", "CIDR is valid")

	// Initialize IPAM pool
	gateway := subnet.Spec.Gateway
	if gateway == "" {
		gateway = netutil.FirstUsableIP(subnetNet).String()
	}
	if err := r.IPAMManager.CreatePool(subnet.Name, subnet.Spec.CIDR, gateway); err != nil {
		log.V(1).Info("IPAM pool may already exist", "subnet", subnet.Name)
	}

	alloc, err := r.IPAMManager.GetAllocator(subnet.Name)
	if err != nil {
		return ctrl.Result{}, err
	}

	subnet.Status.Gateway = alloc.Gateway().String()
	subnet.Status.AllocatedIPs = alloc.AllocatedCount()
	subnet.Status.AvailableIPs = alloc.AvailableCount()
	subnet.Status.Phase = vpcv1alpha1.SubnetPhaseReady
	r.setCondition(&subnet, "Ready", metav1.ConditionTrue, "Provisioned", "Subnet is ready")

	if err := r.Status().Update(ctx, &subnet); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Subnet reconciled", "subnet", subnet.Name, "vpc", subnet.Spec.VPCRef, "cidr", subnet.Spec.CIDR)
	return ctrl.Result{}, nil
}

func (r *SubnetReconciler) reconcileDelete(ctx context.Context, subnet *vpcv1alpha1.Subnet) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if controllerutil.ContainsFinalizer(subnet, subnetFinalizer) {
		used, err := deps.SubnetDependents(ctx, r.Client, subnet.Namespace, subnet.Name)
		if err != nil {
			return ctrl.Result{}, err
		}
		if len(used) > 0 {
			log.Info("Subnet is in use, deletion blocked", "usedBy", used)
			r.setCondition(subnet, "Ready", metav1.ConditionFalse, "InUse", fmt.Sprintf("deletion blocked, in use by: %v", used))
			_ = r.Status().Update(ctx, subnet)
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}
		r.IPAMManager.DeletePool(subnet.Name)

		controllerutil.RemoveFinalizer(subnet, subnetFinalizer)
		if err := r.Update(ctx, subnet); err != nil {
			return ctrl.Result{}, err
		}
		log.Info("Subnet deleted", "subnet", subnet.Name)
	}
	return ctrl.Result{}, nil
}

func (r *SubnetReconciler) setCondition(subnet *vpcv1alpha1.Subnet, condType string, status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(&subnet.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
	})
}

func (r *SubnetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&vpcv1alpha1.Subnet{}).
		Named("vpc-subnet").
		Complete(r)
}
