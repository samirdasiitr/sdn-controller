import ResourceList from '../components/ResourceList';
import { PageHeader } from '../components/ui';

export default function Infra() {
  return (
    <div className="space-y-8">
      <PageHeader title="Infrastructure" subtitle="Tunnels, bonds, interfaces & node network configs" />
      <ResourceList resource="tunnels" />
      <ResourceList resource="bonds" />
      <ResourceList resource="networkinterfaces" />
      <ResourceList resource="nodenetworkconfigs" />
    </div>
  );
}
