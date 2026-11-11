package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

var (
	dynamicClient dynamic.Interface
	listenAddr    string
	staticDir     string
)

// CRD group/version/resource mappings
var gvrMap = map[string]schema.GroupVersionResource{
	"vpcs":                {Group: "vpc.sdncontroller.io", Version: "v1alpha1", Resource: "vpcs"},
	"subnets":             {Group: "vpc.sdncontroller.io", Version: "v1alpha1", Resource: "subnets"},
	"internetgateways":    {Group: "gateway.sdncontroller.io", Version: "v1alpha1", Resource: "internetgateways"},
	"natgateways":         {Group: "gateway.sdncontroller.io", Version: "v1alpha1", Resource: "natgateways"},
	"loadbalancers":       {Group: "service.sdncontroller.io", Version: "v1alpha1", Resource: "loadbalancers"},
	"firewalls":           {Group: "service.sdncontroller.io", Version: "v1alpha1", Resource: "firewalls"},
	"tunnels":             {Group: "infra.sdncontroller.io", Version: "v1alpha1", Resource: "tunnels"},
	"bonds":               {Group: "infra.sdncontroller.io", Version: "v1alpha1", Resource: "bonds"},
	"networkinterfaces":   {Group: "infra.sdncontroller.io", Version: "v1alpha1", Resource: "networkinterfaces"},
	"nodenetworkconfigs":  {Group: "infra.sdncontroller.io", Version: "v1alpha1", Resource: "nodenetworkconfigs"},
	"servicechains":       {Group: "nfv.sdncontroller.io", Version: "v1alpha1", Resource: "servicechains"},
	"virtualnetworkfunctions": {Group: "nfv.sdncontroller.io", Version: "v1alpha1", Resource: "virtualnetworkfunctions"},
}

// Kind to resource mapping for create/update
var kindToResource = map[string]string{
	"VPC": "vpcs", "Subnet": "subnets",
	"InternetGateway": "internetgateways", "NATGateway": "natgateways",
	"LoadBalancer": "loadbalancers", "Firewall": "firewalls",
	"Tunnel": "tunnels", "Bond": "bonds",
	"NetworkInterface": "networkinterfaces", "NodeNetworkConfig": "nodenetworkconfigs",
	"ServiceChain": "servicechains", "VirtualNetworkFunction": "virtualnetworkfunctions",
}

func main() {
	flag.StringVar(&listenAddr, "listen", ":3001", "API server listen address")
	flag.StringVar(&staticDir, "static", "", "Path to static frontend files (optional)")
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
				jsonError(w, 500, err.Error())
				return
			}
			jsonResponse(w, 200, obj.Object)
		} else if namespace != "" {
			// List in namespace
			list, err := dynamicClient.Resource(gvr).Namespace(namespace).List(ctx, metav1.ListOptions{})
			if err != nil {
				jsonError(w, 500, err.Error())
				return
			}
			jsonResponse(w, 200, list.Items)
		} else {
			// List all namespaces
			list, err := dynamicClient.Resource(gvr).Namespace("").List(ctx, metav1.ListOptions{})
			if err != nil {
				jsonError(w, 500, err.Error())
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
			jsonError(w, 500, err.Error())
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
			jsonError(w, 404, err.Error())
			return
		}
		obj.SetResourceVersion(existing.GetResourceVersion())
		obj.SetNamespace(namespace)
		obj.SetName(name)
		updated, err := dynamicClient.Resource(gvr).Namespace(namespace).Update(ctx, &obj, metav1.UpdateOptions{})
		if err != nil {
			jsonError(w, 500, err.Error())
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
			jsonError(w, 500, err.Error())
			return
		}
		jsonResponse(w, 200, map[string]string{"status": "deleted", "name": name})

	default:
		jsonError(w, 405, "method not allowed")
	}
}

// GET /api/topology - returns nodes and edges for the network graph
func handleTopology(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	nodes := []map[string]interface{}{}
	edges := []map[string]interface{}{}

	// Fetch all resource types and build topology
	for resource, gvr := range gvrMap {
		list, err := dynamicClient.Resource(gvr).Namespace("").List(ctx, metav1.ListOptions{})
		if err != nil {
			continue
		}
		for _, item := range list.Items {
			name := item.GetName()
			ns := item.GetNamespace()
			kind := item.GetKind()
			id := fmt.Sprintf("%s/%s/%s", resource, ns, name)

			spec, _ := item.Object["spec"].(map[string]interface{})
			status, _ := item.Object["status"].(map[string]interface{})

			node := map[string]interface{}{
				"id":        id,
				"name":      name,
				"namespace": ns,
				"kind":      kind,
				"resource":  resource,
				"spec":      spec,
				"status":    status,
			}
			nodes = append(nodes, node)

			// Build edges based on refs
			if spec != nil {
				if vpcRef, ok := spec["vpcRef"].(string); ok && vpcRef != "" {
					edges = append(edges, map[string]interface{}{
						"source": id,
						"target": fmt.Sprintf("vpcs/%s/%s", ns, vpcRef),
						"type":   "belongs_to",
					})
				}
				if subnetRef, ok := spec["subnetRef"].(string); ok && subnetRef != "" {
					edges = append(edges, map[string]interface{}{
						"source": id,
						"target": fmt.Sprintf("subnets/%s/%s", ns, subnetRef),
						"type":   "attached_to",
					})
				}
				// Service chain -> VNF edges
				if chain, ok := spec["chain"].([]interface{}); ok {
					for _, hop := range chain {
						if h, ok := hop.(map[string]interface{}); ok {
							if vnfRef, ok := h["vnfRef"].(string); ok {
								edges = append(edges, map[string]interface{}{
									"source": id,
									"target": fmt.Sprintf("virtualnetworkfunctions/%s/%s", ns, vnfRef),
									"type":   "uses_vnf",
								})
							}
						}
					}
				}
			}
		}
	}

	jsonResponse(w, 200, map[string]interface{}{
		"nodes": nodes,
		"edges": edges,
	})
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
