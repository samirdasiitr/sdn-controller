import ResourceList from '../components/ResourceList';
import { PageHeader } from '../components/ui';

export default function Gateways() {
  return (
    <div className="space-y-8">
      <PageHeader title="Gateways" subtitle="Internet & NAT gateways" />
      <ResourceList resource="internetgateways" />
      <ResourceList resource="natgateways" />
    </div>
  );
}
