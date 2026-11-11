import ResourceList from '../components/ResourceList';
import { PageHeader } from '../components/ui';

export default function NFV() {
  return (
    <div className="space-y-8">
      <PageHeader title="NFV" subtitle="Service chains & virtual network functions" />
      <ResourceList resource="servicechains" />
      <ResourceList resource="virtualnetworkfunctions" />
    </div>
  );
}
