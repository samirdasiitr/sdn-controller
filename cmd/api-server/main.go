package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

var (
	agentPort     int
	dynamicClient dynamic.Interface
	listenAddr    string
	staticDir     string
	prometheusURL string
)

var nodesGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "nodes"}

// CRD group/version/resource mappings
var gvrMap = map[string]schema.GroupVersionResource{
	"vpcs":                    {Group: "vpc.sdncontroller.io", Version: "v1alpha1", Resource: "vpcs"},
	"subnets":                 {Group: "vpc.sdncontroller.io", Version: "v1alpha1", Resource: "subnets"},
	"internetgateways":        {Group: "gateway.sdncontroller.io", Version: "v1alpha1", Resource: "internetgateways"},
	"natgateways":             {Group: "gateway.sdncontroller.io", Version: "v1alpha1", Resource: "natgateways"},
	"loadbalancers":           {Group: "service.sdncontroller.io", Version: "v1alpha1", Resource: "loadbalancers"},
	"firewalls":               {Group: "service.sdncontroller.io", Version: "v1alpha1", Resource: "firewalls"},
	"tunnels":                 {Group: "infra.sdncontroller.io", Version: "v1alpha1", Resource: "tunnels"},
	"bonds":                   {Group: "infra.sdncontroller.io", Version: "v1alpha1", Resource: "bonds"},
	"networkinterfaces":       {Group: "infra.sdncontroller.io", Version: "v1alpha1", Resource: "networkinterfaces"},
	"nodenetworkconfigs":      {Group: "infra.sdncontroller.io", Version: "v1alpha1", Resource: "nodenetworkconfigs"},
	"servicechains":           {Group: "nfv.sdncontroller.io", Version: "v1alpha1", Resource: "servicechains"},
	"virtualnetworkfunctions": {Group: "nfv.sdncontroller.io", Version: "v1alpha1", Resource: "virtualnetworkfunctions"},
	"vnics":                   {Group: "vpc.sdncontroller.io", Version: "v1alpha1", Resource: "vnics"},
}

// Kind to resource mapping for create/update
var kindToResource = map[string]string{
	"VPC": "vpcs", "Subnet": "subnets",
	"InternetGateway": "internetgateways", "NATGateway": "natgateways",
	"LoadBalancer": "loadbalancers", "Firewall": "firewalls",
	"Tunnel": "tunnels", "Bond": "bonds",
	"NetworkInterface": "networkinterfaces", "NodeNetworkConfig": "nodenetworkconfigs",
	"ServiceChain": "servicechains", "VirtualNetworkFunction": "virtualnetworkfunctions",
	"VNic": "vnics",
}

func main() {
	flag.StringVar(&listenAddr, "listen", ":3001", "API server listen address")
	flag.StringVar(&staticDir, "static", "", "Path to static frontend files (optional)")
	flag.IntVar(&agentPort, "agent-port", 9101, "Node agent HTTP port (OVS topology/flows proxy)")
	flag.StringVar(&prometheusURL, "prometheus", "http://localhost:9090", "Prometheus base URL for /api/metrics proxy")
	flag.Parse()

	// Build K8s client
	config, err := rest.InClusterConfig()
	if err != nil {
		kubeconfig := os.Getenv("KUBECONFIG")
		if kubeconfig == "" {
			kubeconfig = filepath.Join(homedir.HomeDir(), ".kube", "config")
		}
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			log.Fatalf("Failed to build k8s config: %v", err)
		}
	}

	dynamicClient, err = dynamic.NewForConfig(config)
	if err != nil {
		log.Fatalf("Failed to create dynamic client: %v", err)
	}

	mux := http.NewServeMux()

	// API routes
	mux.HandleFunc("/api/resources", handleListResourceTypes)
	mux.HandleFunc("/api/resources/", handleResource)
	mux.HandleFunc("/api/topology", handleTopology)
	mux.HandleFunc("/api/namespaces", handleNamespaces)
	mux.HandleFunc("/api/nodes", handleNodes)
	mux.HandleFunc("/api/nodes/", handleNodeDetail)
	mux.HandleFunc("/api/tags", handleTags)
	mux.HandleFunc("/api/metrics/query", handleMetricsProxy("query"))
	mux.HandleFunc("/api/metrics/query_range", handleMetricsProxy("query_range"))

	// Serve static frontend
	if staticDir != "" {
		fs := http.FileServer(http.Dir(staticDir))
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			// SPA fallback: serve index.html for non-file routes
			path := r.URL.Path
			if path != "/" && !strings.HasPrefix(path, "/api/") {
				fullPath := filepath.Join(staticDir, path)
				if _, err := os.Stat(fullPath); os.IsNotExist(err) {
					http.ServeFile(w, r, filepath.Join(staticDir, "index.html"))
					return
				}
			}
			fs.ServeHTTP(w, r)
		})
	}

	// CORS middleware
	handler := corsMiddleware(mux)

	log.Printf("SDN API server listening on %s", listenAddr)
	if err := http.ListenAndServe(listenAddr, handler); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func jsonResponse(w http.ResponseWriter, code int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(data)
}

func jsonError(w http.ResponseWriter, code int, msg string) {
	jsonResponse(w, code, map[string]string{"error": msg})
}

// GET /api/resources - list all resource types
func handleListResourceTypes(w http.ResponseWriter, r *http.Request) {
	types := []map[string]string{}
	for name, gvr := range gvrMap {
		types = append(types, map[string]string{
			"name":    name,
			"group":   gvr.Group,
			"version": gvr.Version,
		})
	}
	jsonResponse(w, http.StatusOK, types)
}

// /api/resources/{resource}[/{namespace}[/{name}]]
func handleResource(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/resources/")
	parts := strings.SplitN(path, "/", 3)

	if len(parts) == 0 || parts[0] == "" {
		jsonError(w, 400, "resource type required")
		return
	}

	resource := parts[0]
	gvr, ok := gvrMap[resource]
	if !ok {
		jsonError(w, 404, fmt.Sprintf("unknown resource type: %s", resource))
		return
	}

	namespace := ""
	name := ""
	if len(parts) >= 2 {
		namespace = parts[1]
	}
	if len(parts) >= 3 {
		name = parts[2]
	}

	ctx := context.Background()

	switch r.Method {
	case "GET":
		if name != "" {
			// Get single resource
			obj, err := dynamicClient.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				k8sError(w, err)
				return
			}
			jsonResponse(w, 200, obj.Object)
		} else if namespace != "" {
			// List in namespace
			list, err := dynamicClient.Resource(gvr).Namespace(namespace).List(ctx, metav1.ListOptions{})
			if err != nil {
				k8sError(w, err)
				return
			}
			jsonResponse(w, 200, list.Items)
		} else {
			// List all namespaces
			list, err := dynamicClient.Resource(gvr).Namespace("").List(ctx, metav1.ListOptions{})
			if err != nil {
				k8sError(w, err)
				return
			}
			jsonResponse(w, 200, list.Items)
		}

	case "POST":
		body, err := io.ReadAll(r.Body)
		if err != nil {
			jsonError(w, 400, "failed to read body")
			return
		}
		var obj unstructured.Unstructured
		if err := json.Unmarshal(body, &obj.Object); err != nil {
			jsonError(w, 400, fmt.Sprintf("invalid JSON: %v", err))
			return
		}
		ns := obj.GetNamespace()
		if ns == "" && namespace != "" {
			ns = namespace
			obj.SetNamespace(ns)
		}
		if ns == "" {
			ns = "default"
			obj.SetNamespace(ns)
		}
		created, err := dynamicClient.Resource(gvr).Namespace(ns).Create(ctx, &obj, metav1.CreateOptions{})
		if err != nil {
			k8sError(w, err)
			return
		}
		jsonResponse(w, 201, created.Object)

	case "PUT":
		if name == "" {
			jsonError(w, 400, "resource name required for update")
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			jsonError(w, 400, "failed to read body")
			return
		}
		var obj unstructured.Unstructured
		if err := json.Unmarshal(body, &obj.Object); err != nil {
			jsonError(w, 400, fmt.Sprintf("invalid JSON: %v", err))
			return
		}
		// Get existing to preserve resourceVersion
		existing, err := dynamicClient.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			k8sError(w, err)
			return
		}
		obj.SetResourceVersion(existing.GetResourceVersion())
		obj.SetNamespace(namespace)
		obj.SetName(name)
		updated, err := dynamicClient.Resource(gvr).Namespace(namespace).Update(ctx, &obj, metav1.UpdateOptions{})
		if err != nil {
			k8sError(w, err)
			return
		}
		jsonResponse(w, 200, updated.Object)

	case "DELETE":
		if name == "" {
			jsonError(w, 400, "resource name required for delete")
			return
		}
		err := dynamicClient.Resource(gvr).Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{})
		if err != nil {
			k8sError(w, err)
			return
		}
		jsonResponse(w, 200, map[string]string{"status": "deleted", "name": name})

	default:
		jsonError(w, 405, "method not allowed")
	}
}

// k8sError writes a Kubernetes API error with its proper HTTP status and a friendly message.
func k8sError(w http.ResponseWriter, err error) {
	code := http.StatusInternalServerError
	var status apierrors.APIStatus
	if errors.As(err, &status) {
		if c := int(status.Status().Code); c >= 400 && c < 600 {
			code = c
		}
	}
	jsonError(w, code, err.Error())
}

func refKey(resource, ns, name string) string {
	return fmt.Sprintf("%s/%s/%s", resource, ns, name)
}

// GET /api/topology - returns nodes and edges for the network graph.
// Edges go from the referenced (parent) object to the referencing (child) object.
// Node ids are "<resource>/<namespace>/<name>"; k8s nodes use "nodes//<name>".
func handleTopology(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	nodes := []map[string]interface{}{}
	edges := []map[string]interface{}{}
	ids := map[string]bool{}

	resources := make([]string, 0, len(gvrMap))
	for resource := range gvrMap {
		resources = append(resources, resource)
	}
	sort.Strings(resources)

	type pending struct{ source, target, typ, label string }
	var pend []pending
	addEdge := func(parent, child, typ, label string) {
		pend = append(pend, pending{parent, child, typ, label})
	}

	for _, resource := range resources {
		gvr := gvrMap[resource]
		list, err := dynamicClient.Resource(gvr).Namespace("").List(ctx, metav1.ListOptions{})
		if err != nil {
			continue
		}
		for _, item := range list.Items {
			name := item.GetName()
			ns := item.GetNamespace()
			id := refKey(resource, ns, name)
			ids[id] = true

			spec, _ := item.Object["spec"].(map[string]interface{})
			status, _ := item.Object["status"].(map[string]interface{})

			nodes = append(nodes, map[string]interface{}{
				"id":                id,
				"name":              name,
				"namespace":         ns,
				"kind":              item.GetKind(),
				"resource":          resource,
				"spec":              spec,
				"status":            status,
				"labels":            item.GetLabels(),
				"deletionTimestamp": item.GetDeletionTimestamp(),
			})

			if spec == nil {
				continue
			}
			vpcRef, _ := spec["vpcRef"].(string)
			if vpcRef != "" {
				// Subnets hang off the VPC; other objects hang off their subnet(s) when
				// they have one, otherwise directly off the VPC (resolved below).
				addEdge(refKey("vpcs", ns, vpcRef), id, "belongs_to", "vpc")
			}
			if sr, ok := spec["subnetRef"].(string); ok && sr != "" {
				addEdge(refKey("subnets", ns, sr), id, "attached_to", "subnet")
			}
			if srs, ok := spec["subnetRefs"].([]interface{}); ok {
				for _, x := range srs {
					if sr, ok := x.(string); ok && sr != "" {
						addEdge(refKey("subnets", ns, sr), id, "attached_to", "subnet")
					}
				}
			}
			if chain, ok := spec["chain"].([]interface{}); ok {
				for i, hop := range chain {
					if h, ok := hop.(map[string]interface{}); ok {
						if vnfRef, ok := h["vnfRef"].(string); ok && vnfRef != "" {
							addEdge(id, refKey("virtualnetworkfunctions", ns, vnfRef), "uses_vnf", fmt.Sprintf("hop %d", i+1))
						}
					}
				}
			}
			if fw, ok := spec["firewallRef"].(string); ok && fw != "" && resource == "vnics" {
				addEdge(id, refKey("firewalls", ns, fw), "protected_by", "firewall")
			}
			if resource == "vnics" {
				bn, _, _ := unstructured.NestedString(item.Object, "status", "boundNode")
				if bn == "" {
					bn, _ = spec["nodeName"].(string)
				}
				if bn != "" {
					addEdge(id, refKey("nodes", "", bn), "runs_on", "runs on")
				}
			}
			// Node relationships
			if resource == "tunnels" {
				for _, k := range []string{"localEndpoint", "remoteEndpoint"} {
					if ep, ok := spec[k].(map[string]interface{}); ok {
						if n, ok := ep["nodeName"].(string); ok && n != "" {
							addEdge(id, refKey("nodes", "", n), "tunnel_endpoint", strings.TrimSuffix(k, "Endpoint"))
						}
					}
				}
			}
			if status != nil {
				if an, ok := status["activeNode"].(string); ok && an != "" {
					addEdge(id, refKey("nodes", "", an), "active_on", "active")
				}
				if sn, ok := status["standbyNode"].(string); ok && sn != "" {
					addEdge(id, refKey("nodes", "", sn), "standby_on", "standby")
				}
				if ns2, ok := status["nodes"].([]interface{}); ok {
					for _, x := range ns2 {
						if n, ok := x.(string); ok && n != "" {
							addEdge(id, refKey("nodes", "", n), "runs_on", "runs on")
						}
					}
				}
				if att, ok := status["attachedNodes"].([]interface{}); ok {
					for _, a := range att {
						if m, ok := a.(map[string]interface{}); ok {
							if n, ok := m["nodeName"].(string); ok && n != "" {
								addEdge(id, refKey("nodes", "", n), "attached_node", "attached")
							}
						}
					}
				}
			}
		}
	}

	// Kubernetes nodes (plus synthetic entries for nodes referenced but not in the cluster)
	if list, err := dynamicClient.Resource(nodesGVR).List(ctx, metav1.ListOptions{}); err == nil {
		for _, n := range list.Items {
			info := nodeInfo(n)
			id := refKey("nodes", "", n.GetName())
			ids[id] = true
			nodes = append(nodes, map[string]interface{}{
				"id": id, "name": n.GetName(), "namespace": "", "kind": "Node", "resource": "nodes",
				"spec":   map[string]interface{}{"internalIP": info["internalIP"]},
				"status": map[string]interface{}{"phase": map[bool]string{true: "Ready", false: "NotReady"}[info["ready"] == true]},
			})
		}
	}
	for _, p := range pend {
		if strings.HasPrefix(p.target, "nodes/") && !ids[p.target] {
			ids[p.target] = true
			nodes = append(nodes, map[string]interface{}{
				"id": p.target, "name": strings.TrimPrefix(p.target, "nodes//"), "namespace": "",
				"kind": "Node", "resource": "nodes", "external": true,
				"status": map[string]interface{}{"phase": "Unknown"},
			})
		}
	}

	// Drop dangling edges and de-duplicate. Objects with both a vpcRef and a subnetRef(s)
	// are drawn under the subnet only (the VPC edge is redundant).
	hasSubnetParent := map[string]bool{}
	for _, p := range pend {
		if strings.HasPrefix(p.source, "subnets/") && ids[p.source] && ids[p.target] {
			hasSubnetParent[p.target] = true
		}
	}
	seen := map[string]bool{}
	for _, p := range pend {
		if !ids[p.source] || !ids[p.target] {
			continue
		}
		if p.typ == "belongs_to" && hasSubnetParent[p.target] {
			continue
		}
		key := p.source + "|" + p.target + "|" + p.typ
		if seen[key] {
			continue
		}
		seen[key] = true
		edges = append(edges, map[string]interface{}{
			"id":     key,
			"source": p.source,
			"target": p.target,
			"type":   p.typ,
			"label":  p.label,
		})
	}

	jsonResponse(w, 200, map[string]interface{}{
		"nodes": nodes,
		"edges": edges,
	})
}

func nodeInfo(n unstructured.Unstructured) map[string]interface{} {
	internalIP := ""
	ready := false
	readyReason := ""
	addrs, _, _ := unstructured.NestedSlice(n.Object, "status", "addresses")
	for _, a := range addrs {
		if m, ok := a.(map[string]interface{}); ok && m["type"] == "InternalIP" {
			internalIP, _ = m["address"].(string)
		}
	}
	conds, _, _ := unstructured.NestedSlice(n.Object, "status", "conditions")
	for _, c := range conds {
		if m, ok := c.(map[string]interface{}); ok && m["type"] == "Ready" {
			ready = m["status"] == "True"
			readyReason, _ = m["reason"].(string)
		}
	}
	kubelet, _, _ := unstructured.NestedString(n.Object, "status", "nodeInfo", "kubeletVersion")
	labels := n.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	return map[string]interface{}{
		"name":              n.GetName(),
		"internalIP":        internalIP,
		"labels":            labels,
		"ready":             ready,
		"readyReason":       readyReason,
		"kubeletVersion":    kubelet,
		"creationTimestamp": n.GetCreationTimestamp(),
	}
}

// GET /api/nodes - list core v1 nodes
func handleNodes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, 405, "method not allowed")
		return
	}
	list, err := dynamicClient.Resource(nodesGVR).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		k8sError(w, err)
		return
	}
	out := []map[string]interface{}{}
	for _, n := range list.Items {
		out = append(out, nodeInfo(n))
	}
	jsonResponse(w, 200, out)
}

// GET /api/nodes/{name} - node info plus every SDN resource placed on it.
func handleNodeDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, 405, "method not allowed")
		return
	}
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/nodes/"), "/")
	name, sub, _ := strings.Cut(rest, "/")
	if name == "" {
		handleNodes(w, r)
		return
	}
	ctx := context.Background()
	if sub == "ovs" || sub == "ovs/topology" || sub == "ovs/flows" {
		proxyAgent(w, r, name, sub)
		return
	}
	n, err := dynamicClient.Resource(nodesGVR).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		k8sError(w, err)
		return
	}
	out := nodeInfo(*n)
	hosted := hostedOn(ctx, name)
	out["hosted"] = hosted
	jsonResponse(w, 200, out)
}

// proxyAgent forwards read-only OVS queries to the node agent on the node's internal IP.
//
//	GET /api/nodes/{name}/ovs            -> agent /ovs/topology (bridges, ports, uplinks, NAT)
//	GET /api/nodes/{name}/ovs/flows?bridge=br -> agent /ovs/flows (OpenFlow rules)
func proxyAgent(w http.ResponseWriter, r *http.Request, node, sub string) {
	n, err := dynamicClient.Resource(nodesGVR).Get(r.Context(), node, metav1.GetOptions{})
	if err != nil {
		k8sError(w, err)
		return
	}
	ip, _ := nodeInfo(*n)["internalIP"].(string)
	if ip == "" {
		jsonError(w, 502, "node has no internal IP")
		return
	}
	path := "/ovs/topology"
	if sub == "ovs/flows" {
		path = "/ovs/flows?bridge=" + url.QueryEscape(r.URL.Query().Get("bridge"))
	}
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://%s:%d%s", ip, agentPort, path))
	if err != nil {
		jsonError(w, 502, "node agent unreachable: "+err.Error())
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, io.LimitReader(resp.Body, 32<<20))
}

func hostedOn(ctx context.Context, node string) []map[string]interface{} {
	hosted := []map[string]interface{}{}
	resources := make([]string, 0, len(gvrMap))
	for resource := range gvrMap {
		resources = append(resources, resource)
	}
	sort.Strings(resources)
	for _, resource := range resources {
		list, err := dynamicClient.Resource(gvrMap[resource]).Namespace("").List(ctx, metav1.ListOptions{})
		if err != nil {
			continue
		}
		for _, item := range list.Items {
			spec, _ := item.Object["spec"].(map[string]interface{})
			status, _ := item.Object["status"].(map[string]interface{})
			roles := []string{}
			if v, _ := spec["nodeName"].(string); v == node {
				roles = append(roles, "assigned")
			}
			if v, _ := status["activeNode"].(string); v == node {
				roles = append(roles, "active")
			}
			if v, _ := status["standbyNode"].(string); v == node {
				roles = append(roles, "standby")
			}
			if v, _ := status["boundNode"].(string); v == node {
				roles = append(roles, "bound")
			}
			if l, ok := status["nodes"].([]interface{}); ok {
				for _, x := range l {
					if s, _ := x.(string); s == node {
						roles = append(roles, "placed")
						break
					}
				}
			}
			if l, ok := status["attachedNodes"].([]interface{}); ok {
				for _, x := range l {
					if m, ok := x.(map[string]interface{}); ok {
						if s, _ := m["nodeName"].(string); s == node {
							roles = append(roles, "attached")
							break
						}
					}
				}
			}
			if len(roles) == 0 {
				continue
			}
			if len(roles) > 1 && roles[0] == "assigned" {
				roles = roles[1:]
			}
			hosted = append(hosted, map[string]interface{}{
				"kind": item.GetKind(), "name": item.GetName(), "namespace": item.GetNamespace(),
				"resource": resource, "role": strings.Join(roles, ","),
			})
		}
	}
	return hosted
}

// handleMetricsProxy is a thin GET-only proxy to Prometheus' /api/v1/<endpoint>.
func handleMetricsProxy(endpoint string) http.HandlerFunc {
	client := &http.Client{Timeout: 30 * time.Second}
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			jsonError(w, 405, "only GET is allowed")
			return
		}
		if r.URL.Query().Get("query") == "" {
			jsonError(w, 400, "query parameter required")
			return
		}
		target := strings.TrimRight(prometheusURL, "/") + "/api/v1/" + endpoint + "?" + url.Values(r.URL.Query()).Encode()
		req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, target, nil)
		if err != nil {
			jsonError(w, 500, err.Error())
			return
		}
		resp, err := client.Do(req)
		if err != nil {
			jsonError(w, http.StatusBadGateway, fmt.Sprintf("prometheus unreachable: %v", err))
			return
		}
		defer resp.Body.Close()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
	}
}

// tagLabelPrefix is the label prefix used to store UI "tags" on resources.
const tagLabelPrefix = "tags.sdncontroller.io/"

// GET /api/tags - aggregate tags (labels under tags.sdncontroller.io/) across all resource types.
// Returns [{key,value,count,resources:[{resource,kind,namespace,name}]}]
func handleTags(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, 405, "method not allowed")
		return
	}
	ctx := context.Background()
	type tagRef struct {
		Resource  string `json:"resource"`
		Kind      string `json:"kind"`
		Namespace string `json:"namespace"`
		Name      string `json:"name"`
	}
	type tagEntry struct {
		Key       string   `json:"key"`
		Value     string   `json:"value"`
		Count     int      `json:"count"`
		Resources []tagRef `json:"resources"`
	}
	agg := map[string]*tagEntry{}
	resources := make([]string, 0, len(gvrMap))
	for resource := range gvrMap {
		resources = append(resources, resource)
	}
	sort.Strings(resources)
	for _, resource := range resources {
		list, err := dynamicClient.Resource(gvrMap[resource]).Namespace("").List(ctx, metav1.ListOptions{})
		if err != nil {
			continue
		}
		for _, item := range list.Items {
			for lk, lv := range item.GetLabels() {
				if !strings.HasPrefix(lk, tagLabelPrefix) {
					continue
				}
				key := strings.TrimPrefix(lk, tagLabelPrefix)
				id := key + "\x00" + lv
				e, ok := agg[id]
				if !ok {
					e = &tagEntry{Key: key, Value: lv}
					agg[id] = e
				}
				e.Count++
				e.Resources = append(e.Resources, tagRef{
					Resource:  resource,
					Kind:      item.GetKind(),
					Namespace: item.GetNamespace(),
					Name:      item.GetName(),
				})
			}
		}
	}
	out := make([]*tagEntry, 0, len(agg))
	for _, e := range agg {
		sort.Slice(e.Resources, func(i, j int) bool {
			a, b := e.Resources[i], e.Resources[j]
			if a.Resource != b.Resource {
				return a.Resource < b.Resource
			}
			if a.Namespace != b.Namespace {
				return a.Namespace < b.Namespace
			}
			return a.Name < b.Name
		})
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Key != out[j].Key {
			return out[i].Key < out[j].Key
		}
		return out[i].Value < out[j].Value
	})
	jsonResponse(w, 200, out)
}

// GET /api/namespaces - list namespaces
func handleNamespaces(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	nsGVR := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}
	list, err := dynamicClient.Resource(nsGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		jsonError(w, 500, err.Error())
		return
	}
	names := []string{}
	for _, ns := range list.Items {
		names = append(names, ns.GetName())
	}
	jsonResponse(w, 200, names)
}
