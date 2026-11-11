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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vpcv1alpha1 "github.com/sdncontroller/sdncontroller/api/vpc/v1alpha1"
	"github.com/sdncontroller/sdncontroller/internal/deps"
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
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.sdncontroller.io;service.sdncontroller.io;nfv.sdncontroller.io;infra.sdncontroller.io,resources=*,verbs=get;list;watch

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

	// Default tunnel type and require an explicit VNI for VNI-based encapsulations
	tunnelType := vpc.Spec.TunnelType
	if tunnelType == "" {
		tunnelType = vpcv1alpha1.TunnelTypeVXLAN
	}
	if tunnelType != vpcv1alpha1.TunnelTypeIPIP && vpc.Spec.VNI == nil {
		msg := fmt.Sprintf("spec.vni is required for tunnelType %s so all nodes use the same VNI", tunnelType)
		r.setCondition(&vpc, "VNIAllocated", metav1.ConditionFalse, "VNIRequired", msg)
		vpc.Status.Phase = vpcv1alpha1.VPCPhaseFailed
		_ = r.Status().Update(ctx, &vpc)
		return ctrl.Result{}, nil
	}

	// Allocate VNI (idempotent: reuse existing allocation from status)
	var vni int32
	if vpc.Status.AllocatedVNI > 0 && vpc.Spec.VNI != nil && *vpc.Spec.VNI != vpc.Status.AllocatedVNI {
		r.VNIAllocator.Release(vpc.Status.AllocatedVNI)
		vpc.Status.AllocatedVNI = 0
	}
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

	// Attach nodes selected by spec.nodeSelector (all nodes when unset)
	sel := labels.Everything()
	if vpc.Spec.NodeSelector != nil {
		if s, err := metav1.LabelSelectorAsSelector(vpc.Spec.NodeSelector); err == nil {
			sel = s
		}
	}
	var nodeList corev1.NodeList
	if err := r.List(ctx, &nodeList, client.MatchingLabelsSelector{Selector: sel}); err != nil {
		return ctrl.Result{}, err
	}
	bridge := vpcBridgeName(vpc.Name)
	attached := make([]vpcv1alpha1.VPCNodeAttachment, 0, len(nodeList.Items))
	nodeIP := map[string]string{}
	for _, n := range nodeList.Items {
		ip := ""
		for _, a := range n.Status.Addresses {
			if a.Type == corev1.NodeInternalIP {
				ip = a.Address
				break
			}
		}
		nodeIP[n.Name] = ip
		attached = append(attached, vpcv1alpha1.VPCNodeAttachment{NodeName: n.Name, TunnelIP: ip, Bridge: bridge})
	}
	vpc.Status.AttachedNodes = attached
	vpc.Status.ActiveNodes = int32(len(attached))

	// Route table distributed through the K8s API (replaces BGP): each subnet is
	// local on its nodes and reachable via tunnel from every other node.
	routes := []vpcv1alpha1.VPCRoute{}
	for _, sn := range subnetList.Items {
		if sn.Spec.VPCRef != vpc.Name || sn.Namespace != vpc.Namespace || sn.Status.Phase == vpcv1alpha1.SubnetPhaseFailed {
			continue
		}
		for _, a := range attached {
			routes = append(routes, vpcv1alpha1.VPCRoute{
				Destination: sn.Spec.CIDR, NextHopNode: a.NodeName, NextHopIP: nodeIP[a.NodeName], Type: "Tunnel",
			})
		}
	}
	vpc.Status.Routes = routes

	// Record dependents (informational; deletion is blocked by the finalizer)
	used, err := deps.VPCDependents(ctx, r.Client, vpc.Namespace, vpc.Name)
	if err != nil {
		return ctrl.Result{}, err
	}
	vpc.Status.UsedBy = used

	// Mark Ready
	vpc.Status.Phase = vpcv1alpha1.VPCPhaseReady
	r.setCondition(&vpc, "Ready", metav1.ConditionTrue, "Provisioned", "VPC is ready")

	if err := r.Status().Update(ctx, &vpc); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("VPC reconciled", "vpc", vpc.Name, "vni", vni, "cidr", vpc.Spec.CIDR)
	return ctrl.Result{}, nil
}

func (r *VPCReconciler) reconcileDelete(ctx context.Context, vpc *vpcv1alpha1.VPC) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if controllerutil.ContainsFinalizer(vpc, vpcFinalizer) {
		used, err := deps.VPCDependents(ctx, r.Client, vpc.Namespace, vpc.Name)
		if err != nil {
			return ctrl.Result{}, err
		}
		if len(used) > 0 {
			log.Info("VPC is in use, deletion blocked", "usedBy", used)
			vpc.Status.Phase = vpcv1alpha1.VPCPhaseDeleting
			vpc.Status.UsedBy = used
			r.setCondition(vpc, "Ready", metav1.ConditionFalse, "InUse", fmt.Sprintf("deletion blocked, in use by: %v", used))
			_ = r.Status().Update(ctx, vpc)
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
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

func vpcBridgeName(name string) string {
	b := "br-" + name
	if len(b) > 15 {
		b = b[:15]
	}
	return b
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
	subnetToVPC := handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, o client.Object) []reconcile.Request {
		sn, ok := o.(*vpcv1alpha1.Subnet)
		if !ok {
			return nil
		}
		return []reconcile.Request{{NamespacedName: types.NamespacedName{Name: sn.Spec.VPCRef, Namespace: sn.Namespace}}}
	})
	nodeToVPCs := handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, o client.Object) []reconcile.Request {
		var l vpcv1alpha1.VPCList
		if err := r.List(ctx, &l); err != nil {
			return nil
		}
		reqs := make([]reconcile.Request, 0, len(l.Items))
		for _, v := range l.Items {
			reqs = append(reqs, reconcile.Request{NamespacedName: types.NamespacedName{Name: v.Name, Namespace: v.Namespace}})
		}
		return reqs
	})
	return ctrl.NewControllerManagedBy(mgr).
		For(&vpcv1alpha1.VPC{}).
		Watches(&vpcv1alpha1.Subnet{}, subnetToVPC).
		Watches(&corev1.Node{}, nodeToVPCs, builder.WithPredicates(predicate.LabelChangedPredicate{})).
		Named("vpc-vpc").
		Complete(r)
}
