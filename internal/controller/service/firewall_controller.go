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

package service

import (
	"context"
	"fmt"
	"sort"
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

	servicev1alpha1 "github.com/sdncontroller/sdncontroller/api/service/v1alpha1"
	vpcv1alpha1 "github.com/sdncontroller/sdncontroller/api/vpc/v1alpha1"
	"github.com/sdncontroller/sdncontroller/internal/placement"
)

const firewallFinalizer = "firewall.sdncontroller.io/finalizer"

// FirewallReconciler reconciles a Firewall object.
type FirewallReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=service.sdncontroller.io,resources=firewalls,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=service.sdncontroller.io,resources=firewalls/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=service.sdncontroller.io,resources=firewalls/finalizers,verbs=update
// +kubebuilder:rbac:groups=vpc.sdncontroller.io,resources=vpcs,verbs=get;list;watch

func (r *FirewallReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var fw servicev1alpha1.Firewall
	if err := r.Get(ctx, req.NamespacedName, &fw); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle deletion
	if !fw.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, &fw)
	}

	// Add finalizer
	if !controllerutil.ContainsFinalizer(&fw, firewallFinalizer) {
		controllerutil.AddFinalizer(&fw, firewallFinalizer)
		if err := r.Update(ctx, &fw); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Set phase to Pending if not set
	if fw.Status.Phase == "" {
		fw.Status.Phase = servicev1alpha1.FirewallPhasePending
		if err := r.Status().Update(ctx, &fw); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Validate VPC ref exists
	var vpc vpcv1alpha1.VPC
	if err := r.Get(ctx, types.NamespacedName{Name: fw.Spec.VPCRef, Namespace: fw.Namespace}, &vpc); err != nil {
		r.setCondition(&fw, "VPCReady", metav1.ConditionFalse, "VPCNotFound", fmt.Sprintf("VPC %q not found", fw.Spec.VPCRef))
		fw.Status.Phase = servicev1alpha1.FirewallPhaseFailed
		_ = r.Status().Update(ctx, &fw)
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}
	if vpc.Status.Phase != vpcv1alpha1.VPCPhaseReady {
		r.setCondition(&fw, "VPCReady", metav1.ConditionFalse, "VPCNotReady", "Parent VPC is not ready")
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}
	r.setCondition(&fw, "VPCReady", metav1.ConditionTrue, "Ready", "Parent VPC is ready")

	// Validate rules (priority ordering - check for duplicate priorities)
	if len(fw.Spec.Rules) > 0 {
		priorities := make(map[int32]string)
		for _, rule := range fw.Spec.Rules {
			if existing, ok := priorities[rule.Priority]; ok {
				r.setCondition(&fw, "RulesValid", metav1.ConditionFalse, "DuplicatePriority",
					fmt.Sprintf("Rules %q and %q have the same priority %d", existing, rule.Name, rule.Priority))
				fw.Status.Phase = servicev1alpha1.FirewallPhaseFailed
				_ = r.Status().Update(ctx, &fw)
				return ctrl.Result{}, nil
			}
			priorities[rule.Priority] = rule.Name
		}

		// Verify rules are sorted by priority
		sorted := make([]servicev1alpha1.FirewallRule, len(fw.Spec.Rules))
		copy(sorted, fw.Spec.Rules)
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].Priority < sorted[j].Priority
		})
		r.setCondition(&fw, "RulesValid", metav1.ConditionTrue, "Valid",
			fmt.Sprintf("%d rule(s) validated and ordered by priority", len(fw.Spec.Rules)))
	} else {
		r.setCondition(&fw, "RulesValid", metav1.ConditionTrue, "Valid", "No rules configured, using default action")
	}

	// Placement, then what the node agents actually programmed.
	nodes, err := placement.Nodes(ctx, r.Client, fw.Namespace, fw.Spec.VPCRef, fw.Spec.NodeSelector)
	if err != nil {
		return ctrl.Result{}, err
	}
	fw.Status.Nodes = nodes
	if len(nodes) == 0 {
		r.setCondition(&fw, "Placed", metav1.ConditionFalse, "NoEligibleNodes", "no Ready node matches nodeSelector and VPC attachment")
	} else {
		r.setCondition(&fw, "Placed", metav1.ConditionTrue, "Placed", fmt.Sprintf("scheduled on %v", nodes))
	}
	obs := map[string]placement.Observed{}
	var packets int64
	var warnings []string
	for n, e := range fw.Status.Enforcement {
		obs[n] = placement.Observed{Generation: e.ObservedGeneration, Programmed: e.Programmed, Message: e.Message}
		packets += e.MatchedPackets
		warnings = append(warnings, e.Warnings...)
	}
	fw.Status.MatchedPackets = packets
	fw.Status.RuleCounters = sumRuleCounters(fw.Status.Enforcement, fw.Spec.Rules)
	done, waiting, errs := placement.Evaluate(nodes, fw.Generation, obs)
	switch {
	case len(nodes) == 0:
		fw.Status.Phase, fw.Status.ActiveRules = servicev1alpha1.FirewallPhasePending, 0
		r.setCondition(&fw, "Ready", metav1.ConditionFalse, "NoEligibleNodes", "no node to enforce on")
	case len(errs) > 0:
		fw.Status.Phase, fw.Status.ActiveRules = servicev1alpha1.FirewallPhaseFailed, 0
		r.setCondition(&fw, "Ready", metav1.ConditionFalse, "EnforcementFailed", strings.Join(errs, "; "))
	case len(waiting) > 0:
		fw.Status.Phase, fw.Status.ActiveRules = servicev1alpha1.FirewallPhasePending, 0
		r.setCondition(&fw, "Ready", metav1.ConditionFalse, "WaitingForAgents",
			fmt.Sprintf("enforced on %d/%d nodes, waiting for %v", len(done), len(nodes), waiting))
	default:
		fw.Status.Phase, fw.Status.ActiveRules = servicev1alpha1.FirewallPhaseActive, int32(len(fw.Spec.Rules))
		msg := fmt.Sprintf("enforced in OVS on %d node(s) with %d rule(s), default action: %s", len(done), len(fw.Spec.Rules), fw.Spec.DefaultAction)
		if len(warnings) > 0 {
			msg += " (warnings: " + strings.Join(dedupe(warnings), "; ") + ")"
		}
		r.setCondition(&fw, "Ready", metav1.ConditionTrue, "Enforced", msg)
	}

	if err := r.Status().Update(ctx, &fw); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Firewall reconciled", "fw", fw.Name, "vpc", fw.Spec.VPCRef, "rules", fw.Status.ActiveRules)
	return ctrl.Result{}, nil
}

func (r *FirewallReconciler) reconcileDelete(ctx context.Context, fw *servicev1alpha1.Firewall) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if controllerutil.ContainsFinalizer(fw, firewallFinalizer) {
		controllerutil.RemoveFinalizer(fw, firewallFinalizer)
		if err := r.Update(ctx, fw); err != nil {
			return ctrl.Result{}, err
		}
		log.Info("Firewall deleted", "fw", fw.Name)
	}
	return ctrl.Result{}, nil
}

func (r *FirewallReconciler) setCondition(fw *servicev1alpha1.Firewall, condType string, status metav1.ConditionStatus, reason, message string) {
	meta.SetStatusCondition(&fw.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
	})
}

// SetupWithManager sets up the controller with the Manager.
func (r *FirewallReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&servicev1alpha1.Firewall{}).
		Watches(&vpcv1alpha1.VPC{}, placement.ByVPC(r.Client, func() client.ObjectList { return &servicev1alpha1.FirewallList{} }, func(o client.Object) string { return o.(*servicev1alpha1.Firewall).Spec.VPCRef })).
		Named("service-firewall").
		Complete(r)
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range in {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

// sumRuleCounters adds the per-rule counters of all nodes, in rule priority order.
func sumRuleCounters(enf map[string]servicev1alpha1.EnforcementStatus, rules []servicev1alpha1.FirewallRule) []servicev1alpha1.RuleCounter {
	sum := map[string]servicev1alpha1.RuleCounter{}
	for _, e := range enf {
		for _, rc := range e.Rules {
			s := sum[rc.Name]
			s.Name, s.Action = rc.Name, rc.Action
			s.Packets += rc.Packets
			s.Bytes += rc.Bytes
			sum[rc.Name] = s
		}
	}
	sorted := append([]servicev1alpha1.FirewallRule(nil), rules...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Priority < sorted[j].Priority })
	var out []servicev1alpha1.RuleCounter
	for _, r := range sorted {
		if rc, ok := sum[r.Name]; ok {
			out = append(out, rc)
		}
	}
	return out
}
