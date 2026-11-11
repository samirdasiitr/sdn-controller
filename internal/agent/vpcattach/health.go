package vpcattach

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"

	servicev1alpha1 "github.com/sdncontroller/sdncontroller/api/service/v1alpha1"
)

type backendState struct {
	fails     int
	healthy   bool
	lastProbe time.Time
	msg       string
}

// probe checks one backend the way the LB's healthCheck describes.
func probe(hc *servicev1alpha1.HealthCheck, ip string, timeout time.Duration) (bool, string) {
	addr := net.JoinHostPort(ip, fmt.Sprint(hc.Port))
	switch strings.ToUpper(hc.Protocol) {
	case "HTTP", "HTTPS":
		scheme := strings.ToLower(hc.Protocol)
		path := hc.Path
		if path == "" {
			path = "/"
		}
		cl := &http.Client{Timeout: timeout,
			Transport:     &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, DisableKeepAlives: true},
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		resp, err := cl.Get(fmt.Sprintf("%s://%s%s", scheme, addr, path))
		if err != nil {
			return false, err.Error()
		}
		resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 400 {
			return true, ""
		}
		return false, fmt.Sprintf("HTTP %d", resp.StatusCode)
	default:
		c, err := net.DialTimeout("tcp", addr, timeout)
		if err != nil {
			return false, err.Error()
		}
		c.Close()
		return true, ""
	}
}

// neighborMAC reads the MAC the host resolved for ip on the bridge.
func neighborMAC(ctx context.Context, bridge, ip string) string {
	out, err := ovsOut(ctx, "ip", "neigh", "show", ip, "dev", bridge)
	if err != nil {
		return ""
	}
	f := strings.Fields(out)
	for i, w := range f {
		if w == "lladdr" && i+1 < len(f) {
			return f[i+1]
		}
	}
	return ""
}

// probeBackends runs on the router node (the only node with a presence in the VPC subnets). It
// health-checks every backend of every LB in the VPC, discovers the MACs of backends that are
// not VNics, and publishes the result in status.backends where all LB nodes read it.
func (r *Reconciler) probeBackends(ctx context.Context, bridge string, lbs []servicev1alpha1.LoadBalancer, vnicIPs map[string]bool) {
	for i := range lbs {
		lb := &lbs[i]
		var statuses []servicev1alpha1.BackendStatus
		var mu sync.Mutex
		var wg sync.WaitGroup
		hc := lb.Spec.HealthCheck
		threshold := 3
		timeout := 5 * time.Second
		interval := 10 * time.Second
		if hc != nil {
			if hc.UnhealthyThreshold > 0 {
				threshold = int(hc.UnhealthyThreshold)
			}
			if d := hc.Timeout.Duration; d > 0 {
				timeout = d
			}
			if d := hc.Interval.Duration; d > 0 {
				interval = d
			}
		}
		for _, be := range lb.Spec.Backends {
			wg.Add(1)
			go func(ip string) {
				defer wg.Done()
				key := lb.Namespace + "/" + lb.Name + "/" + ip
				mu.Lock()
				st := r.health[key]
				if st == nil {
					st = &backendState{healthy: true}
					r.health[key] = st
				}
				due := hc != nil && time.Since(st.lastProbe) >= interval-time.Second
				mu.Unlock()
				if due {
					ok, msg := probe(hc, ip, timeout)
					mu.Lock()
					st.lastProbe = time.Now()
					if ok {
						st.fails, st.healthy, st.msg = 0, true, ""
					} else {
						st.fails++
						st.msg = msg
						if st.fails >= threshold {
							st.healthy = false
						}
					}
					mu.Unlock()
				}
				bs := servicev1alpha1.BackendStatus{IP: ip, Healthy: st.healthy}
				if !st.healthy {
					bs.Message = st.msg
				}
				if !vnicIPs[ip] {
					mac := neighborMAC(ctx, bridge, ip)
					if mac == "" && len(lb.Spec.Frontend.Ports) > 0 {
						// poke the backend so the host resolves it
						if c, err := net.DialTimeout("tcp", net.JoinHostPort(ip, fmt.Sprint(lb.Spec.Frontend.Ports[0].TargetPort)), time.Second); err == nil {
							c.Close()
						}
						mac = neighborMAC(ctx, bridge, ip)
					}
					bs.MAC = mac
				}
				mu.Lock()
				statuses = append(statuses, bs)
				mu.Unlock()
			}(be.IP)
		}
		wg.Wait()
		sort.Slice(statuses, func(a, b int) bool { return statuses[a].IP < statuses[b].IP })
		cur := append([]servicev1alpha1.BackendStatus(nil), lb.Status.Backends...)
		sort.Slice(cur, func(a, b int) bool { return cur[a].IP < cur[b].IP })
		if reflect.DeepEqual(cur, statuses) || (len(cur) == 0 && len(statuses) == 0) {
			continue
		}
		orig := lb.DeepCopy()
		lb.Status.Backends = statuses
		if err := r.c.Status().Patch(ctx, lb, client.MergeFrom(orig)); err != nil {
			r.log.Error("report backend health", "lb", lb.Name, "error", err)
		}
	}
}
