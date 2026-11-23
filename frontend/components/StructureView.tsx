"use client";

import { useCallback, useEffect, useState } from "react";
import CodeMirror from "@uiw/react-codemirror";
import { sql } from "@codemirror/lang-sql";
import { ArrowClockwise, Code, Copy, Key, WarningOctagon } from "@phosphor-icons/react";
import { toast } from "sonner";
import { api, type TableStructure } from "@/lib/api";
import { Button, IconButton, Spinner } from "./common";

function Section({ title, count, children }: { title: string; count?: number; children: React.ReactNode }) {
  return (
    <section>
      <h3 className="mb-2 flex items-center gap-2 text-[11px] font-semibold uppercase tracking-wider text-fg-subtle">
        {title}
        {count !== undefined && <span className="rounded bg-bg-3 px-1.5 text-[10.5px] font-medium normal-case tracking-normal text-fg-muted">{count}</span>}
      </h3>
      {children}
    </section>
  );
}

const th = "px-3 py-1.5 text-left text-[11.5px] font-semibold text-fg-muted";
const td = "px-3 py-1.5 align-top";

/** pgAdmin-style Properties/SQL view of a table or view. */
export function StructureView({ connId, path, onOpenQuery }: { connId: string; path: string[]; onOpenQuery: (sql: string) => void }) {
  const [st, setSt] = useState<TableStructure | null>(null);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    setError(null);
    try {
      setSt(await api.structure(connId, path));
    } catch (e) {
      setError((e as Error).message);
    }
  }, [connId, path]);

  useEffect(() => {
    void load();
  }, [load]);

  if (error)
    return (
      <div role="alert" className="m-3 flex gap-2.5 rounded-lg border border-danger/30 bg-danger-soft p-3 text-[13px]">
        <WarningOctagon aria-hidden weight="fill" className="mt-0.5 size-4 shrink-0 text-danger" />
        <span className="selectable">{error}</span>
      </div>
    );
  if (!st)
    return (
      <div className="flex flex-1 items-center justify-center">
        <Spinner className="text-brand-text" />
      </div>
    );

  const cols = st.columns ?? [];
  const idx = st.indexes ?? [];
  const cons = st.constraints ?? [];

  return (
    <div className="min-h-0 flex-1 overflow-auto bg-bg-1">
      <div className="mx-auto max-w-[1100px] space-y-6 p-5">
        <div className="flex flex-wrap items-center gap-x-5 gap-y-1 text-[12.5px] text-fg-muted">
          <span className="rounded bg-bg-3 px-2 py-0.5 text-[11px] font-semibold uppercase tracking-wide text-fg">{st.kind}</span>
          <span>
            <strong className="font-semibold text-fg">{cols.length}</strong> columns
          </span>
          {st.rowEstimate >= 0 && (
            <span>
              ~<strong className="font-semibold text-fg">{st.rowEstimate.toLocaleString()}</strong> rows
            </span>
          )}
          {st.size && (
            <span>
              <strong className="font-semibold text-fg">{st.size}</strong> on disk
            </span>
          )}
          {st.comment && <span className="italic">“{st.comment}”</span>}
          <div className="ml-auto flex items-center gap-1">
            <IconButton label="Refresh" onClick={() => void load()}>
              <ArrowClockwise className="size-4" />
            </IconButton>
          </div>
        </div>

        <Section title="Columns" count={cols.length}>
          <div className="overflow-hidden rounded-lg border border-line">
            <table className="w-full border-collapse text-[13px]">
              <thead className="bg-bg-0">
                <tr className="border-b border-line">
                  <th className={th}>Name</th>
                  <th className={th}>Type</th>
                  <th className={th}>Nullable</th>
                  <th className={th}>Default</th>
                  <th className={th}>Comment</th>
                </tr>
              </thead>
              <tbody className="selectable">
                {cols.map((c) => (
                  <tr key={c.name} className="border-b border-line/70 last:border-0 hover:bg-bg-2/50">
                    <td className={td}>
                      <span className="flex items-center gap-1.5 font-mono text-[12.5px] font-medium">
                        {c.primaryKey && <Key aria-label="Primary key" weight="fill" className="size-3 shrink-0 text-warn" />}
                        {c.name}
                      </span>
                    </td>
                    <td className={`${td} font-mono text-[12.5px] text-[var(--num)]`}>{c.type}</td>
                    <td className={`${td} text-[12px]`}>{c.nullable ? <span className="text-fg-muted">yes</span> : <span className="font-medium text-fg">NOT NULL</span>}</td>
                    <td className={`${td} max-w-[280px] break-all font-mono text-[12px] text-fg-muted`}>{c.default || "—"}</td>
                    <td className={`${td} text-[12px] text-fg-muted`}>{c.comment || ""}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </Section>

        {st.kind === "table" && (
          <div className="grid gap-6 lg:grid-cols-2">
            <Section title="Constraints" count={cons.length}>
              <ItemList items={cons} empty="No constraints" />
            </Section>
            <Section title="Indexes" count={idx.length}>
              <ItemList items={idx} empty="No indexes" />
            </Section>
          </div>
        )}

        <Section title={st.kind === "view" ? "View definition" : "SQL"}>
          <div className="overflow-hidden rounded-lg border border-line">
            <div className="flex items-center justify-end gap-1 border-b border-line bg-bg-0 px-2 py-1">
              <Button
                size="sm"
                variant="ghost"
                className="h-6"
                onClick={() => navigator.clipboard.writeText(st.ddl).then(() => toast.success("Copied SQL"))}
              >
                <Copy aria-hidden className="size-3.5" /> Copy
              </Button>
              <Button size="sm" variant="ghost" className="h-6" onClick={() => onOpenQuery(st.ddl)}>
                <Code aria-hidden className="size-3.5" /> Open in query tab
              </Button>
            </div>
            <CodeMirror
              value={st.ddl}
              readOnly
              editable={false}
              theme="none"
              extensions={[sql()]}
              basicSetup={{ lineNumbers: true, foldGutter: false, highlightActiveLine: false, highlightActiveLineGutter: false }}
              className="text-[12.5px]"
              aria-label="Table SQL"
            />
          </div>
        </Section>
      </div>
    </div>
  );
}

function ItemList({ items, empty }: { items: { name: string; kind: string; definition: string }[]; empty: string }) {
  if (!items.length) return <p className="rounded-lg border border-dashed border-line px-3 py-4 text-center text-[12.5px] text-fg-subtle">{empty}</p>;
  return (
    <ul className="selectable divide-y divide-line/70 overflow-hidden rounded-lg border border-line">
      {items.map((it) => (
        <li key={it.name + it.definition} className="px-3 py-2 hover:bg-bg-2/50">
          <div className="flex items-center gap-2">
            <span className="truncate font-mono text-[12.5px] font-medium">{it.name}</span>
            <span className="shrink-0 rounded bg-bg-3 px-1.5 py-px text-[10.5px] text-fg-muted">{it.kind}</span>
          </div>
          <div className="mt-0.5 break-all font-mono text-[11.5px] text-fg-subtle">{it.definition}</div>
        </li>
      ))}
    </ul>
  );
}
