"use client";

import clsx from "clsx";
import { useVirtualizer } from "@tanstack/react-virtual";
import { memo, useEffect, useMemo, useRef, useState, type KeyboardEvent } from "react";
import { ArrowDown, ArrowUp } from "@phosphor-icons/react";
import type { Cell, Column } from "@/lib/api";
import { useStore } from "@/lib/store";
import { toast } from "sonner";

const ROW_H = 26;
const NUM_W = 52;

export function formatCell(v: Cell): string {
  if (v === null || v === undefined) return "NULL";
  if (typeof v === "boolean") return v ? "true" : "false";
  return String(v);
}

function guessWidth(col: Column, rows: Cell[][], idx: number) {
  let chars = col.name.length + 3;
  for (let i = 0; i < Math.min(rows.length, 60); i++) {
    const v = rows[i][idx];
    chars = Math.max(chars, v === null ? 4 : String(v).length);
  }
  return Math.min(360, Math.max(72, chars * 7.4 + 20));
}

export const DataGrid = memo(function DataGrid({
  columns,
  rows,
  offset = 0,
  sort,
  onSort,
  label,
}: {
  columns: Column[];
  rows: Cell[][];
  offset?: number;
  sort?: { col: string; desc: boolean };
  onSort?: (col: string) => void;
  label: string;
}) {
  const scrollRef = useRef<HTMLDivElement>(null);
  const [widths, setWidths] = useState<number[]>([]);
  const [sel, setSel] = useState<{ r: number; c: number } | null>(null);
  const inspect = useStore((s) => s.inspect);

  // Reset sizing & selection when the shape changes.
  const shape = columns.map((c) => c.name).join("\u0000");
  useEffect(() => {
    setWidths(columns.map((c, i) => guessWidth(c, rows, i)));
    setSel(null);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [shape]);

  const template = useMemo(() => `${NUM_W}px ${columns.map((_, i) => `${widths[i] ?? 120}px`).join(" ")}`, [columns, widths]);
  const totalW = NUM_W + columns.reduce((a, _, i) => a + (widths[i] ?? 120), 0);

  const v = useVirtualizer({
    count: rows.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => ROW_H,
    overscan: 16,
  });

  const openInspector = (r: number, c: number) => {
    const col = columns[c];
    inspect({ column: col.name, type: col.type, value: rows[r]?.[c] });
  };

  const copy = (text: string, what: string) => {
    navigator.clipboard.writeText(text).then(
      () => toast.success(`Copied ${what}`),
      () => toast.error("Clipboard unavailable"),
    );
  };

  const onKeyDown = (e: KeyboardEvent) => {
    if (!sel) {
      if (rows.length && /^Arrow/.test(e.key)) setSel({ r: 0, c: 0 });
      return;
    }
    let { r, c } = sel;
    switch (e.key) {
      case "ArrowDown": r++; break;
      case "ArrowUp": r--; break;
      case "ArrowRight": c++; break;
      case "ArrowLeft": c--; break;
      case "PageDown": r += 20; break;
      case "PageUp": r -= 20; break;
      case "Home": c = 0; break;
      case "End": c = columns.length - 1; break;
      case "Enter": openInspector(r, c); e.preventDefault(); return;
      case "Escape": setSel(null); return;
      case "c":
        if (e.ctrlKey || e.metaKey) {
          e.preventDefault();
          if (e.shiftKey) copy(columns.map((col, i) => formatCell(rows[r][i])).join("\t"), "row");
          else copy(formatCell(rows[r][c]), "value");
        }
        return;
      default: return;
    }
    e.preventDefault();
    r = Math.max(0, Math.min(rows.length - 1, r));
    c = Math.max(0, Math.min(columns.length - 1, c));
    setSel({ r, c });
    v.scrollToIndex(r);
    const el = scrollRef.current;
    if (el) {
      const left = NUM_W + widths.slice(0, c).reduce((a, b) => a + b, 0);
      const right = left + (widths[c] ?? 120);
      if (left - NUM_W < el.scrollLeft) el.scrollLeft = left - NUM_W;
      else if (right > el.scrollLeft + el.clientWidth) el.scrollLeft = right - el.clientWidth;
    }
  };

  const startResize = (i: number, e: React.PointerEvent) => {
    e.preventDefault();
    e.stopPropagation();
    const x0 = e.clientX;
    const w0 = widths[i] ?? 120;
    const move = (ev: PointerEvent) => setWidths((ws) => ws.map((w, j) => (j === i ? Math.max(48, w0 + ev.clientX - x0) : w)));
    const up = () => {
      window.removeEventListener("pointermove", move);
      window.removeEventListener("pointerup", up);
      document.body.style.cursor = "";
    };
    document.body.style.cursor = "col-resize";
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", up);
  };

  return (
    <div
      ref={scrollRef}
      tabIndex={0}
      role="grid"
      aria-label={label}
      aria-rowcount={rows.length + 1}
      aria-colcount={columns.length}
      onKeyDown={onKeyDown}
      className="selectable relative min-h-0 flex-1 overflow-auto bg-bg-1 font-mono text-[12.5px] focus-visible:outline-none"
    >
      <div style={{ width: totalW, minWidth: "100%" }}>
        {/* Header */}
        <div role="row" className="sticky top-0 z-20 grid border-b border-line-strong bg-bg-0" style={{ gridTemplateColumns: template, height: 30 }}>
          <div className="sticky left-0 z-10 border-r border-line bg-bg-0" />
          {columns.map((col, i) => {
            const sorted = sort?.col === col.name;
            return (
              <div
                key={col.name + i}
                role="columnheader"
                aria-sort={sorted ? (sort!.desc ? "descending" : "ascending") : undefined}
                className="group relative flex min-w-0 items-center border-r border-line"
              >
                <button
                  disabled={!onSort}
                  onClick={() => onSort?.(col.name)}
                  title={onSort ? `Sort by ${col.name}` : col.name}
                  className="flex h-full min-w-0 flex-1 cursor-pointer items-center gap-1.5 px-2 text-left font-sans hover:bg-bg-2 disabled:cursor-default disabled:hover:bg-transparent"
                >
                  <span className="truncate text-[12px] font-semibold text-fg">{col.name}</span>
                  {col.type && <span className="truncate text-[10.5px] font-normal uppercase tracking-wide text-fg-subtle">{col.type}</span>}
                  {sorted && (sort!.desc ? <ArrowDown aria-hidden className="ml-auto size-3 shrink-0 text-accent" weight="bold" /> : <ArrowUp aria-hidden className="ml-auto size-3 shrink-0 text-accent" weight="bold" />)}
                </button>
                <span
                  aria-hidden
                  onPointerDown={(e) => startResize(i, e)}
                  onDoubleClick={() => setWidths((ws) => ws.map((w, j) => (j === i ? guessWidth(col, rows, i) : w)))}
                  className="absolute -right-1 top-0 z-10 h-full w-2 cursor-col-resize hover:bg-accent/60"
                />
              </div>
            );
          })}
        </div>

        {/* Body */}
        <div className="relative" style={{ height: v.getTotalSize() }}>
          {v.getVirtualItems().map((vr) => {
            const row = rows[vr.index];
            return (
              <div
                key={vr.key}
                role="row"
                aria-rowindex={vr.index + 2}
                className={clsx("absolute left-0 grid w-full border-b border-line/70", vr.index % 2 === 1 && "bg-bg-2/35")}
                style={{ transform: `translateY(${vr.start}px)`, gridTemplateColumns: template, height: ROW_H }}
              >
                <div className="sticky left-0 z-10 flex items-center justify-end border-r border-line bg-bg-0 pr-2 text-[11px] text-fg-subtle tabular-nums">
                  {offset + vr.index + 1}
                </div>
                {row.map((cell, ci) => {
                  const selected = sel?.r === vr.index && sel?.c === ci;
                  return (
                    <div
                      key={ci}
                      role="gridcell"
                      aria-selected={selected}
                      onMouseDown={() => setSel({ r: vr.index, c: ci })}
                      onDoubleClick={() => openInspector(vr.index, ci)}
                      className={clsx(
                        "flex min-w-0 cursor-default items-center overflow-hidden border-r border-line/70 px-2",
                        typeof cell === "number" && "justify-end",
                        selected && "bg-accent-soft outline outline-2 -outline-offset-2 outline-accent",
                      )}
                    >
                      <CellText v={cell} />
                    </div>
                  );
                })}
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
});

function CellText({ v }: { v: Cell }) {
  if (v === null || v === undefined) return <span className="text-[11px] italic text-[var(--null)]">NULL</span>;
  if (typeof v === "boolean") return <span className="text-[var(--bool)]">{v ? "true" : "false"}</span>;
  if (typeof v === "number") return <span className="tabular-nums text-[var(--num)]">{v}</span>;
  const s = v.length > 300 ? v.slice(0, 300) + "…" : v;
  const json = s.startsWith("{") || s.startsWith("[");
  return <span className={clsx("truncate whitespace-pre", json ? "text-fg-muted" : "text-[var(--str)]")}>{s.replace(/\n/g, "↵")}</span>;
}
