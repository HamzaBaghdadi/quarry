"use client";

import clsx from "clsx";
import { useVirtualizer } from "@tanstack/react-virtual";
import { memo, useEffect, useMemo, useRef, useState, type KeyboardEvent } from "react";
import { ArrowDown, ArrowUp, Key } from "@phosphor-icons/react";
import type { Cell, Column } from "@/lib/api";
import { useStore } from "@/lib/store";
import { toast } from "sonner";

const ROW_H = 26;
const NUM_W = 52;

export function formatCell(v: Cell | undefined): string {
  if (v === null || v === undefined) return "NULL";
  if (typeof v === "boolean") return v ? "true" : "false";
  return String(v);
}

/** Pending edits for an editable grid. Row indexes ≥ rows.length are new rows. */
export interface GridEdits {
  keys: string[];
  changed: Map<number, Record<string, Cell>>;
  deleted: Set<number>;
  added: Record<string, Cell>[];
  onEdit(row: number, col: string, text: string | null): void;
  onToggleDelete(row: number): void;
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
  edits,
}: {
  columns: Column[];
  rows: Cell[][];
  offset?: number;
  sort?: { col: string; desc: boolean };
  onSort?: (col: string) => void;
  label: string;
  edits?: GridEdits;
}) {
  const scrollRef = useRef<HTMLDivElement>(null);
  const [widths, setWidths] = useState<number[]>([]);
  const [sel, setSel] = useState<{ r: number; c: number } | null>(null);
  const [editing, setEditing] = useState<{ r: number; c: number; seed?: string } | null>(null);
  const inspect = useStore((s) => s.inspect);

  const total = rows.length + (edits?.added.length ?? 0);

  // Reset sizing & selection when the shape changes.
  const shape = columns.map((c) => c.name).join("\u0000");
  useEffect(() => {
    setWidths(columns.map((c, i) => guessWidth(c, rows, i)));
    setSel(null);
    setEditing(null);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [shape]);

  const template = useMemo(() => `${NUM_W}px ${columns.map((_, i) => `${widths[i] ?? 120}px`).join(" ")}`, [columns, widths]);
  const totalW = NUM_W + columns.reduce((a, _, i) => a + (widths[i] ?? 120), 0);

  const v = useVirtualizer({
    count: total,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => ROW_H,
    overscan: 16,
  });

  /** Current value of a cell, including pending edits. undefined = DEFAULT (new row). */
  const valueAt = (r: number, c: number): Cell | undefined => {
    const name = columns[c].name;
    if (r >= rows.length) return edits?.added[r - rows.length]?.[name];
    const ch = edits?.changed.get(r);
    if (ch && name in ch) return ch[name];
    return rows[r]?.[c];
  };
  const isChanged = (r: number, c: number) => {
    if (r >= rows.length) return edits?.added[r - rows.length] && columns[c].name in edits.added[r - rows.length];
    const ch = edits?.changed.get(r);
    return !!ch && columns[c].name in ch;
  };

  const canEdit = (r: number) => !!edits && !edits.deleted.has(r);

  const openInspector = (r: number, c: number) => {
    const col = columns[c];
    const value = valueAt(r, c) ?? null;
    inspect({
      column: col.name,
      type: col.type,
      value,
      onSave: canEdit(r) ? (text) => edits!.onEdit(r, col.name, text) : undefined,
    });
  };

  const startEdit = (r: number, c: number, seed?: string) => {
    if (!canEdit(r)) {
      openInspector(r, c);
      return;
    }
    const cur = valueAt(r, c);
    // Long or multi-line values are nicer to edit in the side panel.
    if (seed === undefined && typeof cur === "string" && (cur.length > 160 || cur.includes("\n"))) {
      openInspector(r, c);
      return;
    }
    setEditing({ r, c, seed });
  };

  const commit = (text: string | null, move?: "down" | "right") => {
    if (!editing || !edits) return;
    const { r, c } = editing;
    edits.onEdit(r, columns[c].name, text);
    setEditing(null);
    scrollRef.current?.focus();
    if (move === "down") setSel({ r: Math.min(total - 1, r + 1), c });
    else if (move === "right") setSel({ r, c: Math.min(columns.length - 1, c + 1) });
  };

  const copy = (text: string, what: string) => {
    navigator.clipboard.writeText(text).then(
      () => toast.success(`Copied ${what}`),
      () => toast.error("Clipboard unavailable"),
    );
  };

  const onKeyDown = (e: KeyboardEvent) => {
    if (editing) return; // the input handles its own keys
    if (!sel) {
      if (total && /^Arrow/.test(e.key)) setSel({ r: 0, c: 0 });
      return;
    }
    let { r, c } = sel;
    switch (e.key) {
      case "ArrowDown": r++; break;
      case "ArrowUp": r--; break;
      case "ArrowRight": case "Tab": c += e.shiftKey && e.key === "Tab" ? -1 : 1; break;
      case "ArrowLeft": c--; break;
      case "PageDown": r += 20; break;
      case "PageUp": r -= 20; break;
      case "Home": c = 0; break;
      case "End": c = columns.length - 1; break;
      case "Enter": case "F2": e.preventDefault(); startEdit(r, c); return;
      case " ": e.preventDefault(); openInspector(r, c); return;
      case "Escape": setSel(null); return;
      case "Delete":
        if (edits) {
          e.preventDefault();
          edits.onToggleDelete(r);
        }
        return;
      case "c":
        if (e.ctrlKey || e.metaKey) {
          e.preventDefault();
          if (e.shiftKey) copy(columns.map((_, i) => formatCell(valueAt(r, i))).join("\t"), "row");
          else copy(formatCell(valueAt(r, c)), "value");
          return;
        }
      // fallthrough: typing "c" starts an edit
      default:
        if (edits && e.key.length === 1 && !e.ctrlKey && !e.metaKey && !e.altKey) {
          e.preventDefault();
          startEdit(r, c, e.key);
        }
        return;
    }
    e.preventDefault();
    r = Math.max(0, Math.min(total - 1, r));
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

  // Keep the newest added row in view.
  const addedCount = edits?.added.length ?? 0;
  useEffect(() => {
    if (addedCount > 0) {
      v.scrollToIndex(total - 1);
      setSel({ r: total - 1, c: 0 });
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [addedCount]);

  return (
    <div
      ref={scrollRef}
      tabIndex={0}
      role="grid"
      aria-label={label}
      aria-readonly={!edits}
      aria-rowcount={total + 1}
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
            const isKey = edits?.keys.includes(col.name);
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
                  {isKey && <Key aria-label="Primary key" weight="fill" className="size-3 shrink-0 text-warn" />}
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
            const r = vr.index;
            const isNew = r >= rows.length;
            const isDeleted = !!edits?.deleted.has(r);
            return (
              <div
                key={vr.key}
                role="row"
                aria-rowindex={r + 2}
                className={clsx(
                  "absolute left-0 grid w-full border-b border-line/70",
                  isDeleted ? "bg-danger-soft" : isNew ? "bg-run/10" : r % 2 === 1 && "bg-bg-2/35",
                )}
                style={{ transform: `translateY(${vr.start}px)`, gridTemplateColumns: template, height: ROW_H }}
              >
                <div
                  className={clsx(
                    "sticky left-0 z-10 flex items-center justify-end border-r border-line bg-bg-0 pr-2 text-[11px] tabular-nums",
                    isDeleted ? "text-danger" : isNew ? "text-run" : "text-fg-subtle",
                  )}
                  title={isDeleted ? "Will be deleted" : isNew ? "New row" : undefined}
                >
                  {isDeleted ? "−" : isNew ? "+" : offset + r + 1}
                </div>
                {columns.map((col, ci) => {
                  const selected = sel?.r === r && sel?.c === ci;
                  const isEditing = editing?.r === r && editing?.c === ci;
                  const changed = isChanged(r, ci);
                  const value = valueAt(r, ci);
                  return (
                    <div
                      key={ci}
                      role="gridcell"
                      aria-selected={selected}
                      onMouseDown={() => setSel({ r, c: ci })}
                      onDoubleClick={() => (edits ? startEdit(r, ci) : openInspector(r, ci))}
                      className={clsx(
                        "relative flex min-w-0 cursor-default items-center overflow-hidden border-r border-line/70 px-2",
                        typeof value === "number" && "justify-end",
                        changed && !isNew && "bg-warn/15",
                        isDeleted && "line-through decoration-danger/70",
                        selected && !isEditing && "bg-accent-soft outline outline-2 -outline-offset-2 outline-accent",
                      )}
                    >
                      {isEditing ? (
                        <CellEditor
                          initial={editing!.seed ?? (value === null || value === undefined ? "" : formatCell(value))}
                          wasNull={value === null}
                          onCommit={commit}
                          onCancel={() => {
                            setEditing(null);
                            scrollRef.current?.focus();
                          }}
                        />
                      ) : isNew && value === undefined ? (
                        <span className="text-[11px] italic text-fg-subtle">DEFAULT</span>
                      ) : (
                        <CellText v={value ?? null} />
                      )}
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

function CellEditor({
  initial,
  wasNull,
  onCommit,
  onCancel,
}: {
  initial: string;
  wasNull: boolean;
  onCommit: (text: string | null, move?: "down" | "right") => void;
  onCancel: () => void;
}) {
  const ref = useRef<HTMLInputElement>(null);
  const done = useRef(false);
  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    el.focus();
    el.setSelectionRange(el.value.length, el.value.length);
  }, []);
  const finish = (text: string | null, move?: "down" | "right") => {
    if (done.current) return;
    done.current = true;
    onCommit(text, move);
  };
  return (
    <div className="absolute inset-0 z-10 flex items-stretch bg-bg-0 ring-2 ring-accent ring-inset">
      <input
        ref={ref}
        defaultValue={initial}
        placeholder={wasNull ? "NULL" : ""}
        aria-label="Edit cell value"
        spellCheck={false}
        onKeyDown={(e) => {
          e.stopPropagation();
          if (e.key === "Enter") finish(e.currentTarget.value, "down");
          else if (e.key === "Tab") {
            e.preventDefault();
            finish(e.currentTarget.value, "right");
          } else if (e.key === "Escape") {
            done.current = true;
            onCancel();
          }
        }}
        onBlur={(e) => finish(e.currentTarget.value)}
        className="min-w-0 flex-1 bg-transparent px-2 font-mono text-[12.5px] text-fg outline-none"
      />
      <button
        type="button"
        title="Set to NULL"
        onMouseDown={(e) => {
          e.preventDefault();
          finish(null, "down");
        }}
        className="shrink-0 cursor-pointer border-l border-line px-1.5 font-sans text-[10px] font-semibold text-fg-muted hover:bg-bg-2 hover:text-fg"
      >
        NULL
      </button>
    </div>
  );
}

function CellText({ v }: { v: Cell }) {
  if (v === null || v === undefined) return <span className="text-[11px] italic text-[var(--null)]">NULL</span>;
  if (typeof v === "boolean") return <span className="text-[var(--bool)]">{v ? "true" : "false"}</span>;
  if (typeof v === "number") return <span className="tabular-nums text-[var(--num)]">{v}</span>;
  const s = v.length > 300 ? v.slice(0, 300) + "…" : v;
  const json = s.startsWith("{") || s.startsWith("[");
  return <span className={clsx("truncate whitespace-pre", json ? "text-fg-muted" : "text-[var(--str)]")}>{s.replace(/\n/g, "↵")}</span>;
}
