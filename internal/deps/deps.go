// Package deps finds resources that reference other SDN resources, so
// finalizers can block deletion of entities that are still in use.
package deps

import (
	"context"
	"fmt"
	"sort"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type kindInfo struct {
	group, kind string
}

var referrers = []kindInfo{
	{"vpc.sdncontroller.io", "Subnet"},
	{"gateway.sdncontroller.io", "InternetGateway"},
	{"gateway.sdncontroller.io", "NATGateway"},
	{"service.sdncontroller.io", "LoadBalancer"},
	{"service.sdncontroller.io", "Firewall"},
	{"nfv.sdncontroller.io", "ServiceChain"},
	{"infra.sdncontroller.io", "NetworkInterface"},
	{"infra.sdncontroller.io", "Bond"},
}

func list(ctx context.Context, c client.Client, ki kindInfo, ns string) ([]unstructured.Unstructured, error) {
	ul := &unstructured.UnstructuredList{}
	ul.SetGroupVersionKind(schema.GroupVersionKind{Group: ki.group, Version: "v1alpha1", Kind: ki.kind + "List"})
	if err := c.List(ctx, ul, client.InNamespace(ns)); err != nil {
		return nil, err
	}
	return ul.Items, nil
}

func refs(obj map[string]interface{}, singular, plural string) []string {
	spec, _ := obj["spec"].(map[string]interface{})
	var out []string
	if s, ok := spec[singular].(string); ok && s != "" {
		out = append(out, s)
	}
	if arr, ok := spec[plural].([]interface{}); ok {
		for _, v := range arr {
			if s, ok := v.(string); ok {
				out = append(out, s)
			}
		}
	}
	return out
}

func has(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

// VPCDependents returns "Kind/name" for every resource that references the VPC.
func VPCDependents(ctx context.Context, c client.Client, ns, vpc string) ([]string, error) {
	var out []string
	for _, ki := range referrers {
		items, err := list(ctx, c, ki, ns)
		if err != nil {
			return nil, err
		}
		for _, it := range items {
			if !it.GetDeletionTimestamp().IsZero() {
				continue
			}
			if has(refs(it.Object, "vpcRef", "vpcRefs"), vpc) {
				out = append(out, fmt.Sprintf("%s/%s", ki.kind, it.GetName()))
			}
		}
	}
	sort.Strings(out)
	return out, nil
}

// SubnetDependents returns "Kind/name" for every resource that references the subnet.
func SubnetDependents(ctx context.Context, c client.Client, ns, subnet string) ([]string, error) {
	var out []string
	for _, ki := range referrers {
		if ki.kind == "Subnet" {
			continue
		}
		items, err := list(ctx, c, ki, ns)
		if err != nil {
			return nil, err
		}
		for _, it := range items {
			if !it.GetDeletionTimestamp().IsZero() {
				continue
			}
			if has(refs(it.Object, "subnetRef", "subnetRefs"), subnet) {
				out = append(out, fmt.Sprintf("%s/%s", ki.kind, it.GetName()))
			}
		}
	}
	sort.Strings(out)
	return out, nil
}

// VNFDependents returns service chains whose hops reference the VNF.
func VNFDependents(ctx context.Context, c client.Client, ns, vnf string) ([]string, error) {
	items, err := list(ctx, c, kindInfo{"nfv.sdncontroller.io", "ServiceChain"}, ns)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, it := range items {
		if !it.GetDeletionTimestamp().IsZero() {
			continue
		}
		spec, _ := it.Object["spec"].(map[string]interface{})
		for _, key := range []string{"chain", "hops", "vnfs"} {
			arr, _ := spec[key].([]interface{})
			for _, h := range arr {
				if m, ok := h.(map[string]interface{}); ok && m["vnfRef"] == vnf {
					out = append(out, "ServiceChain/"+it.GetName())
				}
			}
		}
	}
	return out, nil
}
