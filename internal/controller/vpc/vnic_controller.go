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
	"crypto/sha1"
	"fmt"
	"net"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vpcv1alpha1 "github.com/sdncontroller/sdncontroller/api/vpc/v1alpha1"
	"github.com/sdncontroller/sdncontroller/internal/ipam"
)

var fwGVK = schema.GroupVersionKind{Group: "service.sdncontroller.io", Version: "v1alpha1", Kind: "Firewall"}

const vnicFinalizer = "vnic.sdncontroller.io/finalizer"

// VNicReconciler reconciles a VNic object.
type VNicReconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	IPAMManager *ipam.IPAMManager
}

// +kubebuilder:rbac:groups=vpc.sdncontroller.io,resources=vnics,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vpc.sdncontroller.io,resources=vnics/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=vpc.sdncontroller.io,resources=vnics/finalizers,verbs=update

func vnicOwner(v *vpcv1alpha1.VNic) string { return v.Namespace + "/" + v.Name }

func (r *VNicReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var vnic vpcv1alpha1.VNic
	if err := r.Get(ctx, req.NamespacedName, &vnic); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !vnic.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, &vnic)
	}
	if !controllerutil.ContainsFinalizer(&vnic, vnicFinalizer) {
		controllerutil.AddFinalizer(&vnic, vnicFinalizer)
		if err := r.Update(ctx, &vnic); err != nil {
			return ctrl.Result{}, err
		}
	}

	st := &vnic.Status
	var subnet vpcv1alpha1.Subnet
	if err := r.Get(ctx, types.NamespacedName{Name: vnic.Spec.SubnetRef, Namespace: vnic.Namespace}, &subnet); err != nil {
		return r.pending(ctx, &vnic, "SubnetFound", "SubnetNotFound", fmt.Sprintf("Subnet %q not found", vnic.Spec.SubnetRef))
	}
	if subnet.Status.Phase != vpcv1alpha1.SubnetPhaseReady || !subnet.DeletionTimestamp.IsZero() {
		return r.pending(ctx, &vnic, "SubnetReady", "SubnetNotReady", "Subnet is not ready")
	}
	setCond(&vnic, "SubnetReady", metav1.ConditionTrue, "Ready", "Subnet is ready")

	alloc, err := r.IPAMManager.GetAllocator(subnet.Name)
	if err != nil {
		return r.pending(ctx, &vnic, "IPAllocated", "NoPool", err.Error())
	}
	_, ipnet, err := net.ParseCIDR(subnet.Spec.CIDR)
	if err != nil {
		return r.fail(ctx, &vnic, "IPAllocated", "InvalidSubnetCIDR", err.Error())
	}
	owner := vnicOwner(&vnic)

	// Release a previous address if the requested one changed.
	if st.IPAddress != "" && vnic.Spec.IPAddress != "" && st.IPAddress != vnic.Spec.IPAddress {
		alloc.Release(net.ParseIP(st.IPAddress))
		st.IPAddress = ""
	}
	switch {
	case st.IPAddress != "":
		if err := alloc.AllocateSpecific(net.ParseIP(st.IPAddress), owner); err != nil {
			return r.fail(ctx, &vnic, "IPAllocated", "IPConflict", err.Error())
		}
	case vnic.Spec.IPAddress != "":
		ip := net.ParseIP(vnic.Spec.IPAddress)
		if ip == nil || !ipnet.Contains(ip) {
			return r.fail(ctx, &vnic, "IPAllocated", "IPOutOfRange", fmt.Sprintf("%s is not in %s", vnic.Spec.IPAddress, subnet.Spec.CIDR))
		}
		if ip.Equal(alloc.Gateway()) {
			return r.fail(ctx, &vnic, "IPAllocated", "IPIsGateway", "address is the subnet gateway")
		}
		if err := alloc.AllocateSpecific(ip, owner); err != nil {
			return r.fail(ctx, &vnic, "IPAllocated", "IPConflict", err.Error())
		}
		st.IPAddress = ip.String()
	default:
		ip, err := alloc.Allocate(owner)
		if err != nil {
			return r.fail(ctx, &vnic, "IPAllocated", "Exhausted", err.Error())
		}
		st.IPAddress = ip.String()
	}
	setCond(&vnic, "IPAllocated", metav1.ConditionTrue, "Allocated", "Allocated "+st.IPAddress)

	mac, err := r.effectiveMAC(ctx, &vnic)
	if err != nil {
		return r.fail(ctx, &vnic, "MACAssigned", "MACConflict", err.Error())
	}
	st.MACAddress = mac
	setCond(&vnic, "MACAssigned", metav1.ConditionTrue, "Assigned", "MAC "+mac)

	var parent vpcv1alpha1.VPC
	mtu := vnic.Spec.MTU
	if err := r.Get(ctx, types.NamespacedName{Name: subnet.Spec.VPCRef, Namespace: vnic.Namespace}, &parent); err == nil && mtu == 0 {
		mtu = parent.Spec.MTU
	}
	ones, _ := ipnet.Mask.Size()
	st.VPCRef = subnet.Spec.VPCRef
	st.Gateway = alloc.Gateway().String()
	st.PrefixLength = int32(ones)
	st.MTU = mtu

	// Bound is owned by the node agent; the controller only demotes it when placement changes.
	if st.BoundNode != "" && vnic.Spec.NodeName != "" && st.BoundNode != vnic.Spec.NodeName {
		st.BoundNode, st.Bridge, st.OVSPort, st.OFPort = "", "", "", 0
	}
	if st.BoundNode == "" {
		st.Phase = vpcv1alpha1.VNicPhaseAllocated
	} else {
		st.Phase = vpcv1alpha1.VNicPhaseBound
	}

	if vnic.Spec.FirewallRef != "" {
		var fw metav1.PartialObjectMetadata
		fw.SetGroupVersionKind(fwGVK)
		if err := r.Get(ctx, types.NamespacedName{Name: vnic.Spec.FirewallRef, Namespace: vnic.Namespace}, &fw); err != nil {
			setCond(&vnic, "FirewallFound", metav1.ConditionFalse, "FirewallNotFound", fmt.Sprintf("Firewall %q not found", vnic.Spec.FirewallRef))
		} else {
			setCond(&vnic, "FirewallFound", metav1.ConditionTrue, "Found", "Firewall found")
		}
	} else {
		meta.RemoveStatusCondition(&vnic.Status.Conditions, "FirewallFound")
	}

	if err := r.Status().Update(ctx, &vnic); err != nil {
		return ctrl.Result{}, err
	}
	log.Info("VNic reconciled", "vnic", vnic.Name, "subnet", subnet.Name, "ip", st.IPAddress, "mac", st.MACAddress)
	return ctrl.Result{}, nil
}

// effectiveMAC returns spec.macAddress or a deterministic locally-assigned MAC that does not clash with other VNics.
func (r *VNicReconciler) effectiveMAC(ctx context.Context, v *vpcv1alpha1.VNic) (string, error) {
	var all vpcv1alpha1.VNicList
	if err := r.List(ctx, &all); err != nil {
		return "", err
	}
	used := map[string]string{}
	for _, o := range all.Items {
		if o.UID == v.UID {
			continue
		}
		if m := o.Status.MACAddress; m != "" {
			used[strings.ToLower(m)] = o.Namespace + "/" + o.Name
		}
	}
	if v.Spec.MACAddress != "" {
		m := strings.ToLower(v.Spec.MACAddress)
		if who, ok := used[m]; ok {
			return "", fmt.Errorf("MAC %s already used by VNic %s", m, who)
		}
		return m, nil
	}
	if v.Status.MACAddress != "" {
		if _, ok := used[strings.ToLower(v.Status.MACAddress)]; !ok {
			return v.Status.MACAddress, nil
		}
	}
	for i := 0; i < 256; i++ {
		h := sha1.Sum([]byte(fmt.Sprintf("%s/%s/%d", v.Namespace, v.Name, i)))
		m := fmt.Sprintf("52:54:00:%02x:%02x:%02x", h[0], h[1], h[2])
		if _, ok := used[m]; !ok {
			return m, nil
		}
	}
	return "", fmt.Errorf("could not generate a unique MAC")
}

func (r *VNicReconciler) reconcileDelete(ctx context.Context, v *vpcv1alpha1.VNic) (ctrl.Result, error) {
	if controllerutil.ContainsFinalizer(v, vnicFinalizer) {
		if v.Status.IPAddress != "" {
			if alloc, err := r.IPAMManager.GetAllocator(v.Spec.SubnetRef); err == nil {
				alloc.Release(net.ParseIP(v.Status.IPAddress))
			}
		}
		controllerutil.RemoveFinalizer(v, vnicFinalizer)
		if err := r.Update(ctx, v); err != nil {
			return ctrl.Result{}, err
		}
		logf.FromContext(ctx).Info("VNic deleted", "vnic", v.Name)
	}
	return ctrl.Result{}, nil
}

func (r *VNicReconciler) pending(ctx context.Context, v *vpcv1alpha1.VNic, ct, reason, msg string) (ctrl.Result, error) {
	setCond(v, ct, metav1.ConditionFalse, reason, msg)
	v.Status.Phase = vpcv1alpha1.VNicPhasePending
	return ctrl.Result{}, r.Status().Update(ctx, v)
}

func (r *VNicReconciler) fail(ctx context.Context, v *vpcv1alpha1.VNic, ct, reason, msg string) (ctrl.Result, error) {
	setCond(v, ct, metav1.ConditionFalse, reason, msg)
	v.Status.Phase = vpcv1alpha1.VNicPhaseFailed
	return ctrl.Result{}, r.Status().Update(ctx, v)
}

func setCond(v *vpcv1alpha1.VNic, t string, s metav1.ConditionStatus, reason, msg string) {
	meta.SetStatusCondition(&v.Status.Conditions, metav1.Condition{Type: t, Status: s, Reason: reason, Message: msg, LastTransitionTime: metav1.Now()})
}

// SetupWithManager sets up the controller with the Manager.
func (r *VNicReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Re-evaluate VNics when their subnet changes (becomes Ready, is recreated, ...).
	subnetToVNics := handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, o client.Object) []reconcile.Request {
		var l vpcv1alpha1.VNicList
		if err := r.List(ctx, &l, client.InNamespace(o.GetNamespace())); err != nil {
			return nil
		}
		var reqs []reconcile.Request
		for _, v := range l.Items {
			if v.Spec.SubnetRef == o.GetName() {
				reqs = append(reqs, reconcile.Request{NamespacedName: types.NamespacedName{Name: v.Name, Namespace: v.Namespace}})
			}
		}
		return reqs
	})
	return ctrl.NewControllerManagedBy(mgr).
		For(&vpcv1alpha1.VNic{}).
		Watches(&vpcv1alpha1.Subnet{}, subnetToVNics).
		Named("vpc-vnic").
		Complete(r)
}
