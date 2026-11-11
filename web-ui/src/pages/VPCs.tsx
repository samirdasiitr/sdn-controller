import ResourceList from '../components/ResourceList';
import { PageHeader } from '../components/ui';

export default function VPCs() {
  return (
    <div className="space-y-8">
      <PageHeader title="VPCs" subtitle="Virtual private clouds" />
      <ResourceList resource="vpcs" />
    </div>
  );
}
