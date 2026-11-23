"use client";

import clsx from "clsx";
import { useEffect, useState } from "react";
import {
  CloudArrowDown,
  CloudArrowUp,
  Code,
  Command,
  Copy,
  Lightning,
  LockSimple,
  Moon,
  PencilSimple,
  Plug,
  Plus,
  PlusCircle,
  Sun,
  Table,
  Trash,
  UsersThree,
  X,
} from "@phosphor-icons/react";
import { Toaster, toast } from "sonner";
import { useStore } from "@/lib/store";
import { isDesktop } from "@/lib/api";
import { BackupDialog } from "./BackupDialog";
import { CommandPalette } from "./CommandPalette";
import { ConnectionDialog } from "./ConnectionDialog";
import { Explorer } from "./Explorer";
import { QueryTab } from "./QueryTab";
import { RolesTab } from "./RolesTab";
import { DatabaseDialogs } from "./DatabaseDialogs";
import { Split } from "./Split";
import { TableTab } from "./TableTab";
import { Button, ConfirmHost, EngineBadge, IconButton, Kbd, Spinner, Tip, confirmDialog, mod } from "./common";
import { ConnectionMenu, deleteConnection } from "./ConnectionMenu";
import { SettingsMenu } from "./SettingsMenu";
import { TitleBar } from "./TitleBar";
import { UpdateDialog } from "./UpdateDialog";
import { startUpdateChecks } from "@/lib/updater";
import { TooltipProvider } from "@/components/ui/tooltip";
import { Textarea } from "@/components/ui/textarea";

export default function Workbench() {
  const init = useStore((s) => s.init);
  const ready = useStore((s) => s.ready);
  const theme = useStore((s) => s.theme);

  useEffect(() => {
    void init();
  }, [init]);

  // Global shortcuts.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const s = useStore.getState();
      const m = e.ctrlKey || e.metaKey;
      if (m && e.key.toLowerCase() === "k") {
        e.preventDefault();
        s.setPalette(!s.palette);
      } else if (m && e.key.toLowerCase() === "t") {
        e.preventDefault();
        s.newQuery();
      } else if (m && e.key.toLowerCase() === "w" && s.activeTab) {
        e.preventDefault();
        s.closeTab(s.activeTab);
      } else if (m && e.key.toLowerCase() === "n") {
        e.preventDefault();
        s.openDialog();
      } else if (e.ctrlKey && e.key === "Tab" && s.tabs.length > 1) {
        e.preventDefault();
        const i = s.tabs.findIndex((t) => t.id === s.activeTab);
        const next = (i + (e.shiftKey ? -1 : 1) + s.tabs.length) % s.tabs.length;
        s.setActiveTab(s.tabs[next].id);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  useEffect(() => startUpdateChecks(), []);

  return (
    <TooltipProvider delayDuration={350} skipDelayDuration={150}>
    <div className="flex h-screen flex-col bg-bg-0 text-fg">
      <TitleBar />
      <div className="flex min-h-0 flex-1">
        <Rail />
        {ready ? (
          <Split direction="horizontal" initial={272} min={200} max={520} storageKey="sidebar" className="min-w-0 flex-1" first={<Sidebar />} second={<Main />} />
        ) : (
          <div className="flex flex-1 items-center justify-center">
            <Spinner className="text-brand-text" />
          </div>
        )}
      </div>
      <StatusBar />
      <ConfirmHost />
      <ConnectionDialog />
      <BackupDialog />
      <DatabaseDialogs />
      <CommandPalette />
      <UpdateDialog />
      <Toaster theme={theme} position="bottom-right" toastOptions={{ className: "!bg-popover !border-line-strong !text-fg !font-sans !shadow-pop" }} />
    </div>
    </TooltipProvider>
  );
}

/* ------------------------------------------------------------------ Rail */

function Rail() {
  const connections = useStore((s) => s.connections);
  const activeId = useStore((s) => s.activeId);
  const connecting = useStore((s) => s.connecting);
  const { connect, openDialog } = useStore.getState();

  return (
    <nav aria-label="Connections" className="flex w-[52px] shrink-0 flex-col items-center gap-1.5 border-r border-line bg-bg-0 py-2.5">
      <div className="flex min-h-0 flex-1 flex-col items-center gap-1.5 overflow-y-auto px-1">
        {connections.map((c) => (
          <ConnectionMenu key={c.id} conn={c}>
          <div>
          <Tip key={c.id} side="right" label={<span>{c.name}{c.connected && <span className="ml-1.5 text-run">● connected</span>}</span>}>
          <button
            onClick={() => connect(c.id)}
            aria-label={`${c.name}${c.connected ? " (connected)" : ""}`}
            aria-current={activeId === c.id ? "true" : undefined}
            className={clsx(
              "relative flex size-9 shrink-0 cursor-pointer items-center justify-center rounded-lg transition-[background-color,box-shadow] duration-150",
              activeId === c.id ? "bg-bg-3 shadow-[inset_0_0_0_1px_var(--line-strong)]" : "hover:bg-bg-2",
            )}
          >
            {activeId === c.id && <span aria-hidden className="absolute -left-1 h-4 w-[3px] rounded-r bg-brand" />}
            <EngineBadge driver={c.driver} color={c.color} size={26} />
            {connecting === c.id ? (
              <Spinner className="absolute -bottom-0.5 -right-0.5 size-2.5 text-brand-text" />
            ) : (
              c.connected && <span aria-hidden className="absolute bottom-0.5 right-0.5 size-2 rounded-full border-2 border-bg-0 bg-run" />
            )}
          </button>
          </Tip>
          </div>
          </ConnectionMenu>
        ))}
        <IconButton label={`New connection (${mod()}+N)`} className="size-9 rounded-lg border border-dashed border-line-strong" onClick={() => openDialog()}>
          <Plus className="size-4" />
        </IconButton>
      </div>
      <SettingsMenu />
    </nav>
  );
}

function Logo() {
  // A faceted stone: layered data, dug out of a quarry.
  return (
    <svg viewBox="2 1 28 30" className="size-7" aria-hidden>
      <path d="M16 3 28 10 16 17 4 10Z" fill="#60a5fa" />
      <path d="M16 17v12L4 22V10Z" fill="#2563eb" />
      <path d="M16 17 28 10v12L16 29Z" fill="#1d4ed8" />
      <path d="M4 14.1 16 21.1 28 14.1M4 18.1 16 25.1 28 18.1" stroke="var(--bg-0)" strokeWidth="1.1" fill="none" />
    </svg>
  );
}

/* --------------------------------------------------------------- Sidebar */

function Sidebar() {
  const activeId = useStore((s) => s.activeId);
  const conn = useStore((s) => s.connections.find((c) => c.id === s.activeId));
  const drivers = useStore((s) => s.drivers);
  const { newQuery, disconnect, openDialog, removeConnection, openBackup } = useStore.getState();

  if (!conn || !conn.connected || !activeId) return <SavedConnections />;
  const drv = drivers.find((d) => d.id === conn.driver);

  return (
    <aside aria-label="Explorer" className="flex min-h-0 flex-1 flex-col bg-bg-0">
      <header className="drag flex h-12 shrink-0 items-center gap-2.5 px-3">
        <EngineBadge driver={conn.driver} color={conn.color} size={26} />
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-1 truncate text-[13px] font-semibold">
            <span className="truncate">{conn.name}</span>
            {conn.readOnly && <LockSimple aria-label="Read-only" weight="fill" className="size-3 shrink-0 text-warn" />}
          </div>
          <div className="truncate text-[11px] text-fg-subtle">
            {drv?.name} · {conn.config.uri ? "URL" : conn.config.file || `${conn.config.host}${conn.config.port ? ":" + conn.config.port : ""}`}
          </div>
        </div>
      </header>
      <div className="no-drag flex items-center gap-1 px-2 pb-2">
        <Button size="sm" variant="subtle" className="flex-1" onClick={() => newQuery(activeId)}>
          <Code aria-hidden className="size-3.5" /> New query
        </Button>
        {drv?.capabilities?.includes("backup") && (
          <>
            <IconButton label="Back up…" onClick={() => openBackup(activeId, "backup")}>
              <CloudArrowDown className="size-3.5" />
            </IconButton>
            {!conn.readOnly && (
              <IconButton label="Restore…" onClick={() => openBackup(activeId, "restore")}>
                <CloudArrowUp className="size-3.5" />
              </IconButton>
            )}
          </>
        )}
        <IconButton label="Edit connection" onClick={() => openDialog(conn)}>
          <PencilSimple className="size-3.5" />
        </IconButton>
        <IconButton label="Disconnect" onClick={() => disconnect(activeId)}>
          <Plug className="size-3.5" />
        </IconButton>
        <IconButton
          label="Delete connection"
          className="hover:!text-danger"
          onClick={() => {
            void deleteConnection(conn);
          }}
        >
          <Trash className="size-3.5" />
        </IconButton>
      </div>
      <Explorer key={activeId} connId={activeId} />
    </aside>
  );
}

function SavedConnections() {
  const connections = useStore((s) => s.connections);
  const drivers = useStore((s) => s.drivers);
  const connecting = useStore((s) => s.connecting);
  const { connect, openDialog } = useStore.getState();

  return (
    <aside aria-label="Saved connections" className="flex min-h-0 flex-1 flex-col bg-bg-0">
      <header className="drag flex h-12 shrink-0 items-center justify-between px-3">
        <h2 className="text-[11px] font-semibold uppercase tracking-wider text-fg-subtle">Connections</h2>
        <IconButton label="New connection" className="no-drag" onClick={() => openDialog()}>
          <Plus className="size-4" />
        </IconButton>
      </header>
      <ul className="min-h-0 flex-1 space-y-0.5 overflow-auto px-1.5">
        {connections.map((c) => (
          <ConnectionMenu key={c.id} conn={c}>
          <li className="group relative">
            <button
              onClick={() => connect(c.id)}
              className="flex w-full cursor-pointer items-center gap-2.5 rounded-md px-2 py-1.5 text-left transition-colors duration-100 hover:bg-bg-2"
            >
              <EngineBadge driver={c.driver} color={c.color} size={26} />
              <span className="min-w-0 flex-1">
                <span className="block truncate text-[13px] font-medium">{c.name}</span>
                <span className="block truncate text-[11px] text-fg-subtle">{drivers.find((d) => d.id === c.driver)?.name}</span>
              </span>
              {connecting === c.id && <Spinner className="text-brand-text" />}
            </button>
            <div className="absolute right-1.5 top-1/2 hidden -translate-y-1/2 items-center gap-0.5 rounded-md bg-bg-2 group-hover:flex group-focus-within:flex">
              <IconButton label={`Edit ${c.name}`} className="size-6" onClick={() => openDialog(c)}>
                <PencilSimple className="size-3.5" />
              </IconButton>
              <IconButton label={`Delete ${c.name}`} className="size-6 hover:!text-danger" onClick={() => void deleteConnection(c)}>
                <Trash className="size-3.5" />
              </IconButton>
            </div>
          </li>
          </ConnectionMenu>
        ))}
        {!connections.length && <p className="px-2 py-4 text-[12px] text-fg-subtle">No saved connections yet.</p>}
      </ul>
    </aside>
  );
}

/* ------------------------------------------------------------------ Main */

function Main() {
  const tabs = useStore((s) => s.tabs);
  const activeTab = useStore((s) => s.activeTab);
  const inspected = useStore((s) => s.inspected);

  return (
    <main className="flex min-h-0 min-w-0 flex-1 bg-bg-1">
      <div className="flex min-h-0 min-w-0 flex-1 flex-col">
        {tabs.length ? (
          <>
            <TabBar />
            {tabs.map((t) => (
              <div key={t.id} className={clsx("min-h-0 flex-1 flex-col", t.id === activeTab ? "flex" : "hidden")}>
                {t.kind === "query" ? <QueryTab tab={t} /> : t.kind === "roles" ? <RolesTab tab={t} /> : <TableTab tab={t} />}
              </div>
            ))}
          </>
        ) : (
          <Welcome />
        )}
      </div>
      {inspected && <Inspector />}
    </main>
  );
}

function TabBar() {
  const tabs = useStore((s) => s.tabs);
  const activeTab = useStore((s) => s.activeTab);
  const connections = useStore((s) => s.connections);
  const { setActiveTab, closeTab, newQuery } = useStore.getState();

  return (
    <div role="tablist" aria-label="Open tabs" className="drag flex h-10 shrink-0 items-end gap-px overflow-x-auto border-b border-line bg-bg-0 pl-1 pr-2">
      {tabs.map((t) => {
        const conn = connections.find((c) => c.id === t.connId);
        const active = t.id === activeTab;
        return (
          <div
            key={t.id}
            role="tab"
            aria-selected={active}
            tabIndex={active ? 0 : -1}
            onClick={() => setActiveTab(t.id)}
            onMouseDown={(e) => e.button === 1 && (e.preventDefault(), closeTab(t.id))}
            onKeyDown={(e) => (e.key === "Enter" || e.key === " ") && setActiveTab(t.id)}
            className={clsx(
              "no-drag group relative flex h-[34px] min-w-0 max-w-[220px] shrink-0 cursor-pointer items-center gap-2 rounded-t-md pl-2.5 pr-1 text-[12.5px] transition-colors duration-100",
              active ? "bg-bg-1 text-fg shadow-[inset_0_1px_0_var(--line-strong),inset_1px_0_0_var(--line),inset_-1px_0_0_var(--line)]" : "text-fg-muted hover:bg-bg-2 hover:text-fg",
            )}
          >
            {active && <span aria-hidden className="absolute inset-x-0 top-0 h-[2px] rounded-t" style={{ background: conn?.color }} />}
            <span aria-hidden className="size-1.5 shrink-0 rounded-full" style={{ background: conn?.color }} />
            {t.kind === "query" ? <Code aria-hidden className="size-3.5 shrink-0" /> : t.kind === "roles" ? <UsersThree aria-hidden className="size-3.5 shrink-0" /> : <Table aria-hidden className="size-3.5 shrink-0" />}
            <span className="truncate">{t.title}</span>
            <button
              aria-label={`Close ${t.title}`}
              onClick={(e) => {
                e.stopPropagation();
                closeTab(t.id);
              }}
              className={clsx("flex size-5 shrink-0 cursor-pointer items-center justify-center rounded hover:bg-bg-3", active ? "opacity-100" : "opacity-0 group-hover:opacity-100 focus:opacity-100")}
            >
              <X className="size-3" />
            </button>
          </div>
        );
      })}
      <IconButton label={`New query (${mod()}+T)`} className="no-drag mb-1 ml-1" onClick={() => newQuery()}>
        <Plus className="size-3.5" />
      </IconButton>
    </div>
  );
}

function Welcome() {
  const connections = useStore((s) => s.connections);
  const connecting = useStore((s) => s.connecting);
  const drivers = useStore((s) => s.drivers);
  const { connect, openDialog, setPalette } = useStore.getState();

  return (
    <div className="drag flex min-h-0 flex-1 items-center justify-center overflow-auto p-8">
      <div className="no-drag w-full max-w-[560px]">
        <div className="mb-8 flex items-center gap-3">
          <div className="scale-150">
            <Logo />
          </div>
          <div className="ml-2">
            <h1 className="text-[22px] font-semibold tracking-tight">Quarry</h1>
            <p className="text-[13px] text-fg-muted">One small client for {drivers.length} database engines.</p>
          </div>
        </div>

        {connections.length > 0 && (
          <section className="mb-6">
            <h2 className="mb-2 text-[11px] font-semibold uppercase tracking-wider text-fg-subtle">Recent connections</h2>
            <div className="grid gap-2 sm:grid-cols-2">
              {connections.slice(0, 6).map((c) => (
                <button
                  key={c.id}
                  onClick={() => connect(c.id)}
                  className="flex cursor-pointer items-center gap-3 rounded-lg border border-line bg-bg-0 p-3 text-left transition-colors duration-150 hover:border-line-strong hover:bg-bg-2"
                >
                  <EngineBadge driver={c.driver} color={c.color} size={32} />
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-[13px] font-medium">{c.name}</span>
                    <span className="block truncate font-mono text-[11px] text-fg-subtle">
                      {c.config.file || (c.config.uri ? "connection URL" : `${c.config.host}:${c.config.port}`)}
                    </span>
                  </span>
                  {connecting === c.id ? <Spinner className="text-brand-text" /> : <Lightning aria-hidden className="size-4 text-fg-subtle" />}
                </button>
              ))}
            </div>
          </section>
        )}

        <div className="flex flex-wrap gap-2">
          <Button variant="primary" onClick={() => openDialog()}>
            <PlusCircle aria-hidden className="size-4" /> New connection
          </Button>
          <Button variant="ghost" onClick={() => setPalette(true)}>
            <Command aria-hidden className="size-4" /> Command palette <Kbd>{mod()}</Kbd>
            <Kbd>K</Kbd>
          </Button>
        </div>

        <dl className="mt-8 grid grid-cols-2 gap-x-6 gap-y-1.5 text-[12px] text-fg-muted">
          {[
            ["Run query", `${mod()} Enter`],
            ["New query tab", `${mod()} T`],
            ["Close tab", `${mod()} W`],
            ["Next tab", "Ctrl Tab"],
            ["Copy cell / row", `${mod()} C / ⇧`],
            ["Inspect cell", "Enter"],
          ].map(([k, v]) => (
            <div key={k} className="flex justify-between border-b border-line py-1">
              <dt>{k}</dt>
              <dd className="font-mono text-fg-subtle">{v}</dd>
            </div>
          ))}
        </dl>
      </div>
    </div>
  );
}

/* ------------------------------------------------------------- Inspector */

function Inspector() {
  const inspected = useStore((s) => s.inspected)!;
  return <InspectorBody key={`${inspected.column}:${String(inspected.value)}`} />;
}

function InspectorBody() {
  const inspected = useStore((s) => s.inspected)!;
  const inspect = useStore((s) => s.inspect);
  const { value, onSave } = inspected;
  const isJson = typeof value === "string" && /^[[{]/.test(value.trim());

  let pretty = value === null || value === undefined ? "" : String(value);
  if (isJson) {
    try {
      pretty = JSON.stringify(JSON.parse(value as string), null, 2);
    } catch {}
  }
  const [draft, setDraft] = useState(pretty);
  const [jsonError, setJsonError] = useState<string | null>(null);
  const dirty = draft !== pretty;

  const apply = (text: string | null) => {
    if (text !== null && isJson) {
      try {
        text = JSON.stringify(JSON.parse(text)); // keep JSON compact & valid
      } catch (e) {
        setJsonError((e as Error).message);
        return;
      }
    }
    onSave?.(text);
    inspect(null);
    toast("Change staged — save the table to write it");
  };

  return (
    <aside aria-label="Value inspector" className="animate-pop flex w-[360px] shrink-0 flex-col border-l border-line bg-bg-0">
      <header className="flex h-10 shrink-0 items-center gap-2 border-b border-line px-3">
        <span className="min-w-0 flex-1 truncate font-mono text-[12px] font-semibold">{inspected.column}</span>
        {inspected.type && <span className="font-mono text-[10.5px] uppercase text-fg-subtle">{inspected.type}</span>}
        <IconButton label="Copy value" onClick={() => navigator.clipboard.writeText(draft).then(() => toast.success("Copied value"))}>
          <Copy className="size-3.5" />
        </IconButton>
        <IconButton label="Close inspector" onClick={() => inspect(null)}>
          <X className="size-3.5" />
        </IconButton>
      </header>
      {onSave ? (
        <Textarea
          value={draft}
          onChange={(e) => {
            setDraft(e.target.value);
            setJsonError(null);
          }}
          onKeyDown={(e) => {
            if ((e.ctrlKey || e.metaKey) && e.key === "Enter") apply(draft);
          }}
          placeholder={value === null ? "NULL" : ""}
          aria-label={`Edit ${inspected.column}`}
          spellCheck={false}
          className="min-h-0 flex-1 resize-none rounded-none border-0 bg-bg-0 p-3 font-mono text-[12.5px] leading-relaxed text-fg shadow-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-brand dark:bg-bg-0"
        />
      ) : (
        <pre className="selectable min-h-0 flex-1 overflow-auto whitespace-pre-wrap break-all p-3 font-mono text-[12.5px] leading-relaxed text-fg">
          {value === null || value === undefined ? <span className="italic text-fg-subtle">NULL</span> : pretty}
        </pre>
      )}
      {jsonError && (
        <p role="alert" className="border-t border-danger/30 bg-danger-soft px-3 py-1.5 text-[11.5px] text-danger">
          Invalid JSON: {jsonError}
        </p>
      )}
      <footer className="flex min-h-9 items-center gap-2 border-t border-line px-3 py-1.5 text-[11px] text-fg-subtle">
        <span>{typeof value === "string" ? `${value.length.toLocaleString()} chars` : value === null ? "null" : typeof value}</span>
        {onSave && (
          <div className="ml-auto flex gap-1.5">
            <Button size="sm" variant="ghost" onClick={() => apply(null)}>
              Set NULL
            </Button>
            <Button size="sm" variant="primary" disabled={!dirty} onClick={() => apply(draft)}>
              Apply
            </Button>
          </div>
        )}
      </footer>
    </aside>
  );
}

/* ------------------------------------------------------------- StatusBar */

function StatusBar() {
  const conn = useStore((s) => s.connections.find((c) => c.id === s.activeId));
  const drivers = useStore((s) => s.drivers);
  const open = useStore((s) => s.connections.filter((c) => c.connected).length);

  return (
    <footer className="flex h-6 shrink-0 items-center gap-3 border-t border-line bg-bg-0 px-3 text-[11px] text-fg-subtle">
      {conn?.connected ? (
        <span className="flex items-center gap-1.5">
          <span aria-hidden className="size-1.5 rounded-full bg-run" />
          {conn.name} · {drivers.find((d) => d.id === conn.driver)?.name}
        </span>
      ) : (
        <span>Not connected</span>
      )}
      <span>{open} open</span>
      {!isDesktop() && <span className="text-warn">Demo mode (browser) — data is simulated</span>}
      <span className="ml-auto">Quarry</span>
    </footer>
  );
}
