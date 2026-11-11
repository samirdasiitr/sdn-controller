import { useMemo } from 'react';
import { queryMetric } from '../api';
import { useData } from '../context';
import { MetricChart, FORMATS, Stat, scalar } from '../components/charts';
import { Badge, Card, ErrorBanner, MiniTable, NodeLink, PageHeader } from '../components/ui';
import { DaemonBadges, HwOffloadBadge, NicOffloadBadges, TableChips } from '../components/ovsui';
import { fmtBytes, fmtNum } from '../lib';
import { fmtOrDash, fmtPct, useOvs } from '../ovs';
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

export default function Metrics() {
  const totals = useData(() => Promise.all(TOTALS.map(([, q]) => queryMetric(q))));
  const ovs = useOvs();
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

      <h2 className="text-sm font-semibold uppercase tracking-wide text-gray-500 dark:text-gray-400">OVS rules &amp; offload</h2>

      <Card title="OVS per node">
        <MiniTable
          rows={ovs.data?.nodes ?? []}
          empty="No OVS rule/offload metrics"
          cols={[
            { header: 'Node', render: (r) => <NodeLink name={r.node} /> },
            { header: 'Bridges', render: (r) => r.bridges },
            { header: 'Ports', render: (r) => r.ports },
            { header: 'OpenFlow rules', render: (r) => fmtNum(r.rules) },
            { header: 'Datapath flows', render: (r) => fmtOrDash(r.dpFlows) },
            { header: 'Offloaded', render: (r) => fmtOrDash(r.offloaded) },
            { header: 'Software', render: (r) => fmtOrDash(r.software) },
            { header: 'Offload %', render: (r) => fmtPct(r.offloadPct) },
            { header: 'HW offload', render: (r) => <HwOffloadBadge on={r.hwOffload} /> },
            { header: 'NIC hw-tc-offload', render: (r) => <NicOffloadBadges nics={r.nics} /> },
            { header: 'Daemons', render: (r) => <DaemonBadges daemons={r.daemons} /> },
          ]}
        />
      </Card>

      <Card title="OVS bridges">
        <MiniTable
          rows={ovs.data?.bridges ?? []}
          empty="No OVS bridge metrics"
          cols={[
            { header: 'Node', render: (b) => <NodeLink name={b.node} /> },
            { header: 'Bridge', render: (b) => <span className="font-mono">{b.bridge}</span> },
            { header: 'State', render: (b) => (b.up === undefined ? '-' : <Badge tone={b.up ? 'green' : 'yellow'}>{b.up ? 'up' : 'down'}</Badge>) },
            { header: 'Ports', render: (b) => b.ports ?? '-' },
            { header: 'OpenFlow rules', render: (b) => b.flows ?? '-' },
            { header: 'Rule pps', render: (b) => (b.pps === undefined ? '-' : `${fmtNum(b.pps)}/s`) },
            { header: 'Rule Bps', render: (b) => (b.bps === undefined ? '-' : `${fmtBytes(b.bps)}/s`) },
            { header: 'Rules per table', render: (b) => <TableChips bridge={b} /> },
          ]}
        />
      </Card>

      <div className="grid gap-6 lg:grid-cols-2">
        <MetricChart title="OpenFlow rules per bridge" query="sdn_ovs_bridge_flows" labelKey={['node', 'bridge']} />
        <MetricChart
          title="Datapath flows (offloaded vs software)"
          query="sdn_ovs_datapath_flows_offloaded or sdn_ovs_datapath_flows_software"
          labelFn={(m) => `${m.node} ${m.__name__?.includes('offloaded') ? 'offloaded' : 'software'}`}
          stacked
        />
        <MetricChart
          title="Datapath lookup rate (/s)"
          query="sum by (node, result) (rate(sdn_ovs_datapath_lookups[1m]))"
          labelFn={(m) => `${m.node} ${m.result}`}
        />
      </div>

      <div className="grid gap-6 lg:grid-cols-2">
        <MetricChart title="Port RX throughput" query="sum by (node,bridge,port) (rate(sdn_ovs_port_rx_bytes[1m]))" labelKey={['bridge', 'port']} fmt={FORMATS.bps} />
        <MetricChart title="Port TX throughput" query="sum by (node,bridge,port) (rate(sdn_ovs_port_tx_bytes[1m]))" labelKey={['bridge', 'port']} fmt={FORMATS.bps} />
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
