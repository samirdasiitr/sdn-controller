import { getTopology, listNodes } from '../api';
import { useData } from '../context';
import { countByResource, hostedByNode } from '../placement';
import { Badge, Card, ErrorBanner, MiniTable, NodeLink, PageHeader } from '../components/ui';

export default function Nodes() {
  const nodes = useData(listNodes);
  const topo = useData(() => getTopology().catch(() => undefined));
  const hosted = hostedByNode(topo.data);
  return (
    <div className="space-y-6">
      <PageHeader title="Nodes" subtitle="Kubernetes nodes and the SDN services they host" />
      <ErrorBanner message={nodes.error} />
      <Card>
        <MiniTable rows={nodes.data ?? []} empty={nodes.loading ? 'Loading…' : 'No nodes'} cols={[
          { header: 'Name', render: (n) => <NodeLink name={n.name} /> },
          { header: 'Internal IP', render: (n) => n.internalIP },
          { header: 'Ready', render: (n) => <Badge tone={n.ready ? 'green' : 'yellow'}>{n.ready ? 'Ready' : 'NotReady'}</Badge> },
          { header: 'Hosted', render: (n) => hosted.get(n.name)?.length ?? 0 },
          { header: 'Breakdown', render: (n) => {
            const c = countByResource(hosted.get(n.name));
            return Object.entries(c).map(([k, v]) => `${v} ${k}`).join(', ') || '-';
          } },
        ]} />
      </Card>
    </div>
  );
}
