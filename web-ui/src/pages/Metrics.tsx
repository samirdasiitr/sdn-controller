import { useMemo } from 'react';
import { queryMetric } from '../api';
import { useData } from '../context';
import { MetricChart, FORMATS, Stat, scalar } from '../components/charts';
import { Card, ErrorBanner, MiniTable, PageHeader } from '../components/ui';
import { fmtNum } from '../lib';
import { GRAFANA_URL } from '../resources';

const TOTALS: [string, string][] = [
  ['VPCs', 'sdn_vpc_total'],
  ['Subnets', 'sdn_subnet_total'],
  ['Internet GWs', 'sdn_internetgateway_total'],
  ['NAT GWs', 'sdn_natgateway_total'],
  ['Load balancers', 'sdn_loadbalancer_total'],
  ['Firewalls', 'sdn_firewall_total'],
  ['Service chains', 'sdn_servicechain_total'],
  ['VNFs', 'sdn_vnf_total'],
  ['Tunnels', 'sdn_tunnel_total'],
  ['OVS bridges', 'sdn_ovs_bridge_total'],
  ['Nodes', 'sdn_node_total'],
];

export function useBridgeTable() {
  return useData(async () => {
    const [up, ports, flows] = await Promise.all([
      queryMetric('sdn_ovs_bridge_up'),
      queryMetric('sdn_ovs_bridge_ports'),
      queryMetric('sdn_ovs_bridge_flows'),
    ]);
    const key = (m: Record<string, string>) => `${m.node}/${m.bridge}`;
    const map = new Map<string, { node: string; bridge: string; up: boolean; ports?: number; flows?: number }>();
    for (const s of up) map.set(key(s.metric), { node: s.metric.node, bridge: s.metric.bridge, up: Number(s.value?.[1]) === 1 });
    for (const s of ports) { const r = map.get(key(s.metric)); if (r) r.ports = Number(s.value?.[1]); }
    for (const s of flows) { const r = map.get(key(s.metric)); if (r) r.flows = Number(s.value?.[1]); }
    return [...map.values()].sort((a, b) => key(a as any).localeCompare(key(b as any)));
  });
}

export default function Metrics() {
  const totals = useData(() => Promise.all(TOTALS.map(([, q]) => queryMetric(q))));
  const bridges = useBridgeTable();
  const totalVals = useMemo(() => totals.data?.map((s) => scalar(s)), [totals.data]);

  return (
    <div className="space-y-6">
      <PageHeader
        title="Metrics"
        subtitle="Live data from Prometheus (via /api/metrics)"
        actions={<a className="text-sm text-blue-600 hover:underline dark:text-blue-400" href={GRAFANA_URL} target="_blank" rel="noreferrer">Open Grafana ↗</a>}
      />
      <ErrorBanner message={totals.error && `Prometheus unavailable: ${totals.error}`} />

      <div className="grid grid-cols-2 gap-4 md:grid-cols-4 xl:grid-cols-6">
        {TOTALS.map(([label], i) => (
          <Stat key={label} label={label} value={totalVals?.[i] === undefined ? '-' : fmtNum(totalVals[i]!)} />
        ))}
      </div>

      <Card title="OVS bridges per node">
        <MiniTable
          rows={bridges.data ?? []}
          empty="No OVS bridge metrics"
          cols={[
            { header: 'Node', render: (r) => r.node },
            { header: 'Bridge', render: (r) => r.bridge },
            { header: 'State', render: (r) => <span className={r.up ? 'font-medium text-green-600 dark:text-green-400' : 'font-medium text-red-600 dark:text-red-400'}>{r.up ? 'Up' : 'Down'}</span> },
            { header: 'Ports', render: (r) => r.ports ?? '-' },
            { header: 'Flows', render: (r) => r.flows ?? '-' },
          ]}
        />
      </Card>

      <div className="grid gap-6 lg:grid-cols-2">
        <MetricChart title="Port RX throughput" query="sum by (node,bridge,port) (rate(sdn_ovs_port_rx_bytes[1m]))" labelKey={['bridge', 'port']} fmt={FORMATS.bps} />
        <MetricChart title="Port TX throughput" query="sum by (node,bridge,port) (rate(sdn_ovs_port_tx_bytes[1m]))" labelKey={['bridge', 'port']} fmt={FORMATS.bps} />
        <MetricChart title="Flows per bridge" query="sdn_ovs_bridge_flows" labelKey={['bridge']} />
        <MetricChart title="Controller reconcile rate (/s)" query="sum by (controller) (rate(controller_runtime_reconcile_total[1m]))" labelKey="controller" />
        <MetricChart title="Reconcile errors (/s)" query="sum by (controller) (rate(controller_runtime_reconcile_errors_total[1m]))" labelKey="controller" />
        <MetricChart title="Workqueue depth" query="workqueue_depth" labelKey="name" />
        <MetricChart title="Port drops & errors (/s)" query="sum(rate(sdn_ovs_port_rx_drops[1m])) + sum(rate(sdn_ovs_port_tx_drops[1m]))" labelFn={() => 'drops'} />
        <MetricChart title="Firewall rules" query="sdn_firewall_rules" labelKey="name" />
        <MetricChart title="Load balancer backends" query="sdn_loadbalancer_backends" labelKey="name" />
        <MetricChart title="Available IPs per subnet" query="sdn_subnet_info" labelKey="name" />
      </div>
    </div>
  );
}
