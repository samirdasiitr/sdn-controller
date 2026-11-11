import { useState, type ReactNode } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import * as yaml from 'js-yaml';
import { ChevronDown, ChevronRight, Pencil, Trash2 } from 'lucide-react';
import { ApiError, deleteResource, friendlyError, getNodeOvs, getResource, listNodes, listResources } from '../api';
import { isPlaceable, placementOf, placedCondition } from '../placement';
import { useData, useRefresh } from '../context';
import { RESOURCES } from '../resources';
import { age } from '../lib';
import { tagsOf, updateTags, type Tag } from '../tags';
import { TagEditor } from '../components/tags';
import FlowTable, { Explainer, portNamesFor } from '../components/FlowTable';
import { ResourceStatus } from '../components/StatusBadge';
import ResourceModal from '../components/ResourceModal';
import ConfirmDialog from '../components/ConfirmDialog';
import { MetricChart } from '../components/charts';
import {
  Badge, Breadcrumb, Card, EntityLink, EntityLinks, ErrorBanner, KindRefLink, MiniTable, NodeLink,
  PageHeader, ProgressBar, Props, btnSecondary, linkCls,
} from '../components/ui';

const LIST_PAGE: Record<string, [string, string]> = {
  vpcs: ['VPCs', '/vpcs'],
  subnets: ['Subnets', '/subnets'],
  vnics: ['VNICs', '/vnics'],
  internetgateways: ['Gateways', '/gateways'],
  natgateways: ['Gateways', '/gateways'],
  loadbalancers: ['Services', '/services'],
  firewalls: ['Services', '/services'],
  servicechains: ['NFV', '/nfv'],
  virtualnetworkfunctions: ['NFV', '/nfv'],
  tunnels: ['Infrastructure', '/infra'],
  bonds: ['Infrastructure', '/infra'],
  networkinterfaces: ['Infrastructure', '/infra'],
  nodenetworkconfigs: ['Infrastructure', '/infra'],
};

const actionColor = (a: string) =>
  a === 'Allow' ? 'text-green-600 dark:text-green-400' : a === 'Deny' ? 'text-red-600 dark:text-red-400' : 'text-yellow-600 dark:text-yellow-400';

const Code = ({ children }: { children: ReactNode }) => (
  <code className="rounded bg-gray-100 px-1 py-0.5 text-xs dark:bg-gray-800">{children}</code>
);

function Pre({ obj }: { obj: any }) {
  return (
    <pre className="max-h-96 overflow-auto bg-gray-50 p-4 text-xs text-gray-800 dark:bg-gray-950 dark:text-gray-200">
      {yaml.dump(obj ?? {}, { noRefs: true, lineWidth: 120 })}
    </pre>
  );
}

function Labels({ labels }: { labels?: Record<string, string> }) {
  const e = Object.entries(labels ?? {}).filter(([k]) => k !== 'kubectl.kubernetes.io/last-applied-configuration');
  if (!e.length) return null;
  return (
    <span className="flex flex-wrap gap-1">
      {e.map(([k, v]) => (
        <span key={k} className="max-w-full break-all rounded-full bg-gray-100 px-2 py-0.5 text-xs text-gray-700 dark:bg-gray-800 dark:text-gray-300">{k}={v}</span>
      ))}
    </span>
  );
}

/* ---------------- Generic panels ---------------- */

function TagsPanel({ resource, item, onSaved }: { resource: string; item: any; onSaved: () => void }) {
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState<Tag[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const tags = tagsOf(item);
  const md = item.metadata;
  const start = () => { setDraft(tags); setErr(null); setEditing(true); };
  const save = async () => {
    setSaving(true);
    setErr(null);
    try {
      await updateTags(resource, md.namespace, md.name, draft);
      setEditing(false);
      onSaved();
    } catch (e) {
      setErr(friendlyError(e));
    } finally {
      setSaving(false);
    }
  };
  return (
    <Card title={`Tags (${tags.length})`} actions={!editing && <button className={btnSecondary} onClick={start}><Pencil className="h-3.5 w-3.5" /> Edit</button>}>
      <div className="p-5">
        {editing ? (
          <>
            <TagEditor obj={{ metadata: { labels: Object.fromEntries(draft.map((t) => [`tags.sdncontroller.io/${t.key}`, t.value])) } }} onChange={setDraft} />
            {err && <p className="mt-2 text-xs text-red-500">{err}</p>}
            <div className="mt-3 flex gap-2">
              <button className={btnSecondary} onClick={() => setEditing(false)} disabled={saving}>Cancel</button>
              <button className="inline-flex items-center gap-2 rounded-lg bg-blue-600 px-3 py-2 text-sm font-medium text-white hover:bg-blue-500 disabled:opacity-50" onClick={save} disabled={saving}>
                {saving ? 'Saving…' : 'Save tags'}
              </button>
            </div>
          </>
        ) : tags.length === 0 ? (
          <span className="text-sm text-gray-400">No tags — add some to group &amp; filter resources.</span>
        ) : (
          <div className="flex flex-wrap gap-1.5">
            {tags.map((t) => (
              <span key={t.key} className="rounded-full border border-purple-300 bg-purple-100 px-2.5 py-1 text-xs font-medium text-purple-700 dark:border-purple-500/30 dark:bg-purple-500/15 dark:text-purple-300">
                <span className="font-semibold">{t.key}</span>=<span>{t.value === '' ? '∅' : t.value}</span>
              </span>
            ))}
          </div>
        )}
      </div>
    </Card>
  );
}

function References({ item }: { item: any }) {
  const ns = item.metadata.namespace;
  const s = item.spec ?? {};
  const rows: [string, ReactNode][] = [];
  if (s.vpcRef) rows.push(['VPC', <EntityLink resource="vpcs" ns={ns} name={s.vpcRef} />]);
  if (s.subnetRef) rows.push(['Subnet', <EntityLink resource="subnets" ns={ns} name={s.subnetRef} />]);
  if (s.subnetRefs?.length) rows.push(['Subnets', <EntityLinks resource="subnets" ns={ns} names={s.subnetRefs} />]);
  if (s.chain?.length)
    rows.push(['VNFs', <EntityLinks resource="virtualnetworkfunctions" ns={ns} names={s.chain.map((c: any) => c.vnfRef)} />]);
  if (item.kind === 'VNic' && item.status?.vpcRef) rows.push(['VPC', <EntityLink resource="vpcs" ns={ns} name={item.status.vpcRef} />]);
  if (s.firewallRef) rows.push(['Firewall', <EntityLink resource="firewalls" ns={ns} name={s.firewallRef} />]);
  if (item.kind === 'VNic' && item.status?.boundNode) rows.push(['Bound node', <NodeLink name={item.status.boundNode} />]);
  else if (s.nodeName) rows.push(['Node', <NodeLink name={s.nodeName} />]);
  if (item.status?.activeNode) rows.push(['Active node', <NodeLink name={item.status.activeNode} />]);
  if (item.status?.standbyNode) rows.push(['Standby node', <NodeLink name={item.status.standbyNode} />]);
  if (!rows.length) return null;
  return (
    <Card title="Parents & references">
      <Props items={rows} />
    </Card>
  );
}

function Conditions({ conditions }: { conditions?: any[] }) {
  return (
    <Card title="Conditions">
      <MiniTable
        rows={conditions ?? []}
        empty="No conditions"
        cols={[
          { header: 'Type', render: (c) => c.type },
          { header: 'Status', render: (c) => (
            <span className={c.status === 'True' ? 'text-green-600 dark:text-green-400' : c.status === 'False' ? 'text-red-600 dark:text-red-400' : ''}>{c.status}</span>
          ) },
          { header: 'Reason', render: (c) => c.reason },
          { header: 'Message', render: (c) => c.message },
          { header: 'Last transition', render: (c) => age(c.lastTransitionTime) + ' ago' },
        ]}
      />
    </Card>
  );
}

function PlacementPanel({ resource, item }: { resource: string; item: any }) {
  const { data: nodes } = useData(() => listNodes().catch(() => []), []);
  const p = placementOf(resource, item);
  const cond = placedCondition(item);
  return (
    <Card title="Placement">
      <MiniTable
        rows={p}
        empty="Not placed on any node"
        cols={[
          { header: 'Node', render: (x) => <NodeLink name={x.node} /> },
          { header: 'Role', render: (x) => (x.role === 'active' ? <Badge tone="green">active</Badge> : x.role === 'standby' ? <Badge tone="blue">standby</Badge> : <Badge tone="gray">{x.role}</Badge>) },
          { header: 'Internal IP', render: (x) => (nodes ?? []).find((n) => n.name === x.node)?.internalIP ?? '-' },
          { header: 'Ready', render: (x) => {
            const n = (nodes ?? []).find((k) => k.name === x.node);
            return n ? <Badge tone={n.ready ? 'green' : 'yellow'}>{n.ready ? 'Ready' : 'NotReady'}</Badge> : <span className="text-gray-400">unknown</span>;
          } },
        ]}
      />
      {(p.length === 0 || (cond && cond.status !== 'True')) && (
        <div className="border-t border-gray-100 px-5 py-3 text-sm dark:border-gray-800">
          {p.length === 0 && <Badge tone="yellow">Not placed</Badge>}{' '}
          {cond ? <span className="text-gray-600 dark:text-gray-300">{cond.reason ? `${cond.reason}: ` : ''}{cond.message}</span> : null}
        </div>
      )}
      {p.length > 0 && cond?.status === 'True' && cond.message && (
        <div className="border-t border-gray-100 px-5 py-3 text-sm text-gray-600 dark:border-gray-800 dark:text-gray-300">{cond.message}</div>
      )}
    </Card>
  );
}

function VNicPanel({ item }: { item: any }) {
  const sp = item.spec ?? {};
  const st = item.status ?? {};
  return (
    <>
      <Card title="Network identity">
        <Props items={[
          ['IP address', st.ipAddress ? `${st.ipAddress}${st.prefixLength ? '/' + st.prefixLength : ''}` : sp.ipAddress && `${sp.ipAddress} (requested)`],
          ['MAC address', <span className="font-mono">{st.macAddress ?? sp.macAddress}</span>],
          ['Gateway', st.gateway],
          ['MTU', st.mtu ?? sp.mtu],
          ['Port security', sp.portSecurity === false ? 'disabled' : 'enabled'],
          ['Workload', sp.attachment && `${sp.attachment.kind}/${sp.attachment.name}`],
        ]} />
      </Card>
      <Card title="Binding">
        <Props items={[
          ['Node', <NodeLink name={st.boundNode || sp.nodeName} />],
          ['Bridge', st.bridge],
          ['OVS port', st.ovsPort],
          ['OpenFlow port', st.ofPort],
          ['Phase', st.phase],
        ]} />
      </Card>
    </>
  );
}

function VNicTable({ vnics, ns }: { vnics: any[]; ns: string }) {
  return (
    <MiniTable rows={vnics} empty="No VNICs in this subnet" cols={[
      { header: 'Name', render: (v) => <EntityLink resource="vnics" ns={ns} name={v.metadata.name} /> },
      { header: 'IP', render: (v) => v.status?.ipAddress ?? v.spec?.ipAddress ?? '-' },
      { header: 'MAC', render: (v) => <span className="font-mono text-xs">{v.status?.macAddress ?? v.spec?.macAddress ?? '-'}</span> },
      { header: 'Workload', render: (v) => (v.spec?.attachment ? `${v.spec.attachment.kind}/${v.spec.attachment.name}` : '-') },
      { header: 'Node', render: (v) => <NodeLink name={v.status?.boundNode || v.spec?.nodeName} /> },
      { header: 'Phase', render: (v) => <ResourceStatus item={v} showBlockers={false} /> },
    ]} />
  );
}

/* ---------------- Type-specific panels ---------------- */

function FirewallPanel({ item }: { item: any }) {
  const ns = item.metadata.namespace;
  const rules = [...(item.spec?.rules ?? [])].sort((a: any, b: any) => a.priority - b.priority);
  return (
    <>
      <Card title="Firewall">
        <Props items={[
          ['VPC', <EntityLink resource="vpcs" ns={ns} name={item.spec?.vpcRef} />],
          ['Subnets', <EntityLinks resource="subnets" ns={ns} names={item.spec?.subnetRefs} />],
          ['Default action', <span className={actionColor(item.spec?.defaultAction)}>{item.spec?.defaultAction}</span>],
          ['Stateful', item.spec?.stateful ? 'yes' : 'no'],
          ['Active rules', item.status?.activeRules],
          ['Matched packets', item.status?.matchedPackets],
        ]} />
      </Card>
      <Card title={`Rules (${rules.length})`}>
        <MiniTable
          rows={rules}
          empty="No rules — default action applies"
          cols={[
            { header: 'Priority', render: (r) => r.priority },
            { header: 'Name', render: (r) => r.name },
            { header: 'Direction', render: (r) => r.direction },
            { header: 'Action', render: (r) => <b className={actionColor(r.action)}>{r.action}</b> },
            { header: 'Protocol', render: (r) => r.match?.protocol ?? 'any' },
            { header: 'Src port', render: (r) => r.match?.srcPort ?? 'any' },
            { header: 'Dst port', render: (r) => r.match?.dstPort ?? 'any' },
            { header: 'Source', render: (r) => r.match?.srcCIDR ?? 'any' },
            { header: 'Destination', render: (r) => r.match?.dstCIDR ?? 'any' },
            { header: 'Log', render: (r) => (r.logging ? 'yes' : '') },
          ]}
        />
      </Card>
    </>
  );
}

function LBPanel({ item }: { item: any }) {
  const ns = item.metadata.namespace;
  const sp = item.spec ?? {};
  const backends: any[] = sp.backends ?? [];
  const totalW = backends.reduce((a, b) => a + (b.weight ?? 1), 0);
  const active = item.status?.activeBackends;
  const health = active === undefined ? '-' : `${active} / ${backends.length} healthy`;
  return (
    <>
      <Card title="Load balancer">
        <Props items={[
          ['VPC', <EntityLink resource="vpcs" ns={ns} name={sp.vpcRef} />],
          ['Frontend VIP', sp.frontend?.ip],
          ['Type', sp.type],
          ['Algorithm', sp.algorithm],
          ['Health', <span className={active !== undefined && active < backends.length ? 'text-yellow-600 dark:text-yellow-400' : 'text-green-600 dark:text-green-400'}>{health}</span>],
          ['Total connections', item.status?.totalConnections],
          ['Health check', sp.healthCheck && `${sp.healthCheck.protocol}:${sp.healthCheck.port}${sp.healthCheck.path ?? ''} every ${sp.healthCheck.interval ?? '?'}, timeout ${sp.healthCheck.timeout ?? '?'}, unhealthy after ${sp.healthCheck.unhealthyThreshold ?? '?'}`],
          ['Session affinity', sp.sessionAffinity?.enabled ? `enabled (${sp.sessionAffinity.timeout ?? 'default'})` : undefined],
        ]} />
      </Card>
      <Card title="Frontend ports">
        <MiniTable rows={sp.frontend?.ports ?? []} cols={[
          { header: 'VIP:Port', render: (p) => `${sp.frontend?.ip}:${p.port}` },
          { header: 'Protocol', render: (p) => p.protocol },
          { header: 'Target port', render: (p) => p.targetPort },
        ]} />
      </Card>
      <Card title={`Backends (${backends.length})`}>
        <MiniTable rows={backends} empty="No backends" cols={[
          { header: 'IP', render: (b) => b.ip },
          { header: 'Ports', render: () => (sp.frontend?.ports ?? []).map((p: any) => `${p.targetPort}/${p.protocol}`).join(', ') || '-' },
          { header: 'Weight', render: (b) => b.weight ?? 1 },
          { header: 'Share', render: (b) => `${(((b.weight ?? 1) / (totalW || 1)) * 100).toFixed(0)}%` },
          { header: 'Health', render: () => (
            <span className={active === undefined ? 'text-gray-400' : active >= backends.length ? 'text-green-600 dark:text-green-400' : 'text-yellow-600 dark:text-yellow-400'}>
              {active === undefined ? 'unknown' : active >= backends.length ? 'healthy' : 'pool degraded'}
            </span>
          ) },
        ]} />
      </Card>
    </>
  );
}

function SubnetPanel({ item }: { item: any }) {
  const ns = item.metadata.namespace;
  const sp = item.spec ?? {};
  const { data: allVnics } = useData(() => listResources('vnics', ns).catch(() => []), [ns]);
  const vnics = (allVnics ?? []).filter((v) => v.spec?.subnetRef === item.metadata.name);
  const withIP = vnics.filter((v) => v.status?.ipAddress).length;
  const reported = item.status?.allocatedIPs ?? 0;
  const avail0 = item.status?.availableIPs;
  const total = avail0 === undefined ? 0 : reported + avail0;
  // keep the usage bar consistent with VNICs that already hold an IP the status hasn't counted yet
  const extra = Math.max(0, withIP - reported);
  const alloc = reported + extra;
  const avail = avail0 === undefined ? undefined : Math.max(0, avail0 - extra);
  return (
    <>
    <Card title="Subnet">
      <Props items={[
        ['VPC', <EntityLink resource="vpcs" ns={ns} name={sp.vpcRef} />],
        ['CIDR', sp.cidr],
        ['Gateway', item.status?.gateway ?? sp.gateway],
        ['Type', sp.type],
        ['VLAN', sp.vlanID],
        ['MTU', sp.mtu],
        ['DHCP', sp.dhcp ? (sp.dhcp.enabled ? `enabled${sp.dhcp.range ? ` (${sp.dhcp.range.start}–${sp.dhcp.range.end})` : ''}` : 'disabled') : undefined],
        ['Allocated IPs', alloc],
        ['Available IPs', avail],
        ['Total IPs', total || undefined],
      ]} />
      <div className="px-5 pb-5">
        <ProgressBar value={alloc} max={total} label={`IP usage (${withIP} VNIC${withIP === 1 ? '' : 's'} with an IP)`} />
      </div>
    </Card>
    <Card title={`VNICs in this subnet (${vnics.length})`}>
      <VNicTable vnics={vnics} ns={ns} />
    </Card>
    </>
  );
}

function NodeFlowsSection({ node, bridge }: { node: string; bridge?: string }) {
  const [open, setOpen] = useState(false);
  const { data: ovsTopo, error } = useData(() => (open ? getNodeOvs(node) : Promise.resolve(null)), [node, open]);
  const br = bridge ?? ovsTopo?.bridges?.[0]?.name;
  return (
    <div className="border-b border-gray-100 last:border-0 dark:border-gray-800">
      <button className="flex w-full items-center gap-2 px-5 py-3 text-left text-sm hover:bg-gray-50 dark:hover:bg-gray-800/50" onClick={() => setOpen((o) => !o)}>
        {open ? <ChevronDown className="h-4 w-4 text-gray-400" /> : <ChevronRight className="h-4 w-4 text-gray-400" />}
        <NodeLink name={node} />
        <span className="text-gray-400">· bridge</span>
        <span className="font-mono text-xs">{bridge ?? '?'}</span>
      </button>
      {open && (
        <div className="border-t border-gray-100 dark:border-gray-800">
          {error ? (
            <div className="px-5 py-4 text-sm text-red-500">Node agent on {node} unreachable: {error}</div>
          ) : !br ? (
            <div className="px-5 py-4 text-sm text-gray-400">No bridge reported{ovsTopo === null ? ' (agent unreachable)' : ''}.</div>
          ) : (
            <FlowTable node={node} bridge={br} portNames={ovsTopo ? portNamesFor(ovsTopo.bridges, br) : undefined} />
          )}
        </div>
      )}
    </div>
  );
}

function VPCPanel({ item }: { item: any }) {
  const ns = item.metadata.namespace;
  const sp = item.spec ?? {};
  const st = item.status ?? {};
  const { data: subnets } = useData(() => listResources('subnets', ns), [ns]);
  const mine = (subnets ?? []).filter((s) => s.spec?.vpcRef === item.metadata.name);
  const { data: vnics } = useData(() => listResources('vnics', ns).catch(() => []), [ns]);
  const mineSubnets = new Set(mine.map((s) => s.metadata.name));
  const vnicCount = (vnics ?? []).filter((v) => v.status?.vpcRef === item.metadata.name || mineSubnets.has(v.spec?.subnetRef)).length;
  return (
    <>
      <Card title="VPC">
        <Props items={[
          ['CIDR', sp.cidr],
          ['Secondary CIDRs', (sp.secondaryCIDRs ?? []).join(', ')],
          ['Tunnel type', sp.tunnelType],
          ['VNI', sp.vni ?? st.allocatedVNI],
          ['MTU', sp.mtu],
          ['DNS', sp.enableDNS ? 'enabled' : 'disabled'],
          ['Active nodes', st.activeNodes],
          ['Allocated subnets', st.allocatedSubnets],
          ['VNICs', vnicCount],
          ['Node selector', <Labels labels={sp.nodeSelector?.matchLabels} />],
        ]} />
      </Card>
      <Card title={`Subnets in this VPC (${mine.length})`}>
        <MiniTable rows={mine} empty="No subnets" cols={[
          { header: 'Name', render: (s) => <EntityLink resource="subnets" ns={ns} name={s.metadata.name} /> },
          { header: 'CIDR', render: (s) => s.spec?.cidr },
          { header: 'Gateway', render: (s) => s.spec?.gateway ?? s.status?.gateway },
          { header: 'IPs used', render: (s) => (s.status?.availableIPs === undefined ? '-' : `${s.status?.allocatedIPs ?? 0} / ${(s.status?.allocatedIPs ?? 0) + s.status.availableIPs}`) },
          { header: 'Status', render: (s) => <ResourceStatus item={s} /> },
        ]} />
      </Card>
      <Card title={`Attached nodes (${st.attachedNodes?.length ?? 0})`}>
        <MiniTable rows={st.attachedNodes ?? []} empty="No nodes attached" cols={[
          { header: 'Node', render: (n) => <NodeLink name={n.nodeName} /> },
          { header: 'Tunnel IP', render: (n) => n.tunnelIP },
          { header: 'Bridge', render: (n) => n.bridge },
        ]} />
      </Card>
      {(st.attachedNodes?.length ?? 0) > 0 && (
        <Card title={`OpenFlow rules per attached node (${st.attachedNodes.length})`}>
          <Explainer />
          <div>
            {st.attachedNodes.map((n: any) => (
              <NodeFlowsSection key={n.nodeName} node={n.nodeName} bridge={n.bridge} />
            ))}
          </div>
        </Card>
      )}
      <Card title={`Route table (${st.routes?.length ?? 0})`}>
        <MiniTable rows={st.routes ?? []} empty="No routes" cols={[
          { header: 'Destination', render: (r) => r.destination },
          { header: 'Next-hop node', render: (r) => <NodeLink name={r.nextHopNode} /> },
          { header: 'Next-hop IP', render: (r) => r.nextHopIP },
          { header: 'Type', render: (r) => r.type },
        ]} />
      </Card>
      <Card title={`Used by (${st.usedBy?.length ?? 0})`}>
        {(st.usedBy?.length ?? 0) === 0 ? (
          <div className="px-5 py-6 text-center text-sm text-gray-400">Nothing depends on this VPC</div>
        ) : (
          <ul className="flex flex-wrap gap-2 p-5">
            {st.usedBy.map((u: string) => (
              <li key={u} className="rounded-full border border-gray-200 px-3 py-1 text-sm dark:border-gray-700">
                <KindRefLink value={u} ns={ns} />
              </li>
            ))}
          </ul>
        )}
      </Card>
    </>
  );
}

function GatewayPanel({ item, resource }: { item: any; resource: string }) {
  const ns = item.metadata.namespace;
  const sp = item.spec ?? {};
  const st = item.status ?? {};
  const nat = resource === 'natgateways';
  return (
    <>
      <Card title={nat ? 'NAT gateway' : 'Internet gateway'}>
        <Props items={[
          ['VPC', <EntityLink resource="vpcs" ns={ns} name={sp.vpcRef} />],
          ['Active node', <NodeLink name={st.activeNode} />],
          ...(!nat ? [['Standby node', <NodeLink name={st.standbyNode} />] as [string, ReactNode]] : []),
          [nat ? 'External IP' : 'External IPs', nat ? sp.externalIP : (sp.externalIPs ?? []).join(', ')],
          ...(!nat ? [['External interface', sp.externalInterface] as [string, ReactNode]] : []),
          ['Type', sp.type],
          ['HA', sp.ha ? `${sp.ha.enabled ? 'enabled' : 'disabled'}${sp.ha.mode ? ` (${sp.ha.mode})` : ''}` : undefined],
          ['Port range', sp.portRange && `${sp.portRange.start}–${sp.portRange.end}`],
          ['Translations', st.translationCount],
        ]} />
      </Card>
      {nat && (
        <Card title="Attached subnets">
          <MiniTable rows={(sp.subnetRefs ?? []).map((n: string) => ({ n }))} empty="No subnets" cols={[
            { header: 'Subnet', render: (r) => <EntityLink resource="subnets" ns={ns} name={r.n} /> },
          ]} />
        </Card>
      )}
    </>
  );
}

function ChainPanel({ item }: { item: any }) {
  const ns = item.metadata.namespace;
  const sp = item.spec ?? {};
  return (
    <>
      <Card title="Service chain">
        <Props items={[
          ['VPC', <EntityLink resource="vpcs" ns={ns} name={sp.vpcRef} />],
          ['Subnet', <EntityLink resource="subnets" ns={ns} name={sp.subnetRef} />],
          ['Direction', sp.direction],
          ['Chain length', item.status?.chainLength],
          ['Processed packets', item.status?.processedPackets],
          ['Match', sp.match && Object.entries(sp.match).map(([k, v]) => `${k}=${v}`).join(', ')],
          ['Offload', sp.offload?.enabled ? 'enabled' : undefined],
        ]} />
      </Card>
      <Card title={`Hops (${sp.chain?.length ?? 0})`}>
        <ol className="space-y-2 p-5">
          {(sp.chain ?? []).map((h: any, i: number) => (
            <li key={i} className="flex items-center gap-3 rounded-lg border border-gray-200 px-4 py-2 text-sm dark:border-gray-700">
              <span className="flex h-6 w-6 items-center justify-center rounded-full bg-blue-100 text-xs font-bold text-blue-700 dark:bg-blue-500/20 dark:text-blue-300">{i + 1}</span>
              <span className="font-medium">{h.name}</span>
              <span className="text-gray-400">→</span>
              <EntityLink resource="virtualnetworkfunctions" ns={ns} name={h.vnfRef} />
              <span className="ml-auto text-xs text-gray-500">in <Code>{h.portPair?.ingress}</Code> out <Code>{h.portPair?.egress}</Code></span>
            </li>
          ))}
          {!(sp.chain?.length) && <li className="text-center text-sm text-gray-400">Empty chain</li>}
        </ol>
      </Card>
    </>
  );
}

function VNFPanel({ item }: { item: any }) {
  const ns = item.metadata.namespace;
  const sp = item.spec ?? {};
  const { data: chains } = useData(() => listResources('servicechains', ns), [ns]);
  const users = (chains ?? []).filter((c) => (c.spec?.chain ?? []).some((h: any) => h.vnfRef === item.metadata.name));
  return (
    <>
      <Card title="Virtual network function">
        <Props items={[
          ['Type', sp.type],
          ['Image', sp.image ?? sp.vmConfig?.image],
          ['VM config', sp.vmConfig && `${sp.vmConfig.vcpus ?? '?'} vCPU, ${sp.vmConfig.memoryMB ?? '?'} MB`],
          ['Replicas', `${item.status?.readyReplicas ?? 0} ready / ${sp.replicas ?? 1} desired`],
          ['CPU', sp.resources?.cpu],
          ['Memory', sp.resources?.memory],
        ]} />
      </Card>
      <Card title="Interfaces">
        <MiniTable rows={sp.interfaces ?? []} cols={[
          { header: 'Name', render: (i) => i.name },
          { header: 'Type', render: (i) => i.type },
        ]} />
      </Card>
      <Card title={`Used in service chains (${users.length})`}>
        <MiniTable rows={users} empty="Not used by any chain" cols={[
          { header: 'Chain', render: (c) => <EntityLink resource="servicechains" ns={ns} name={c.metadata.name} /> },
          { header: 'Position', render: (c) => (c.spec.chain as any[]).map((h, i) => (h.vnfRef === item.metadata.name ? i + 1 : null)).filter(Boolean).join(', ') },
        ]} />
      </Card>
    </>
  );
}

function TunnelPanel({ item }: { item: any }) {
  const sp = item.spec ?? {};
  const st = item.status ?? {};
  const ep = (e: any) => (e ? <span><NodeLink name={e.nodeName} /> <span className="text-gray-500">({e.ip})</span></span> : undefined);
  return (
    <Card title="Tunnel">
      <Props items={[
        ['Type', sp.type],
        ['VNI', sp.vni],
        ['Local endpoint', ep(sp.localEndpoint)],
        ['Remote endpoint', ep(sp.remoteEndpoint)],
        ['MTU', sp.mtu],
        ['Checksum', sp.checksumEnabled ? 'enabled' : 'disabled'],
        ['Offload', sp.offload?.enabled ? `enabled${sp.offload.driver ? ` (${sp.offload.driver})` : ''}` : undefined],
        ['Offload active', st.offloadActive === undefined ? undefined : String(st.offloadActive)],
        ['TX packets', st.txPackets],
        ['RX packets', st.rxPackets],
      ]} />
    </Card>
  );
}

function metricsFor(resource: string, item: any): ReactNode {
  const name = item.metadata.name, ns = item.metadata.namespace;
  const sel = `name="${name}",namespace="${ns}"`;
  switch (resource) {
    case 'firewalls':
      return <MetricChart title="Firewall rule count (sdn_firewall_rules)" query={`sdn_firewall_rules{${sel}}`} labelKey="name" />;
    case 'loadbalancers':
      return <MetricChart title="Backend count (sdn_loadbalancer_backends)" query={`sdn_loadbalancer_backends{${sel}}`} labelKey="name" />;
    case 'subnets':
      return <MetricChart title="Available IPs (sdn_subnet_info)" query={`sdn_subnet_info{${sel}}`} labelKey="name" />;
    case 'vpcs':
      return <MetricChart title="Available IPs per subnet in this VPC" query={`sdn_subnet_info{vpc="${name}",namespace="${ns}"}`} labelKey="name" />;
    default:
      return null;
  }
}

/* ---------------- Page ---------------- */

export default function EntityDetail() {
  const { type = '', ns = '', name = '' } = useParams();
  const navigate = useNavigate();
  const { refresh } = useRefresh();
  const meta = RESOURCES[type];
  const { data: item, error, loading } = useData(() => getResource(type, ns, name), [type, ns, name]);
  const [editing, setEditing] = useState(false);
  const [confirm, setConfirm] = useState(false);
  const [delError, setDelError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  if (!meta) return <ErrorBanner message={`Unknown resource type "${type}"`} />;
  const [listLabel, listPath] = LIST_PAGE[type];
  const crumbs = [
    { label: 'Dashboard', to: '/' },
    { label: listLabel, to: listPath },
    { label: meta.label + 's' },
    { label: name },
  ];
  // the 4th crumb is plain text; drop the redundant kind crumb for clarity
  crumbs.splice(2, 1);

  const gone = error && /not found|404/i.test(error);
  if (loading && !item) return <div><Breadcrumb items={crumbs} /><div className="text-gray-400">Loading…</div></div>;
  if (!item) {
    return (
      <div>
        <Breadcrumb items={crumbs} />
        <ErrorBanner message={gone ? `${meta.label} "${name}" no longer exists (it may have been deleted).` : error} />
        <Link to={listPath} className={linkCls}>← Back to {listLabel}</Link>
      </div>
    );
  }

  const doDelete = async () => {
    setBusy(true);
    setDelError(null);
    try {
      await deleteResource(type, ns, name);
      setConfirm(false);
      refresh();
    } catch (e) {
      if (e instanceof ApiError && e.status === 404) navigate(listPath);
      else setDelError(friendlyError(e));
    } finally {
      setBusy(false);
    }
  };

  const terminating = !!item.metadata?.deletionTimestamp;
  const usedBy: string[] = item.status?.usedBy ?? [];
  const md = item.metadata;

  return (
    <div className="space-y-6">
      <Breadcrumb items={crumbs} />
      <PageHeader
        title={<span className="flex items-center gap-3">{name} <ResourceStatus item={item} showBlockers={false} /></span>}
        subtitle={`${meta.label} · namespace ${ns}`}
        actions={
          <>
            <button className={btnSecondary} onClick={() => setEditing(true)}><Pencil className="h-4 w-4" /> Edit</button>
            <button className="inline-flex items-center gap-2 rounded-lg border border-red-300 px-3 py-2 text-sm font-medium text-red-600 hover:bg-red-50 dark:border-red-500/40 dark:text-red-400 dark:hover:bg-red-500/10"
              onClick={() => { setDelError(null); setConfirm(true); }}>
              <Trash2 className="h-4 w-4" /> Delete
            </button>
          </>
        }
      />
      <ErrorBanner message={error} />

      {terminating && (
        <div className="rounded-lg border border-orange-300 bg-orange-50 px-4 py-3 text-sm text-orange-800 dark:border-orange-500/40 dark:bg-orange-500/10 dark:text-orange-300">
          This resource is <b>Terminating</b> (since {age(md.deletionTimestamp)} ago).{' '}
          {usedBy.length > 0 ? (
            <>Deletion is blocked by finalizers while it is still used by:{' '}
              {usedBy.map((u, i) => <span key={u}><KindRefLink value={u} ns={ns} />{i < usedBy.length - 1 && ', '}</span>)}.
              Remove those dependents and it will finish deleting.</>
          ) : 'Waiting for finalizers to complete.'}
        </div>
      )}

      <Card title="Metadata">
        <Props items={[
          ['Name', md.name],
          ['Namespace', md.namespace],
          ['Kind', item.kind],
          ['UID', md.uid],
          ['Created', md.creationTimestamp && `${new Date(md.creationTimestamp).toLocaleString()} (${age(md.creationTimestamp)} ago)`],
          ['Generation', md.generation],
          ['Resource version', md.resourceVersion],
          ['Labels', <Labels labels={md.labels} />],
          ['Annotations', <Labels labels={md.annotations} />],
          ['Finalizers', (md.finalizers ?? []).join(', ')],
          ['Owner', (md.ownerReferences ?? []).map((o: any) => `${o.kind}/${o.name}`).join(', ')],
          ['Deletion timestamp', md.deletionTimestamp],
        ]} />
      </Card>

      <TagsPanel resource={type} item={item} onSaved={refresh} />

      <References item={item} />

      {type === 'firewalls' && <FirewallPanel item={item} />}
      {type === 'loadbalancers' && <LBPanel item={item} />}
      {type === 'subnets' && <SubnetPanel item={item} />}
      {type === 'vpcs' && <VPCPanel item={item} />}
      {(type === 'natgateways' || type === 'internetgateways') && <GatewayPanel item={item} resource={type} />}
      {type === 'servicechains' && <ChainPanel item={item} />}
      {type === 'virtualnetworkfunctions' && <VNFPanel item={item} />}
      {type === 'tunnels' && <TunnelPanel item={item} />}
      {type === 'vnics' && <VNicPanel item={item} />}
      {isPlaceable(type) && <PlacementPanel resource={type} item={item} />}

      {metricsFor(type, item)}

      {type !== 'vpcs' && usedBy.length > 0 && (
        <Card title="Used by">
          <ul className="flex flex-wrap gap-2 p-5">
            {usedBy.map((u) => <li key={u} className="rounded-full border border-gray-200 px-3 py-1 text-sm dark:border-gray-700"><KindRefLink value={u} ns={ns} /></li>)}
          </ul>
        </Card>
      )}

      <Conditions conditions={item.status?.conditions} />

      <div className="grid gap-6 lg:grid-cols-2">
        <Card title="Spec"><Pre obj={item.spec} /></Card>
        <Card title="Status"><Pre obj={item.status} /></Card>
      </div>

      <ResourceModal resource={type} open={editing} target={{ namespace: ns, name }} onClose={() => setEditing(false)} onSaved={refresh} />
      <ConfirmDialog
        open={confirm}
        title={`Delete ${meta.label}`}
        message={<>Delete <b>{name}</b>? Deletion is blocked while other resources depend on it.</>}
        error={delError}
        busy={busy}
        onConfirm={doDelete}
        onCancel={() => setConfirm(false)}
      />
    </div>
  );
}
