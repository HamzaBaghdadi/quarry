"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { ArrowClockwise, ArrowCounterClockwise, CaretLeft, CaretRight, Code, FloppyDisk, Funnel, LockSimple, MagnifyingGlass, Plus, X } from "@phosphor-icons/react";
import { toast } from "sonner";
import { api, newRunId, type Cell, type ChangeSet, type Result } from "@/lib/api";
import { useDriver, useStore, type Tab } from "@/lib/store";
import type { GridEdits } from "./DataGrid";
import { ResultPanel } from "./ResultPanel";
import { Button, IconButton, Input, Kbd, Select, Spinner, Tip, confirmDialog, mod } from "./common";

const PAGE_SIZES = [100, 200, 500, 1000];

/** Turn edited text back into a typed value, guided by the original value and column type. */
function coerce(text: string | null, original: Cell | undefined, type: string): Cell {
  if (text === null) return null;
  const t = type.toLowerCase();
  if (typeof original === "number" || /^(int|smallint|bigint|tinyint|mediumint|integer|float|double|real|int\d)/.test(t)) {
    const n = Number(text);
    if (text.trim() !== "" && Number.isFinite(n) && Math.abs(n) <= Number.MAX_SAFE_INTEGER) return n;
  }
  if (typeof original === "boolean" || /^bool|^bit$/.test(t)) {
    if (/^(true|t|1|yes)$/i.test(text.trim())) return true;
    if (/^(false|f|0|no)$/i.test(text.trim())) return false;
  }
  return text;
}

const same = (a: Cell | undefined, b: Cell) => a === b || (a !== null && a !== undefined && b !== null && String(a) === String(b));

export function TableTab({ tab }: { tab: Tab }) {
  const conn = useStore((s) => s.connections.find((c) => c.id === tab.connId));
  const active = useStore((s) => s.activeTab === tab.id);
  const driver = useDriver(conn?.driver);
  const newQuery = useStore((s) => s.newQuery);

  const [limit, setLimit] = useState(200);
  const [offset, setOffset] = useState(0);
  const [filterDraft, setFilterDraft] = useState("");
  const [filter, setFilter] = useState("");
  const [search, setSearch] = useState("");
  const [sort, setSort] = useState<{ col: string; desc: boolean } | undefined>();
  const [result, setResult] = useState<Result | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [reloadTick, setReloadTick] = useState(0);
  const runId = useRef("");

  // Pending edits.
  const [changed, setChanged] = useState<Map<number, Record<string, Cell>>>(new Map());
  const [deleted, setDeleted] = useState<Set<number>>(new Set());
  const [added, setAdded] = useState<Record<string, Cell>[]>([]);
  const [saving, setSaving] = useState(false);
  const pending = changed.size + deleted.size + added.length;

  const clearPending = () => {
    setChanged(new Map());
    setDeleted(new Set());
    setAdded([]);
  };

  /** Run fn, asking first if it would throw away unsaved edits. */
  const guard = async (fn: () => void) => {
    if (
      pending &&
      !(await confirmDialog({
        title: `Discard ${pending} unsaved change${pending === 1 ? "" : "s"}?`,
        description: "Your edits to this table haven't been saved yet.",
        confirmLabel: "Discard",
        destructive: true,
      }))
    )
      return;
    clearPending();
    fn();
  };

  const load = useCallback(async () => {
    runId.current = newRunId();
    setLoading(true);
    setError(null);
    try {
      const r = await api.browse(
        tab.connId,
        { path: tab.path ?? [], limit, offset, filter, search, orderBy: sort?.col ?? "", desc: !!sort?.desc },
        runId.current,
      );
      setResult(r);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setLoading(false);
    }
    // reloadTick forces a reload with identical params.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tab.connId, tab.path, limit, offset, filter, search, sort, reloadTick]);

  useEffect(() => {
    void load();
  }, [load]);

  const columns = useMemo(() => result?.columns ?? [], [result]);
  const rows = useMemo(() => result?.rows ?? [], [result]);
  const keys = result?.keys ?? [];
  const editable = keys.length > 0 && !error;

  const edits: GridEdits | undefined = editable
    ? {
        keys,
        changed,
        deleted,
        added,
        onEdit(r, col, text) {
          const ci = columns.findIndex((c) => c.name === col);
          const type = columns[ci]?.type ?? "";
          if (r >= rows.length) {
            const i = r - rows.length;
            setAdded((a) => a.map((row, j) => (j === i ? { ...row, [col]: coerce(text, undefined, type) } : row)));
            return;
          }
          const original = rows[r][ci];
          const value = coerce(text, original, type);
          setChanged((m) => {
            const next = new Map(m);
            const row = { ...(next.get(r) ?? {}) };
            if (same(original, value)) delete row[col];
            else row[col] = value;
            if (Object.keys(row).length) next.set(r, row);
            else next.delete(r);
            return next;
          });
        },
        onToggleDelete(r) {
          if (r >= rows.length) {
            setAdded((a) => a.filter((_, j) => j !== r - rows.length));
            return;
          }
          setDeleted((s) => {
            const next = new Set(s);
            if (next.has(r)) next.delete(r);
            else next.add(r);
            return next;
          });
        },
      }
    : undefined;

  const save = useCallback(async () => {
    if (!pending || saving) return;
    const idx = (name: string) => columns.findIndex((c) => c.name === name);
    const keyOf = (r: number) => Object.fromEntries(keys.map((k) => [k, rows[r][idx(k)]]));
    const cs: ChangeSet = {
      path: tab.path ?? [],
      updates: [...changed.entries()].filter(([r]) => !deleted.has(r)).map(([r, values]) => ({ key: keyOf(r), values })),
      inserts: added,
      deletes: [...deleted].map(keyOf),
      types: Object.fromEntries(columns.map((c) => [c.name, c.type])),
    };
    setSaving(true);
    try {
      const r = await api.applyChanges(tab.connId, cs);
      toast.success(r.message || "Saved");
      clearPending();
      setReloadTick((n) => n + 1);
    } catch (e) {
      toast.error("Nothing was saved", { description: (e as Error).message, duration: 10000 });
    } finally {
      setSaving(false);
    }
  }, [pending, saving, columns, keys, rows, changed, deleted, added, tab.path, tab.connId]);

  // Ctrl/⌘+S saves; F5 refreshes — only for the visible tab.
  useEffect(() => {
    if (!active) return;
    const onKey = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "s") {
        e.preventDefault();
        void save();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [active, save]);

  const onSort = (col: string) =>
    guard(() => {
      setOffset(0);
      setSort((s) => (s?.col !== col ? { col, desc: false } : s.desc ? undefined : { col, desc: true }));
    });

  const family = driver?.family;
  const filterLabel = family === "document" ? "Filter" : family === "keyvalue" ? null : "WHERE";
  const filterPlaceholder = family === "document" ? "{ status: 'active', age: { $gt: 30 } }" : "status = 'active' AND created_at > now() - interval '7 days'";
  const shown = rows.length;

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="flex h-10 shrink-0 items-center gap-2 border-b border-line bg-bg-1 px-2">
        {filterLabel ? (
          <form
            className="flex min-w-0 flex-1 items-center gap-0 rounded-md border border-line bg-bg-0 focus-within:border-brand focus-within:ring-2 focus-within:ring-brand-soft"
            onSubmit={(e) => {
              e.preventDefault();
              guard(() => {
                setOffset(0);
                setFilter(filterDraft.trim());
              });
            }}
          >
            <span className="flex h-7 items-center gap-1 border-r border-line px-2 font-mono text-[11px] font-semibold text-brand-text">
              <Funnel aria-hidden className="size-3" /> {filterLabel}
            </span>
            <Input
              value={filterDraft}
              onChange={(e) => setFilterDraft(e.target.value)}
              placeholder={filterPlaceholder}
              aria-label={`${filterLabel} filter, press Enter to apply`}
              spellCheck={false}
              className="h-7 min-w-0 flex-1 rounded-none border-0 bg-transparent px-2 font-mono text-[12px] focus-visible:ring-0 dark:bg-transparent"
            />
            {filter && (
              <IconButton
                type="button"
                label="Clear filter"
                className="mr-0.5 size-6"
                onClick={() =>
                  guard(() => {
                    setFilterDraft("");
                    setFilter("");
                    setOffset(0);
                  })
                }
              >
                <X className="size-3" />
              </IconButton>
            )}
          </form>
        ) : (
          <div className="flex-1 truncate font-mono text-[12px] text-fg-muted">{tab.path?.join(" › ")}</div>
        )}

        {search && (
          <span className="flex h-7 shrink-0 items-center gap-1 rounded-md border border-brand/40 bg-brand-soft pl-2 pr-0.5 text-[12px] text-fg">
            <MagnifyingGlass aria-hidden className="size-3.5 text-brand-text" />
            <span className="max-w-40 truncate">
              All rows matching <strong className="font-semibold">“{search}”</strong>
            </span>
            <IconButton label="Clear search" className="size-6" onClick={() => guard(() => { setOffset(0); setSearch(""); })}>
              <X className="size-3" />
            </IconButton>
          </span>
        )}
        {editable ? (
          <Button size="sm" variant="ghost" onClick={() => setAdded((a) => [...a, {}])}>
            <Plus aria-hidden className="size-3.5" /> Add row
          </Button>
        ) : (
          result?.editBlocked && (
            <Tip label={result.editBlocked}>
            <span tabIndex={0} className="flex shrink-0 items-center gap-1 text-[11px] text-fg-subtle">
              <LockSimple aria-hidden className="size-3" /> Read-only
            </span>
            </Tip>
          )
        )}
        <IconButton label="Refresh" onClick={() => guard(() => setReloadTick((n) => n + 1))}>
          <ArrowClockwise className="size-4" />
        </IconButton>
        {result?.statement && (
          <IconButton label="Open as query" onClick={() => newQuery(tab.connId, tab.scope, result.statement)}>
            <Code className="size-4" />
          </IconButton>
        )}
      </div>

      {pending > 0 && (
        <div role="status" className="animate-pop flex h-10 shrink-0 items-center gap-3 border-b border-warn/30 bg-warn/10 px-3 text-[12.5px]">
          <span className="size-2 rounded-full bg-warn" aria-hidden />
          <span>
            <strong className="font-semibold">{pending}</strong> unsaved change{pending === 1 ? "" : "s"}
            <span className="ml-2 text-fg-muted">
              {[changed.size && `${changed.size} edited`, added.length && `${added.length} new`, deleted.size && `${deleted.size} to delete`].filter(Boolean).join(" · ")}
            </span>
          </span>
          <div className="ml-auto flex items-center gap-2">
            <Button size="sm" variant="ghost" onClick={clearPending} disabled={saving}>
              <ArrowCounterClockwise aria-hidden className="size-3.5" /> Discard
            </Button>
            <Button size="sm" variant="primary" onClick={save} disabled={saving}>
              {saving ? <Spinner /> : <FloppyDisk aria-hidden className="size-3.5" />} Save changes
              <span className="ml-1 opacity-75">
                <Kbd>{mod()}</Kbd> <Kbd>S</Kbd>
              </span>
            </Button>
          </div>
        </div>
      )}

      <ResultPanel
        result={result}
        error={error}
        loading={loading}
        onCancel={() => api.cancel(runId.current)}
        name={tab.title}
        offset={offset}
        sort={family === "keyvalue" ? undefined : sort}
        onSort={family === "keyvalue" ? undefined : onSort}
        edits={edits}
        active={active}
        onSearchAll={(term) =>
          guard(() => {
            setOffset(0);
            setSearch(term);
          })
        }
        footerExtra={
          <div className="mr-2 flex items-center gap-1">
            {editable && (
              <span className="mr-2 hidden text-[11px] text-fg-subtle md:inline">
                Double-click or type to edit · <Kbd>Del</Kbd> marks row for deletion
              </span>
            )}
            <IconButton label="Previous page" className="size-6" disabled={offset === 0 || loading} onClick={() => guard(() => setOffset(Math.max(0, offset - limit)))}>
              <CaretLeft className="size-3.5" />
            </IconButton>
            <span className="min-w-20 text-center tabular-nums">{shown ? `${offset + 1}–${offset + shown}` : "0"}</span>
            <IconButton label="Next page" className="size-6" disabled={!result?.hasMore || loading} onClick={() => guard(() => setOffset(offset + limit))}>
              <CaretRight className="size-3.5" />
            </IconButton>
            <Select
              aria-label="Rows per page"
              size="sm"
              value={String(limit)}
              onChange={(v) => {
                const n = Number(v);
                guard(() => {
                  setOffset(0);
                  setLimit(n);
                });
              }}
              className="!h-6 w-auto text-[11.5px]"
              options={PAGE_SIZES.map((n) => ({ value: String(n), label: `${n} / page` }))}
            />
          </div>
        }
      />
    </div>
  );
}
