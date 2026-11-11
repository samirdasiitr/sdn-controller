import ResourceList from '../components/ResourceList';
import { PageHeader } from '../components/ui';

export default function VNICs() {
  return (
    <div className="space-y-8">
      <PageHeader title="VNICs" subtitle="Workload virtual NICs attached to subnets" />
      <ResourceList resource="vnics" title="VNICs" />
    </div>
  );
}
