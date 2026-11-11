import ResourceList from '../components/ResourceList';
import { PageHeader } from '../components/ui';

export default function Subnets() {
  return (
    <div className="space-y-8">
      <PageHeader title="Subnets" subtitle="Subnets inside VPCs" />
      <ResourceList resource="subnets" />
    </div>
  );
}
