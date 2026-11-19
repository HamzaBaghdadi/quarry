"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { ArrowClockwise, CaretLeft, CaretRight, Code, Funnel, X } from "@phosphor-icons/react";
import { api, newRunId, type Result } from "@/lib/api";
import { useDriver, useStore, type Tab } from "@/lib/store";
import { ResultPanel } from "./ResultPanel";
import { IconButton, Select } from "./ui";

const PAGE_SIZES = [100, 200, 500, 1000];

export function TableTab({ tab }: { tab: Tab }) {
  const conn = useStore((s) => s.connections.find((c) => c.id === tab.connId));
  const driver = useDriver(conn?.driver);
  const newQuery = useStore((s) => s.newQuery);

  const [limit, setLimit] = useState(200);
  const [offset, setOffset] = useState(0);
  const [filterDraft, setFilterDraft] = useState("");
  const [filter, setFilter] = useState("");
  const [sort, setSort] = useState<{ col: string; desc: boolean } | undefined>();
  const [result, setResult] = useState<Result | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const runId = useRef("");

  const load = useCallback(async () => {
    runId.current = newRunId();
    setLoading(true);
    setError(null);
    try {
      const r = await api.browse(
        tab.connId,
        { path: tab.path ?? [], limit, offset, filter, orderBy: sort?.col ?? "", desc: !!sort?.desc },
        runId.current,
      );
      setResult(r);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setLoading(false);
    }
  }, [tab.connId, tab.path, limit, offset, filter, sort]);

  useEffect(() => {
    void load();
  }, [load]);

  const onSort = (col: string) => {
    setOffset(0);
    setSort((s) => (s?.col !== col ? { col, desc: false } : s.desc ? undefined : { col, desc: true }));
  };

  const family = driver?.family;
  const filterLabel = family === "document" ? "Filter" : family === "keyvalue" ? null : "WHERE";
  const filterPlaceholder = family === "document" ? "{ status: 'active', age: { $gt: 30 } }" : "status = 'active' AND created_at > now() - interval '7 days'";
  const rows = result?.rows?.length ?? 0;

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="flex h-10 shrink-0 items-center gap-2 border-b border-line bg-bg-1 px-2">
        {filterLabel && (
          <form
            className="flex min-w-0 flex-1 items-center gap-0 rounded-md border border-line bg-bg-0 focus-within:border-accent focus-within:ring-2 focus-within:ring-accent-soft"
            onSubmit={(e) => {
              e.preventDefault();
              setOffset(0);
              setFilter(filterDraft.trim());
            }}
          >
            <span className="flex h-7 items-center gap-1 border-r border-line px-2 font-mono text-[11px] font-semibold text-accent">
              <Funnel aria-hidden className="size-3" /> {filterLabel}
            </span>
            <input
              value={filterDraft}
              onChange={(e) => setFilterDraft(e.target.value)}
              placeholder={filterPlaceholder}
              aria-label={`${filterLabel} filter, press Enter to apply`}
              spellCheck={false}
              className="h-7 min-w-0 flex-1 bg-transparent px-2 font-mono text-[12px] text-fg placeholder:text-fg-subtle focus:outline-none"
            />
            {filter && (
              <IconButton
                type="button"
                label="Clear filter"
                className="mr-0.5 size-6"
                onClick={() => {
                  setFilterDraft("");
                  setFilter("");
                  setOffset(0);
                }}
              >
                <X className="size-3" />
              </IconButton>
            )}
          </form>
        )}
        {!filterLabel && <div className="flex-1 truncate font-mono text-[12px] text-fg-muted">{tab.path?.join(" › ")}</div>}
        <IconButton label="Refresh (F5)" onClick={load}>
          <ArrowClockwise className="size-4" />
        </IconButton>
        {result?.statement && (
          <IconButton label="Open as query" onClick={() => newQuery(tab.connId, tab.scope, result.statement)}>
            <Code className="size-4" />
          </IconButton>
        )}
      </div>

      <ResultPanel
        result={result}
        error={error}
        loading={loading}
        onCancel={() => api.cancel(runId.current)}
        name={tab.title}
        offset={offset}
        sort={family === "keyvalue" ? undefined : sort}
        onSort={family === "keyvalue" ? undefined : onSort}
        footerExtra={
          <div className="mr-2 flex items-center gap-1">
            <IconButton label="Previous page" className="size-6" disabled={offset === 0 || loading} onClick={() => setOffset(Math.max(0, offset - limit))}>
              <CaretLeft className="size-3.5" />
            </IconButton>
            <span className="min-w-20 text-center tabular-nums">
              {rows ? `${offset + 1}–${offset + rows}` : "0"}
            </span>
            <IconButton label="Next page" className="size-6" disabled={!result?.hasMore || loading} onClick={() => setOffset(offset + limit)}>
              <CaretRight className="size-3.5" />
            </IconButton>
            <Select
              aria-label="Rows per page"
              value={limit}
              onChange={(e) => {
                setOffset(0);
                setLimit(Number(e.target.value));
              }}
              className="!h-6 !w-auto !px-1 text-[11px]"
            >
              {PAGE_SIZES.map((n) => (
                <option key={n} value={n}>
                  {n} / page
                </option>
              ))}
            </Select>
          </div>
        }
      />
    </div>
  );
}
