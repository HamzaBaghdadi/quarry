"use client";

import clsx from "clsx";
import { useEffect, useMemo, useState } from "react";
import { CheckCircle, CloudArrowDown, CloudArrowUp, File, Warning, WarningOctagon, X } from "@phosphor-icons/react";
import { toast } from "sonner";
import { api, newRunId, onProgress, type BackupReport } from "@/lib/api";
import { treeKey, useStore } from "@/lib/store";
import { Button, EngineBadge, IconButton, Label, Select, Spinner } from "./common";
import { Dialog, DialogContent, DialogTitle } from "@/components/ui/dialog";

const fmtBytes = (n: number) =>
  n < 1024 ? `${n} B` : n < 1 << 20 ? `${(n / 1024).toFixed(1)} KB` : n < 1 << 30 ? `${(n / (1 << 20)).toFixed(1)} MB` : `${(n / (1 << 30)).toFixed(2)} GB`;

const fmtMs = (ms: number) => (ms < 1000 ? `${ms} ms` : ms < 60000 ? `${(ms / 1000).toFixed(1)} s` : `${Math.floor(ms / 60000)}m ${Math.round((ms % 60000) / 1000)}s`);

export function BackupDialog() {
  const job = useStore((s) => s.backup);
  if (!job) return null;
  return <Body key={job.connId + job.mode} />;
}

type Phase = { name: "idle" } | { name: "running"; message: string; done: number; total: number } | { name: "done"; report: BackupReport } | { name: "error"; message: string };

function Body() {
  const job = useStore((s) => s.backup)!;
  const close = useStore((s) => s.closeBackup);
  const conn = useStore((s) => s.connections.find((c) => c.id === job.connId));
  const drivers = useStore((s) => s.drivers);
  const rootNodes = useStore((s) => s.tree[treeKey(job.connId, [])]?.nodes);
  const loadChildren = useStore((s) => s.loadChildren);
  const drv = drivers.find((d) => d.id === conn?.driver);
  const restore = job.mode === "restore";

  const scopes = useMemo(() => (rootNodes ?? []).filter((n) => n.kind === "database").map((n) => n.name), [rootNodes]);
  const [scope, setScope] = useState(job.scope || conn?.config.database || scopes[0] || "");
  const [file, setFile] = useState("");
  const [phase, setPhase] = useState<Phase>({ name: "idle" });
  const [runId] = useState(newRunId);
  const running = phase.name === "running";

  useEffect(() => {
    if (!scope && scopes.length) setScope(scopes[0]);
  }, [scopes, scope]);

  useEffect(
    () =>
      onProgress((e) => {
        if (e.runId === runId) setPhase({ name: "running", message: e.message, done: e.done, total: e.total });
      }),
    [runId],
  );


  const isSqlite = conn?.driver === "sqlite";
  const format = drv?.family === "sql" ? "SQL script (.sql)" : "Quarry archive (.ndjson)";

  const pick = async () => {
    try {
      const f = await api.pickBackupFile(job.connId);
      if (f) setFile(f);
    } catch (e) {
      toast.error((e as Error).message);
    }
  };

  const start = async () => {
    setPhase({ name: "running", message: restore ? "Starting restore…" : "Starting backup…", done: 0, total: 0 });
    try {
      const report = restore ? await api.restore(job.connId, scope, file, runId) : await api.backup(job.connId, scope, runId);
      if (!report.path) {
        setPhase({ name: "idle" }); // save dialog cancelled
        return;
      }
      setPhase({ name: "done", report });
      if (restore) {
        // Everything may have changed; reload the explorer.
        useStore.setState((s) => ({
          tree: Object.fromEntries(Object.entries(s.tree).filter(([k]) => !k.startsWith(job.connId + "\u0000") || k === treeKey(job.connId, []))),
        }));
        void loadChildren(job.connId, [], true);
      }
    } catch (e) {
      setPhase({ name: "error", message: (e as Error).message });
    }
  };

  const pct = phase.name === "running" && phase.total > 0 ? Math.min(100, (phase.done / phase.total) * 100) : null;

  return (
    <Dialog open onOpenChange={(o) => !o && !running && close()}>
      <DialogContent
        showCloseButton={false}
        aria-describedby={undefined}
        onInteractOutside={(e) => running && e.preventDefault()}
        className="w-full max-w-[520px] gap-0 overflow-hidden rounded-xl border-line-strong bg-bg-1 p-0 shadow-pop sm:max-w-[520px]"
      >
        <header className="flex h-12 items-center gap-2.5 border-b border-line px-4">
          {restore ? <CloudArrowUp aria-hidden className="size-5 text-brand-text" /> : <CloudArrowDown aria-hidden className="size-5 text-brand-text" />}
          <DialogTitle className="text-[14px] font-semibold">
            {restore ? "Restore backup" : "Back up database"}
          </DialogTitle>
          <IconButton label="Close" className="ml-auto" onClick={close} disabled={running}>
            <X className="size-4" />
          </IconButton>
        </header>

        <div className="space-y-4 p-4">
          <div className="flex items-center gap-3 rounded-lg border border-line bg-bg-0 p-3">
            <EngineBadge driver={conn?.driver} color={conn?.color} size={32} />
            <div className="min-w-0">
              <div className="truncate text-[13px] font-medium">{conn?.name}</div>
              <div className="text-[11.5px] text-fg-subtle">
                {drv?.name} · {format}
              </div>
            </div>
          </div>

          {!isSqlite && scopes.length > 0 && (
            <div>
              <Label htmlFor="bk-scope">{restore ? "Restore into database" : "Database to back up"}</Label>
              <Select
                id="bk-scope"
                value={scope}
                onChange={setScope}
                disabled={running || phase.name === "done"}
                options={scopes.map((s) => ({ value: s, label: s }))}
              />
            </div>
          )}

          {restore && phase.name !== "done" && (
            <>
              <div>
                <Label>Backup file</Label>
                <div className="flex gap-2">
                  <div className={clsx("flex h-8 min-w-0 flex-1 items-center gap-2 rounded-md border border-line bg-bg-0 px-2.5 font-mono text-[12px]", !file && "text-fg-subtle")}>
                    <File aria-hidden className="size-3.5 shrink-0" />
                    <span className="truncate">
                      {file || "No file chosen"}
                    </span>
                  </div>
                  <Button onClick={pick} disabled={running}>
                    Choose…
                  </Button>
                </div>
              </div>
              <div className="flex gap-2.5 rounded-lg border border-warn/35 bg-warn/10 p-3 text-[12.5px]">
                <Warning aria-hidden weight="fill" className="mt-0.5 size-4 shrink-0 text-warn" />
                <p>
                  Tables, collections or keys in the backup <strong>replace</strong> existing ones with the same name
                  {scope ? (
                    <>
                      {" "}
                      in <span className="font-mono">{scope}</span>
                    </>
                  ) : null}
                  . Other objects are left alone.
                </p>
              </div>
            </>
          )}

          {!restore && phase.name === "idle" && (
            <p className="text-[12.5px] leading-relaxed text-fg-muted">
              {drv?.family === "sql"
                ? "Writes a portable SQL script with the schema (tables, keys, indexes, views) and all rows, read from a consistent snapshot."
                : drv?.family === "document"
                  ? "Exports every collection, its documents (types preserved as Extended JSON) and its indexes."
                  : "Exports every key with its TTL using Redis DUMP, restorable on the same or a newer server version."}{" "}
              You’ll choose where to save it next.
            </p>
          )}

          {phase.name === "running" && (
            <div aria-live="polite" className="space-y-2">
              <div className="flex items-center gap-2 text-[12.5px]">
                <Spinner className="text-brand-text" />
                <span className="min-w-0 flex-1 truncate">{phase.message}</span>
                <span className="font-mono text-[11.5px] text-fg-subtle">
                  {fmtBytes(phase.done)}
                  {phase.total ? ` / ${fmtBytes(phase.total)}` : ""}
                </span>
              </div>
              <div className="h-1.5 overflow-hidden rounded-full bg-bg-3" role="progressbar" aria-valuenow={pct ?? undefined} aria-valuemin={0} aria-valuemax={100}>
                <div
                  className={clsx("h-full rounded-full bg-brand transition-[width] duration-200", pct === null && "w-1/3 animate-[indeterminate_1.2s_ease-in-out_infinite]")}
                  style={pct !== null ? { width: `${pct}%` } : undefined}
                />
              </div>
            </div>
          )}

          {phase.name === "done" && (
            <div className="rounded-lg border border-run/30 bg-run/10 p-3">
              <div className="flex items-center gap-2 text-[13px] font-semibold">
                <CheckCircle aria-hidden weight="fill" className="size-4 text-run" />
                {restore ? "Restore complete" : "Backup complete"}
              </div>
              <dl className="mt-2 grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-[12px]">
                <dt className="text-fg-muted">{drv?.family === "keyvalue" ? "Keys" : drv?.family === "document" ? "Collections" : "Tables"}</dt>
                <dd className="tabular-nums">{phase.report.stats.objects.toLocaleString()}</dd>
                {drv?.family !== "keyvalue" && (
                  <>
                    <dt className="text-fg-muted">{drv?.family === "document" ? "Documents" : "Rows"}</dt>
                    <dd className="tabular-nums">{phase.report.stats.rows.toLocaleString()}</dd>
                  </>
                )}
                <dt className="text-fg-muted">Size</dt>
                <dd className="tabular-nums">{fmtBytes(phase.report.bytes)}</dd>
                <dt className="text-fg-muted">Time</dt>
                <dd className="tabular-nums">{fmtMs(phase.report.durationMs)}</dd>
                <dt className="text-fg-muted">File</dt>
                <dd className="selectable break-all font-mono text-[11.5px]">{phase.report.path}</dd>
              </dl>
            </div>
          )}

          {phase.name === "error" && (
            <div role="alert" className="flex gap-2.5 rounded-lg border border-danger/30 bg-danger-soft p-3">
              <WarningOctagon aria-hidden weight="fill" className="mt-0.5 size-4 shrink-0 text-danger" />
              <div className="min-w-0">
                <p className="text-[13px] font-semibold text-danger">{restore ? "Restore failed" : "Backup failed"}</p>
                <pre className="selectable mt-1 max-h-40 overflow-auto whitespace-pre-wrap break-words font-mono text-[11.5px]">{phase.message}</pre>
              </div>
            </div>
          )}
        </div>

        <footer className="flex h-14 items-center justify-end gap-2 border-t border-line bg-bg-0 px-4">
          {running ? (
            <Button variant="danger" onClick={() => api.cancel(runId)}>
              Cancel
            </Button>
          ) : phase.name === "done" ? (
            <Button variant="primary" onClick={close}>
              Done
            </Button>
          ) : (
            <>
              <Button variant="ghost" onClick={close}>
                Close
              </Button>
              <Button variant={restore ? "danger" : "primary"} className={restore ? "border border-danger/40" : ""} onClick={start} disabled={restore && !file}>
                {restore ? "Restore now" : "Choose location & back up"}
              </Button>
            </>
          )}
        </footer>
      </DialogContent>
    </Dialog>
  );
}
