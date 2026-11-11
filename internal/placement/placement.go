// Package placement resolves which nodes a distributed network service runs on.
package placement

import (
	"context"
	"sort"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vpcv1alpha1 "github.com/sdncontroller/sdncontroller/api/vpc/v1alpha1"
)

// Nodes returns the Ready nodes matching sel (all nodes when nil). When vpcRef
// names a VPC with attached nodes, the result is limited to those nodes, since
// the dataplane for the VPC only exists there.
func Nodes(ctx context.Context, c client.Client, ns, vpcRef string, sel *metav1.LabelSelector) ([]string, error) {
	selector := labels.Everything()
	if sel != nil {
		s, err := metav1.LabelSelectorAsSelector(sel)
		if err != nil {
			return nil, err
		}
		selector = s
	}
	var nl corev1.NodeList
	if err := c.List(ctx, &nl, client.MatchingLabelsSelector{Selector: selector}); err != nil {
		return nil, err
	}
	attached := map[string]bool{}
	if vpcRef != "" {
		var vpc vpcv1alpha1.VPC
		if err := c.Get(ctx, types.NamespacedName{Name: vpcRef, Namespace: ns}, &vpc); err == nil {
			for _, a := range vpc.Status.AttachedNodes {
				attached[a.NodeName] = true
			}
		}
	}
	out := []string{}
	for _, n := range nl.Items {
		if !ready(&n) || (len(attached) > 0 && !attached[n.Name]) {
			continue
		}
		out = append(out, n.Name)
	}
	sort.Strings(out)
	return out, nil
}

func ready(n *corev1.Node) bool {
	for _, c := range n.Status.Conditions {
		if c.Type == corev1.NodeReady {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}

func requests(ctx context.Context, c client.Client, l client.ObjectList, ns string, match func(client.Object) bool) []reconcile.Request {
	opts := []client.ListOption{}
	if ns != "" {
		opts = append(opts, client.InNamespace(ns))
	}
	if err := c.List(ctx, l, opts...); err != nil {
		return nil
	}
	objs, _ := meta.ExtractList(l)
	var reqs []reconcile.Request
	for _, o := range objs {
		if co, ok := o.(client.Object); ok && match(co) {
			reqs = append(reqs, reconcile.Request{NamespacedName: types.NamespacedName{Name: co.GetName(), Namespace: co.GetNamespace()}})
		}
	}
	return reqs
}

// ByVPC re-enqueues every object of the list type whose VPC (per vpcOf) changed.
func ByVPC(c client.Client, newList func() client.ObjectList, vpcOf func(client.Object) string) handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, o client.Object) []reconcile.Request {
		return requests(ctx, c, newList(), o.GetNamespace(), func(x client.Object) bool { return vpcOf(x) == o.GetName() })
	})
}

// All re-enqueues every object of the list type (used for Node events).
func All(c client.Client, newList func() client.ObjectList) handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, o client.Object) []reconcile.Request {
		return requests(ctx, c, newList(), "", func(client.Object) bool { return true })
	})
}

// Observed is what a node agent reported for one object.
type Observed struct {
	Generation int64
	Programmed bool
	Message    string
}

// Evaluate compares the placed nodes with the agents' reports for the current generation.
// It returns the nodes that are programmed, the nodes still waiting for their agent,
// and error messages from agents that failed to program this generation.
func Evaluate(nodes []string, gen int64, obs map[string]Observed) (programmed, waiting, errs []string) {
	for _, n := range nodes {
		o, ok := obs[n]
		switch {
		case !ok || o.Generation != gen:
			waiting = append(waiting, n)
		case o.Programmed:
			programmed = append(programmed, n)
		default:
			errs = append(errs, n+": "+o.Message)
		}
	}
	return
}
