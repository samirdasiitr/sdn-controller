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

package gateway

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	gatewayv1alpha1 "github.com/sdncontroller/sdncontroller/api/gateway/v1alpha1"
	vpcv1alpha1 "github.com/sdncontroller/sdncontroller/api/vpc/v1alpha1"
	"github.com/sdncontroller/sdncontroller/internal/placement"
)

const natGatewayFinalizer = "natgateway.sdncontroller.io/finalizer"

// NATGatewayReconciler reconciles a NATGateway object.
type NATGatewayReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=gateway.sdncontroller.io,resources=natgateways,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.sdncontroller.io,resources=natgateways/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=gateway.sdncontroller.io,resources=natgateways/finalizers,verbs=update
// +kubebuilder:rbac:groups=vpc.sdncontroller.io,resources=vpcs,verbs=get;list;watch
// +kubebuilder:rbac:groups=vpc.sdncontroller.io,resources=subnets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch

func (r *NATGatewayReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var natgw gatewayv1alpha1.NATGateway
	if err := r.Get(ctx, req.NamespacedName, &natgw); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle deletion
	if !natgw.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, &natgw)
	}

	// Add finalizer
	if !controllerutil.ContainsFinalizer(&natgw, natGatewayFinalizer) {
		controllerutil.AddFinalizer(&natgw, natGatewayFinalizer)
		if err := r.Update(ctx, &natgw); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Set phase to Provisioning
	if natgw.Status.Phase == "" || natgw.Status.Phase == gatewayv1alpha1.GatewayPhasePending {
		natgw.Status.Phase = gatewayv1alpha1.GatewayPhaseProvisioning
		if err := r.Status().Update(ctx, &natgw); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Validate VPC ref exists
	var vpc vpcv1alpha1.VPC
	if err := r.Get(ctx, types.NamespacedName{Name: natgw.Spec.VPCRef, Namespace: natgw.Namespace}, &vpc); err != nil {
		r.setCondition(&natgw, "VPCReady", metav1.ConditionFalse, "VPCNotFound", fmt.Sprintf("VPC %q not found", natgw.Spec.VPCRef))
		natgw.Status.Phase = gatewayv1alpha1.GatewayPhaseFailed
		_ = r.Status().Update(ctx, &natgw)
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}
	if vpc.Status.Phase != vpcv1alpha1.VPCPhaseReady {
		r.setCondition(&natgw, "VPCReady", metav1.ConditionFalse, "VPCNotReady", "Parent VPC is not ready")
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}
	r.setCondition(&natgw, "VPCReady", metav1.ConditionTrue, "Ready", "Parent VPC is ready")

	// Validate subnet refs exist
	for _, subnetRef := range natgw.Spec.SubnetRefs {
		var subnet vpcv1alpha1.Subnet
		if err := r.Get(ctx, types.NamespacedName{Name: subnetRef, Namespace: natgw.Namespace}, &subnet); err != nil {
			r.setCondition(&natgw, "SubnetsValid", metav1.ConditionFalse, "SubnetNotFound",
				fmt.Sprintf("Subnet %q not found", subnetRef))
			natgw.Status.Phase = gatewayv1alpha1.GatewayPhaseFailed
			_ = r.Status().Update(ctx, &natgw)
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}
	}
	r.setCondition(&natgw, "SubnetsValid", metav1.ConditionTrue, "Valid",
		fmt.Sprintf("%d subnet(s) validated", len(natgw.Spec.SubnetRefs)))

	// Configure NAT type
	natType := natgw.Spec.Type
	if natType == "" {
		natType = gatewayv1alpha1.NATTypeSNAT
	}
	if natType == gatewayv1alpha1.NATTypeDNAT || natType == gatewayv1alpha1.NATTypeFullNAT {
		if len(natgw.Spec.PortForwards) == 0 {
			r.setCondition(&natgw, "NATConfigured", metav1.ConditionFalse, "NoPortForwards",
				fmt.Sprintf("type %s needs spec.portForwards", natType))
			natgw.Status.Phase = gatewayv1alpha1.GatewayPhaseFailed
			_ = r.Status().Update(ctx, &natgw)
			return ctrl.Result{}, nil
		}
		seen := map[string]bool{}
		for _, pf := range natgw.Spec.PortForwards {
			k := fmt.Sprintf("%s/%d", pf.Protocol, pf.ExternalPort)
			if net.ParseIP(pf.InternalIP) == nil || seen[k] {
				msg := fmt.Sprintf("port forward %q: invalid internalIP %q", pf.Name, pf.InternalIP)
				if seen[k] {
					msg = fmt.Sprintf("port forward %q: %s is forwarded twice", pf.Name, k)
				}
				r.setCondition(&natgw, "NATConfigured", metav1.ConditionFalse, "InvalidPortForward", msg)
				natgw.Status.Phase = gatewayv1alpha1.GatewayPhaseFailed
				_ = r.Status().Update(ctx, &natgw)
				return ctrl.Result{}, nil
			}
			seen[k] = true
		}
	}
	r.setCondition(&natgw, "NATConfigured", metav1.ConditionTrue, "Configured",
		fmt.Sprintf("NAT type %s configured with external IP %s", natType, natgw.Spec.ExternalIP))

	// Select the active node among Ready nodes that match the selector and are attached to the VPC.
	nodes, err := placement.Nodes(ctx, r.Client, natgw.Namespace, natgw.Spec.VPCRef, natgw.Spec.NodeSelector)
	if err != nil {
		r.setCondition(&natgw, "NodeSelected", metav1.ConditionFalse, "InvalidSelector", err.Error())
		natgw.Status.Phase = gatewayv1alpha1.GatewayPhaseFailed
		_ = r.Status().Update(ctx, &natgw)
		return ctrl.Result{}, nil
	}
	if len(nodes) == 0 {
		r.setCondition(&natgw, "NodeSelected", metav1.ConditionFalse, "NoReadyNodes", "No Ready node matches the selector and is attached to the VPC")
		natgw.Status.Phase = gatewayv1alpha1.GatewayPhaseFailed
		natgw.Status.ActiveNode = ""
		return ctrl.Result{}, r.Status().Update(ctx, &natgw)
	}
	// Keep the current active node while it is still eligible (avoid flapping).
	active := nodes[0]
	for _, n := range nodes {
		if n == natgw.Status.ActiveNode {
			active = n
		}
	}
	natgw.Status.ActiveNode = active
	r.setCondition(&natgw, "NodeSelected", metav1.ConditionTrue, "NodeAssigned", fmt.Sprintf("Active: %s", active))

	// Ready only when the active node's agent has programmed this generation.
	e, ok := natgw.Status.Enforcement[active]
	switch {
	case !ok || e.ObservedGeneration != natgw.Generation:
		natgw.Status.Phase = gatewayv1alpha1.GatewayPhaseProvisioning
		r.setCondition(&natgw, "Ready", metav1.ConditionFalse, "WaitingForAgent", fmt.Sprintf("waiting for the agent on %s to program SNAT", active))
	case !e.Programmed:
		natgw.Status.Phase = gatewayv1alpha1.GatewayPhaseFailed
		r.setCondition(&natgw, "Ready", metav1.ConditionFalse, "ProgrammingFailed", e.Message)
	default:
		natgw.Status.Phase = gatewayv1alpha1.GatewayPhaseReady
		msg := fmt.Sprintf("%s %v via %s programmed in OVS on %s", natType, natgw.Spec.SubnetRefs, natgw.Spec.ExternalIP, active)
		if len(natgw.Spec.PortForwards) > 0 {
			msg += fmt.Sprintf(" (%d port forward(s))", len(natgw.Spec.PortForwards))
		}
		if len(e.Warnings) > 0 {
			msg += " (warnings: " + strings.Join(e.Warnings, "; ") + ")"
		}
		r.setCondition(&natgw, "Ready", metav1.ConditionTrue, "Programmed", msg)
	}
	natgw.Status.TranslationCount = e.Translations
	var totalTr int64
	for _, en := range natgw.Status.Enforcement {
		totalTr += en.NewTranslations
	}
	natgw.Status.TotalTranslations = totalTr

	if err := r.Status().Update(ctx, &natgw); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("NATGateway reconciled", "natgw", natgw.Name, "vpc", natgw.Spec.VPCRef, "type", natType)
	return ctrl.Result{}, nil
}

func (r *NATGatewayReconciler) reconcileDelete(ctx context.Context, natgw *gatewayv1alpha1.NATGateway) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if controllerutil.ContainsFinalizer(natgw, natGatewayFinalizer) {
		controllerutil.RemoveFinalizer(natgw, natGatewayFinalizer)
		if err := r.Update(ctx, natgw); err != nil {
			return ctrl.Result{}, err
		}
		log.Info("NATGateway deleted", "natgw", natgw.Name)
	}
	return ctrl.Result{}, nil
}

func (r *NATGatewayReconciler) setCondition(natgw *gatewayv1alpha1.NATGateway, condType string, status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(&natgw.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
	})
}

// SetupWithManager sets up the controller with the Manager.
func (r *NATGatewayReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&gatewayv1alpha1.NATGateway{}).
		Watches(&vpcv1alpha1.VPC{}, placement.ByVPC(r.Client, func() client.ObjectList { return &gatewayv1alpha1.NATGatewayList{} },
			func(o client.Object) string { return o.(*gatewayv1alpha1.NATGateway).Spec.VPCRef })).
		Named("gateway-natgateway").
		Complete(r)
}
