"use client";

import clsx from "clsx";
import { useState, type ReactNode } from "react";
import { BracketsCurly, CheckCircle, Copy, DownloadSimple, Rows, WarningOctagon } from "@phosphor-icons/react";
import { toast } from "sonner";
import { api, type Result } from "@/lib/api";
import { DataGrid } from "./DataGrid";
import { Button, Spinner } from "./ui";

export function ResultPanel({
  result,
  error,
  loading,
  onCancel,
  name,
  offset,
  sort,
  onSort,
  footerExtra,
  emptyHint,
}: {
  result: Result | null;
  error: string | null;
  loading: boolean;
  onCancel?: () => void;
  name: string;
  offset?: number;
  sort?: { col: string; desc: boolean };
  onSort?: (col: string) => void;
  footerExtra?: ReactNode;
  emptyHint?: ReactNode;
}) {
  const [view, setView] = useState<"grid" | "json">("grid");
  const hasDocs = !!result?.documents?.length;
  const columns = result?.columns ?? [];
  const rows = result?.rows ?? [];

  const doExport = async (format: "csv" | "json") => {
    if (!result) return;
    try {
      const path = await api.exportResult(format, result, name.replace(/[^\w.-]+/g, "_"));
      if (path) toast.success("Exported", { description: path });
    } catch (e) {
      toast.error("Export failed", { description: (e as Error).message });
    }
  };

  let body: ReactNode;
  if (error) {
    body = (
      <div role="alert" className="m-3 flex items-start gap-2.5 rounded-lg border border-danger/30 bg-danger-soft p-3">
        <WarningOctagon aria-hidden weight="fill" className="mt-0.5 size-4 shrink-0 text-danger" />
        <div className="min-w-0 flex-1">
          <p className="text-[13px] font-semibold text-danger">Query failed</p>
          <pre className="selectable mt-1 whitespace-pre-wrap break-words font-mono text-[12px] text-fg">{error}</pre>
        </div>
        <button aria-label="Copy error" title="Copy error" onClick={() => navigator.clipboard.writeText(error)} className="cursor-pointer rounded p-1 text-fg-muted hover:bg-bg-2 hover:text-fg">
          <Copy className="size-3.5" />
        </button>
      </div>
    );
  } else if (!result) {
    body = <div className="flex flex-1 items-center justify-center p-6 text-center text-[12px] text-fg-subtle">{loading ? null : emptyHint}</div>;
  } else if (!columns.length) {
    body = (
      <div className="m-3 flex items-center gap-2.5 rounded-lg border border-run/25 bg-run/10 p-3 text-[13px]">
        <CheckCircle aria-hidden weight="fill" className="size-4 text-run" />
        <span>{result.message || "Statement executed"}</span>
      </div>
    );
  } else if (view === "json" && hasDocs) {
    body = (
      <pre className="selectable min-h-0 flex-1 overflow-auto bg-bg-1 p-3 font-mono text-[12.5px] leading-relaxed text-fg">
        {result.documents!.join(",\n")}
      </pre>
    );
  } else {
    body = <DataGrid columns={columns} rows={rows} offset={offset} sort={sort} onSort={onSort} label={`${name} results`} />;
  }

  return (
    <div className="relative flex min-h-0 flex-1 flex-col">
      {body}
      {loading && (
        <div className="absolute inset-0 z-30 flex items-center justify-center bg-bg-1/55 backdrop-blur-[1px]" aria-live="polite">
          <div className="flex items-center gap-3 rounded-lg border border-line-strong bg-bg-2 px-4 py-2.5 shadow-pop">
            <Spinner className="text-accent" />
            <span className="text-[13px]">Running…</span>
            {onCancel && (
              <Button size="sm" variant="ghost" onClick={onCancel}>
                Cancel
              </Button>
            )}
          </div>
        </div>
      )}

      <footer className="flex h-8 shrink-0 items-center gap-3 border-t border-line bg-bg-0 px-2.5 text-[12px] text-fg-muted">
        {result && !error ? (
          <>
            {columns.length > 0 && (
              <span className="tabular-nums">
                <strong className="font-semibold text-fg">{rows.length.toLocaleString()}</strong> {rows.length === 1 ? "row" : "rows"}
                {result.hasMore && <span className="text-warn"> · more available</span>}
              </span>
            )}
            <span className="tabular-nums">{result.durationMs.toFixed(result.durationMs < 10 ? 1 : 0)} ms</span>
            {result.message && columns.length > 0 && <span className="truncate">{result.message}</span>}
          </>
        ) : (
          <span>{error ? "Error" : "No results yet"}</span>
        )}
        <div className="ml-auto flex items-center gap-1">
          {footerExtra}
          {hasDocs && (
            <div role="radiogroup" aria-label="Result view" className="flex rounded-md border border-line p-0.5">
              {(["grid", "json"] as const).map((m) => (
                <button
                  key={m}
                  role="radio"
                  aria-checked={view === m}
                  onClick={() => setView(m)}
                  className={clsx("flex h-5 cursor-pointer items-center gap-1 rounded px-1.5 text-[11px]", view === m ? "bg-bg-3 text-fg" : "text-fg-muted hover:text-fg")}
                >
                  {m === "grid" ? <Rows aria-hidden className="size-3" /> : <BracketsCurly aria-hidden className="size-3" />}
                  {m === "grid" ? "Grid" : "JSON"}
                </button>
              ))}
            </div>
          )}
          {columns.length > 0 && !error && (
            <>
              <Button size="sm" variant="ghost" className="h-6 px-1.5" onClick={() => doExport("csv")} title="Export as CSV">
                <DownloadSimple aria-hidden className="size-3.5" /> CSV
              </Button>
              <Button size="sm" variant="ghost" className="h-6 px-1.5" onClick={() => doExport("json")} title="Export as JSON">
                <DownloadSimple aria-hidden className="size-3.5" /> JSON
              </Button>
            </>
          )}
        </div>
      </footer>
    </div>
  );
}
