import { Badge } from './ui';
import type { OvsBridge, OvsDaemon, OvsNic } from '../ovs';

/** On/Off badge for OVS hw-offload, with explanation tooltip. */
export function HwOffloadBadge({ on }: { on?: boolean }) {
  if (on === undefined) return <span className="text-gray-400">-</span>;
  return (
    <Badge
      tone={on ? 'green' : 'gray'}
      title={on ? 'OVS hardware offload is enabled (other_config:hw-offload)' : 'HW offload is off — datapath flows are processed in software'}
    >
      {on ? 'On' : 'Off'}
    </Badge>
  );
}

/** "eth0: off" style badges for NIC hw-tc-offload state. */
export function NicOffloadBadges({ nics }: { nics: OvsNic[] }) {
  if (nics.length === 0) return <span className="text-gray-400">-</span>;
  return (
    <span className="flex flex-wrap gap-1">
      {nics.map((n) => (
        <Badge key={n.device} tone={n.on ? 'green' : 'gray'} title={`${n.device}: hw-tc-offload ${n.on ? 'on' : 'off'}`}>
          {n.device}: {n.on ? 'on' : 'off'}
        </Badge>
      ))}
    </span>
  );
}

/** Up/down badges for OVS daemons (ovsdb-server, ovs-vswitchd, …). */
export function DaemonBadges({ daemons }: { daemons: OvsDaemon[] }) {
  if (daemons.length === 0) return <span className="text-gray-400">-</span>;
  return (
    <span className="flex flex-wrap gap-1">
      {daemons.map((d) => (
        <Badge key={d.daemon} tone={d.up ? 'green' : 'yellow'} title={`${d.daemon} is ${d.up ? 'up' : 'down'}`}>
          {d.daemon}: {d.up ? 'up' : 'down'}
        </Badge>
      ))}
    </span>
  );
}

/** "table 10: 4" style chips for OpenFlow rules per table. */
export function TableChips({ bridge }: { bridge: OvsBridge }) {
  if (bridge.tables.length === 0) return <span className="text-gray-400">-</span>;
  return (
    <span className="flex flex-wrap gap-1">
      {bridge.tables.map((t) => (
        <span
          key={t.table}
          className="rounded-full bg-gray-100 px-2 py-0.5 text-[11px] text-gray-700 dark:bg-gray-800 dark:text-gray-300"
        >
          table {t.table}: {t.flows}
        </span>
      ))}
    </span>
  );
}
