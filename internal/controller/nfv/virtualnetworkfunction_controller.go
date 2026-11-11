/*
Copyright (c) 2026 Samir Das. All rights reserved.

PROPRIETARY AND CONFIDENTIAL. No license is granted. Any use of this software, and in
particular production or commercial use, requires the prior explicit written permission of
Samir Das <samiruor@gmail.com>. See the LICENSE file.
*/

package nfv

import (
	"context"
	"fmt"
	corev1 "k8s.io/api/core/v1"
	"time"

	"github.com/sdncontroller/sdncontroller/internal/deps"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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

const virtualNetworkFunctionFinalizer = "virtualnetworkfunction.sdncontroller.io/finalizer"

// VirtualNetworkFunctionReconciler reconciles a VirtualNetworkFunction object.
type VirtualNetworkFunctionReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=nfv.sdncontroller.io,resources=virtualnetworkfunctions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=nfv.sdncontroller.io,resources=virtualnetworkfunctions/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=nfv.sdncontroller.io,resources=virtualnetworkfunctions/finalizers,verbs=update

// +kubebuilder:rbac:groups=vpc.sdncontroller.io,resources=vnics,verbs=get;list;watch;create;update;patch;delete

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
	if vnf.Spec.Type == nfvv1alpha1.VNFTypeVM {
		hasSubnet := false
		for _, itf := range vnf.Spec.Interfaces {
			hasSubnet = hasSubnet || itf.SubnetRef != ""
		}
		if !hasSubnet {
			r.setCondition(&vnf, "InterfacesValid", metav1.ConditionFalse, "NoSubnet",
				"a VM VNF needs at least one interface with subnetRef so the guest has a network")
			vnf.Status.Phase = nfvv1alpha1.VNFPhaseFailed
			_ = r.Status().Update(ctx, &vnf)
			return ctrl.Result{}, nil
		}
	}
	r.setCondition(&vnf, "InterfacesValid", metav1.ConditionTrue, "Valid",
		fmt.Sprintf("%d interface(s) configured", len(vnf.Spec.Interfaces)))

	nodes, err := placement.Nodes(ctx, r.Client, vnf.Namespace, "", vnf.Spec.NodeSelector)
	if err != nil {
		return ctrl.Result{}, err
	}
	vnf.Status.Nodes = nodes
	if len(nodes) == 0 {
		r.setCondition(&vnf, "Placed", metav1.ConditionFalse, "NoEligibleNodes", "no Ready node matches nodeSelector")
		r.setCondition(&vnf, "Ready", metav1.ConditionFalse, "NoEligibleNodes", "no instances can be scheduled")
		vnf.Status.Phase, vnf.Status.ReadyReplicas, vnf.Status.Instances = nfvv1alpha1.VNFPhasePending, 0, nil
		return ctrl.Result{}, r.Status().Update(ctx, &vnf)
	}
	r.setCondition(&vnf, "Placed", metav1.ConditionTrue, "Placed", fmt.Sprintf("scheduled on %v", nodes))

	if vnf.Spec.Type == nfvv1alpha1.VNFTypeContainer {
		r.setCondition(&vnf, "Ready", metav1.ConditionFalse, "ContainerVNFUnsupported",
			"Container VNFs are not implemented yet; use type VM")
		vnf.Status.Phase, vnf.Status.ReadyReplicas, vnf.Status.Instances = nfvv1alpha1.VNFPhaseFailed, 0, nil
		return ctrl.Result{}, r.Status().Update(ctx, &vnf)
	}

	// VM VNF: one guest per replica, spread round-robin over the placed nodes. Each
	// interface with a subnetRef gets a VNic (IPAM address + MAC); the node agent boots
	// the guest with those identities and binds the port, which makes the VNic Bound.
	wantVNics := map[string]bool{}
	var instances []nfvv1alpha1.VNFInstanceStatus
	var ready int32
	for i := int32(0); i < vnf.Spec.Replicas; i++ {
		inst := nfvv1alpha1.VNFInstanceStatus{Name: fmt.Sprintf("%s-%d", vnf.Name, i), Node: nodes[int(i)%len(nodes)], Ready: true}
		for _, itf := range vnf.Spec.Interfaces {
			if itf.SubnetRef == "" {
				continue
			}
			vn, err := r.ensureVNic(ctx, &vnf, inst, itf)
			if err != nil {
				return ctrl.Result{}, err
			}
			wantVNics[vn.Name] = true
			if vn.Status.Phase != vpcv1alpha1.VNicPhaseBound {
				inst.Ready = false
				if inst.Message == "" {
					inst.Message = fmt.Sprintf("VNic %s is %s", vn.Name, phaseOr(string(vn.Status.Phase), "Pending"))
				}
			}
		}
		if inst.Ready {
			ready++
		}
		instances = append(instances, inst)
	}
	if err := r.pruneVNics(ctx, &vnf, wantVNics); err != nil {
		return ctrl.Result{}, err
	}
	vnf.Status.Instances = instances
	vnf.Status.ReadyReplicas = ready
	if ready == vnf.Spec.Replicas {
		vnf.Status.Phase = nfvv1alpha1.VNFPhaseRunning
		r.setCondition(&vnf, "Ready", metav1.ConditionTrue, "Running",
			fmt.Sprintf("VNF running: %d/%d instances up", ready, vnf.Spec.Replicas))
	} else {
		vnf.Status.Phase = nfvv1alpha1.VNFPhaseCreating
		r.setCondition(&vnf, "Ready", metav1.ConditionFalse, "Provisioning",
			fmt.Sprintf("%d/%d instances up", ready, vnf.Spec.Replicas))
	}
	if err := r.Status().Update(ctx, &vnf); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("VirtualNetworkFunction reconciled", "vnf", vnf.Name, "type", vnf.Spec.Type,
		"replicas", vnf.Spec.Replicas, "ready", vnf.Status.ReadyReplicas)
	return ctrl.Result{}, nil
}

func phaseOr(p, d string) string {
	if p == "" {
		return d
	}
	return p
}

// ensureVNic creates (or re-pins) the VNic for one instance interface.
func (r *VirtualNetworkFunctionReconciler) ensureVNic(ctx context.Context, vnf *nfvv1alpha1.VirtualNetworkFunction,
	inst nfvv1alpha1.VNFInstanceStatus, itf nfvv1alpha1.VNFInterface) (*vpcv1alpha1.VNic, error) {
	name := inst.Name + "-" + itf.Name
	var vn vpcv1alpha1.VNic
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: vnf.Namespace}, &vn)
	if apierrors.IsNotFound(err) {
		vn = vpcv1alpha1.VNic{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: vnf.Namespace},
			Spec: vpcv1alpha1.VNicSpec{
				SubnetRef:  itf.SubnetRef,
				Attachment: &vpcv1alpha1.VNicAttachment{Kind: vpcv1alpha1.VNicAttachVNF, Name: inst.Name},
				NodeName:   inst.Node,
			},
		}
		if err := controllerutil.SetControllerReference(vnf, &vn, r.Scheme); err != nil {
			return nil, err
		}
		return &vn, r.Create(ctx, &vn)
	}
	if err != nil {
		return nil, err
	}
	if vn.Spec.NodeName != inst.Node || vn.Spec.SubnetRef != itf.SubnetRef {
		vn.Spec.NodeName, vn.Spec.SubnetRef = inst.Node, itf.SubnetRef
		if err := r.Update(ctx, &vn); err != nil {
			return nil, err
		}
	}
	return &vn, nil
}

// pruneVNics deletes VNics owned by this VNF that are no longer wanted (scale down, interface removed).
func (r *VirtualNetworkFunctionReconciler) pruneVNics(ctx context.Context, vnf *nfvv1alpha1.VirtualNetworkFunction, want map[string]bool) error {
	var l vpcv1alpha1.VNicList
	if err := r.List(ctx, &l, client.InNamespace(vnf.Namespace)); err != nil {
		return err
	}
	for i := range l.Items {
		vn := &l.Items[i]
		if metav1.IsControlledBy(vn, vnf) && !want[vn.Name] && vn.DeletionTimestamp.IsZero() {
			if err := r.Delete(ctx, vn); client.IgnoreNotFound(err) != nil {
				return err
			}
		}
	}
	return nil
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
		Owns(&vpcv1alpha1.VNic{}).
		Watches(&corev1.Node{}, placement.All(r.Client, func() client.ObjectList { return &nfvv1alpha1.VirtualNetworkFunctionList{} })).
		Named("nfv-virtualnetworkfunction").
		Complete(r)
}
