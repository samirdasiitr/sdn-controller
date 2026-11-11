import ResourceList from '../components/ResourceList';
import { PageHeader } from '../components/ui';

export default function Services() {
  return (
    <div className="space-y-8">
      <PageHeader title="Services" subtitle="Load balancers & firewalls" />
      <ResourceList resource="loadbalancers" />
      <ResourceList resource="firewalls" />
    </div>
  );
}
