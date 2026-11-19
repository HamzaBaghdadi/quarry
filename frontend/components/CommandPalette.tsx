"use client";

import { Command } from "cmdk";
import { CloudArrowDown, CloudArrowUp, Code, Moon, Plug, Plus, Table, Lightning } from "@phosphor-icons/react";
import { useStore } from "@/lib/store";
import { EngineBadge } from "./ui";

const item =
  "flex h-9 cursor-pointer items-center gap-2.5 rounded-md px-2.5 text-[13px] text-fg data-[selected=true]:bg-accent data-[selected=true]:text-accent-fg";

export function CommandPalette() {
  const open = useStore((s) => s.palette);
  const connections = useStore((s) => s.connections);
  const tree = useStore((s) => s.tree);
  const drivers = useStore((s) => s.drivers);
  const s = useStore.getState();
  if (!open) return null;

  const close = () => s.setPalette(false);
  const run = (fn: () => void) => () => {
    close();
    fn();
  };

  // Every browsable object we've loaded so far, across connections.
  const objects = Object.entries(tree).flatMap(([k, e]) => {
    const connId = k.split("\u0000")[0];
    return (e.nodes ?? []).filter((n) => n.browsable).map((n) => ({ connId, node: n }));
  });

  return (
    <div className="fixed inset-0 z-50 flex items-start justify-center bg-black/45 pt-[14vh]" onMouseDown={(e) => e.target === e.currentTarget && close()}>
      <Command
        label="Command palette"
        loop
        onKeyDown={(e) => e.key === "Escape" && close()}
        className="animate-pop w-full max-w-[560px] overflow-hidden rounded-xl border border-line-strong bg-bg-1 shadow-pop"
      >
        <Command.Input autoFocus placeholder="Search connections, tables, actions…" className="h-12 w-full border-b border-line bg-transparent px-4 text-[14px] text-fg placeholder:text-fg-subtle focus:outline-none" />
        <Command.List className="max-h-[50vh] overflow-auto p-1.5">
          <Command.Empty className="px-3 py-6 text-center text-[13px] text-fg-subtle">Nothing found.</Command.Empty>

          <Command.Group heading="Actions" className="text-[11px] text-fg-subtle [&_[cmdk-group-heading]]:px-2.5 [&_[cmdk-group-heading]]:py-1.5">
            <Command.Item className={item} onSelect={run(() => s.openDialog())}>
              <Plus aria-hidden className="size-4" /> New connection
            </Command.Item>
            <Command.Item className={item} onSelect={run(() => s.newQuery())}>
              <Code aria-hidden className="size-4" /> New query tab
            </Command.Item>
            <Command.Item className={item} onSelect={run(() => s.toggleTheme())}>
              <Moon aria-hidden className="size-4" /> Toggle light / dark theme
            </Command.Item>
            {connections
              .filter((c) => c.connected && drivers.find((d) => d.id === c.driver)?.capabilities?.includes("backup"))
              .flatMap((c) => [
                <Command.Item key={"b" + c.id} className={item} value={`backup ${c.name}`} onSelect={run(() => s.openBackup(c.id, "backup"))}>
                  <CloudArrowDown aria-hidden className="size-4" /> Back up {c.name}…
                </Command.Item>,
                !c.readOnly && (
                  <Command.Item key={"r" + c.id} className={item} value={`restore ${c.name}`} onSelect={run(() => s.openBackup(c.id, "restore"))}>
                    <CloudArrowUp aria-hidden className="size-4" /> Restore into {c.name}…
                  </Command.Item>
                ),
              ])}
            {connections
              .filter((c) => c.connected)
              .map((c) => (
                <Command.Item key={"d" + c.id} className={item} value={`disconnect ${c.name}`} onSelect={run(() => s.disconnect(c.id))}>
                  <Plug aria-hidden className="size-4" /> Disconnect {c.name}
                </Command.Item>
              ))}
          </Command.Group>

          <Command.Group heading="Connections" className="text-[11px] text-fg-subtle [&_[cmdk-group-heading]]:px-2.5 [&_[cmdk-group-heading]]:py-1.5">
            {connections.map((c) => (
              <Command.Item key={c.id} className={item} value={`connect ${c.name} ${c.driver}`} onSelect={run(() => s.connect(c.id))}>
                <EngineBadge driver={c.driver} color={c.color} size={20} />
                {c.name}
                {c.connected && <Lightning aria-label="connected" weight="fill" className="ml-auto size-3.5 text-run" />}
              </Command.Item>
            ))}
          </Command.Group>

          {objects.length > 0 && (
            <Command.Group heading="Tables & collections" className="text-[11px] text-fg-subtle [&_[cmdk-group-heading]]:px-2.5 [&_[cmdk-group-heading]]:py-1.5">
              {objects.slice(0, 300).map(({ connId, node }) => (
                <Command.Item key={connId + node.path.join("/")} className={item} value={`open ${node.path.join(".")} ${connId}`} onSelect={run(() => s.openTable(connId, node))}>
                  <Table aria-hidden className="size-4" />
                  <span className="truncate">{node.name}</span>
                  <span className="ml-auto truncate font-mono text-[11px] opacity-70">{node.path.slice(0, -1).join(".")}</span>
                </Command.Item>
              ))}
            </Command.Group>
          )}
        </Command.List>
      </Command>
    </div>
  );
}
