"use client";

import clsx from "clsx";
import { memo, useMemo, useState, type KeyboardEvent } from "react";
import {
  ArrowClockwise,
  CaretRight,
  CloudArrowDown,
  Database,
  Eye,
  FolderSimple,
  Key,
  MagnifyingGlass,
  Plus,
  Stack,
  Table,
  Warning,
} from "@phosphor-icons/react";
import type { NodeKind, TreeNode } from "@/lib/api";
import { treeKey, useStore } from "@/lib/store";
import { IconButton, Input, Spinner } from "./common";

const icons: Record<NodeKind, typeof Database> = {
  database: Database,
  schema: FolderSimple,
  table: Table,
  view: Eye,
  collection: Stack,
  key: Key,
  group: FolderSimple,
};

const iconTint: Partial<Record<NodeKind, string>> = {
  database: "text-[#7aa2f7]",
  schema: "text-[#e0af68]",
  table: "text-fg-muted",
  view: "text-[#bb9af7]",
  collection: "text-[#73daca]",
  key: "text-[#ff9e64]",
};

/** Arrow-key navigation across visible tree items (roving focus). */
function onTreeKey(e: KeyboardEvent<HTMLDivElement>) {
  const items = Array.from(e.currentTarget.querySelectorAll<HTMLElement>('[role="treeitem"]'));
  const i = items.indexOf(document.activeElement as HTMLElement);
  if (i < 0) return;
  const go = (j: number) => {
    e.preventDefault();
    items[Math.max(0, Math.min(items.length - 1, j))]?.focus();
  };
  if (e.key === "ArrowDown") go(i + 1);
  else if (e.key === "ArrowUp") go(i - 1);
  else if (e.key === "Home") go(0);
  else if (e.key === "End") go(items.length - 1);
}

export function Explorer({ connId }: { connId: string }) {
  const [filter, setFilter] = useState("");
  const tree = useStore((s) => s.tree);
  const loadChildren = useStore((s) => s.loadChildren);
  const root = tree[treeKey(connId, [])];

  // When filtering, search every node we've already loaded for this connection.
  const matches = useMemo(() => {
    const q = filter.trim().toLowerCase();
    if (!q) return null;
    const out: TreeNode[] = [];
    const prefix = connId + "\u0000";
    for (const [k, entry] of Object.entries(tree)) {
      if (!k.startsWith(prefix) && k !== connId + "\u0000") continue;
      for (const n of entry.nodes ?? []) if (n.name.toLowerCase().includes(q)) out.push(n);
    }
    return out.slice(0, 500);
  }, [filter, tree, connId]);

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="flex items-center gap-1 px-2 pb-2">
        <div className="relative flex-1">
          <MagnifyingGlass aria-hidden className="pointer-events-none absolute left-2 top-1/2 size-3.5 -translate-y-1/2 text-fg-subtle" />
          <Input
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
            onKeyDown={(e) => e.key === "Escape" && setFilter("")}
            placeholder="Filter loaded items"
            aria-label="Filter explorer"
            spellCheck={false}
            className="h-7 pl-7 text-[12px]"
          />
        </div>
        <IconButton label="Refresh explorer" onClick={() => loadChildren(connId, [], true)}>
          <ArrowClockwise className="size-3.5" />
        </IconButton>
      </div>

      <div role="tree" aria-label="Database explorer" onKeyDown={onTreeKey} className="min-h-0 flex-1 overflow-auto px-1 pb-3">
        {matches ? (
          matches.length ? (
            matches.map((n) => <Row key={n.path.join("/")} connId={connId} node={n} depth={0} flat />)
          ) : (
            <p className="px-3 py-6 text-center text-[12px] text-fg-subtle">No loaded items match “{filter}”. Expand a database to search inside it.</p>
          )
        ) : (
          <Children connId={connId} path={[]} depth={0} entry={root} />
        )}
      </div>
    </div>
  );
}

function Children({ connId, path, depth, entry }: { connId: string; path: string[]; depth: number; entry?: { nodes?: TreeNode[]; loading?: boolean; error?: string } }) {
  const loadChildren = useStore((s) => s.loadChildren);
  const pad = { paddingLeft: 8 + depth * 14 };
  if (!entry || entry.loading)
    return (
      <div className="flex h-6 items-center gap-2 text-[12px] text-fg-subtle" style={pad}>
        <Spinner className="size-3" /> Loading…
      </div>
    );
  if (entry.error)
    return (
      <div className="flex items-start gap-1.5 py-1 pr-2 text-[12px] text-danger" style={pad}>
        <Warning aria-hidden className="mt-0.5 size-3.5 shrink-0" />
        <span className="min-w-0 break-words">
          {entry.error}{" "}
          <button className="cursor-pointer underline underline-offset-2" onClick={() => loadChildren(connId, path, true)}>
            Retry
          </button>
        </span>
      </div>
    );
  if (!entry.nodes?.length)
    return (
      <div className="h-6 text-[12px] italic leading-6 text-fg-subtle" style={pad}>
        Empty
      </div>
    );
  return (
    <div role="group">
      {entry.nodes.map((n) => (
        <Row key={n.path.join("/")} connId={connId} node={n} depth={depth} />
      ))}
    </div>
  );
}

const Row = memo(function Row({ connId, node, depth, flat }: { connId: string; node: TreeNode; depth: number; flat?: boolean }) {
  const key = treeKey(connId, node.path);
  const open = useStore((s) => !!s.expanded[key]);
  const entry = useStore((s) => s.tree[key]);
  const active = useStore((s) => {
    const t = s.tabs.find((t) => t.id === s.activeTab);
    return t?.kind === "table" && t.connId === connId && t.path?.join("/") === node.path.join("/");
  });
  const { toggle, openTable, newQuery, openBackup } = useStore.getState();
  const canBackup = useStore((s) => {
    const c = s.connections.find((c) => c.id === connId);
    return !!s.drivers.find((d) => d.id === c?.driver)?.capabilities?.includes("backup");
  });
  const Icon = icons[node.kind] ?? Table;
  const expandable = node.hasChildren && !flat;

  const activate = () => {
    if (node.browsable) openTable(connId, node);
    else if (expandable) toggle(connId, node);
  };

  const onKeyDown = (e: KeyboardEvent) => {
    if (e.key === "Enter" || e.key === " ") {
      e.preventDefault();
      activate();
    } else if (e.key === "ArrowRight" && expandable && !open) {
      e.preventDefault();
      toggle(connId, node);
    } else if (e.key === "ArrowLeft" && expandable && open) {
      e.preventDefault();
      toggle(connId, node);
    }
  };

  return (
    <>
      <div
        role="treeitem"
        aria-label={flat ? node.path.join(" › ") : node.name}
        aria-expanded={expandable ? open : undefined}
        aria-selected={active}
        tabIndex={0}
        onClick={activate}
        onKeyDown={onKeyDown}

        className={clsx(
          "group flex h-[26px] cursor-pointer items-center gap-1.5 rounded-md pr-1 text-[13px] transition-colors duration-100 focus-visible:outline-offset-[-2px]",
          active ? "bg-brand-soft text-fg" : "text-fg/90 hover:bg-bg-2",
        )}
        style={{ paddingLeft: 4 + depth * 14 }}
      >
        <span className="flex size-4 shrink-0 items-center justify-center text-fg-subtle">
          {expandable && <CaretRight aria-hidden weight="bold" className={clsx("size-3 transition-transform duration-150", open && "rotate-90")} />}
        </span>
        <Icon aria-hidden weight={node.kind === "database" ? "fill" : "regular"} className={clsx("size-[15px] shrink-0", iconTint[node.kind])} />
        <span className="min-w-0 flex-1 truncate">{node.name}</span>
        {flat && node.path.length > 1 && <span className="truncate text-[11px] text-fg-subtle">{node.path.slice(0, -1).join(".")}</span>}
        {node.detail && !flat && <span className="shrink-0 font-mono text-[11px] text-fg-subtle group-hover:hidden">{node.detail}</span>}
        {node.kind === "database" && canBackup && (
          <button
            aria-label={`Back up ${node.name}`}
            onClick={(e) => {
              e.stopPropagation();
              openBackup(connId, "backup", node.path[0]);
            }}
            className="hidden size-5 shrink-0 cursor-pointer items-center justify-center rounded text-fg-muted hover:bg-bg-3 hover:text-fg group-hover:flex group-focus-within:flex"
          >
            <CloudArrowDown className="size-3" weight="bold" />
          </button>
        )}
        {node.kind === "database" && (
          <button
            aria-label={`New query in ${node.name}`}
            onClick={(e) => {
              e.stopPropagation();
              newQuery(connId, node.path[0]);
            }}
            className="hidden size-5 shrink-0 cursor-pointer items-center justify-center rounded text-fg-muted hover:bg-bg-3 hover:text-fg group-hover:flex group-focus-within:flex"
          >
            <Plus className="size-3" weight="bold" />
          </button>
        )}
      </div>
      {expandable && open && <Children connId={connId} path={node.path} depth={depth + 1} entry={entry} />}
    </>
  );
});
