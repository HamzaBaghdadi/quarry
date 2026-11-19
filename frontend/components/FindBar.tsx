"use client";

import { useEffect, useRef } from "react";
import { CaretDown, CaretUp, Database, MagnifyingGlass, TextAa, X } from "@phosphor-icons/react";
import { Toggle } from "@/components/ui/toggle";
import { cn } from "@/lib/utils";
import { IconButton, Input, Tip } from "./common";

export interface FindState {
  open: boolean;
  term: string;
  matchCase: boolean;
  index: number;
}

export const closedFind: FindState = { open: false, term: "", matchCase: false, index: 0 };

/**
 * Floating find bar (Ctrl/⌘+F) for result grids: highlights matches in the
 * loaded rows and steps through them. Table tabs also offer "search all
 * rows", which asks the database to filter the whole table.
 */
export function FindBar({
  state,
  onChange,
  count,
  onClose,
  onSearchAll,
}: {
  state: FindState;
  onChange: (s: FindState) => void;
  count: number;
  onClose: () => void;
  onSearchAll?: (term: string) => void;
}) {
  const ref = useRef<HTMLInputElement>(null);
  useEffect(() => {
    ref.current?.focus();
    ref.current?.select();
  }, [state.open]);

  const step = (d: number) => count && onChange({ ...state, index: (state.index + d + count) % count });

  return (
    <div
      role="search"
      aria-label="Find in results"
      className="animate-pop absolute right-3 top-9 z-30 flex items-center gap-1 rounded-lg border border-line-strong bg-popover p-1 pl-2 shadow-pop"
    >
      <MagnifyingGlass aria-hidden className="size-3.5 shrink-0 text-fg-subtle" />
      <Input
        ref={ref}
        value={state.term}
        placeholder="Find in results"
        aria-label="Find in results"
        onChange={(e) => onChange({ ...state, term: e.target.value, index: 0 })}
        onKeyDown={(e) => {
          if (e.key === "Enter" && (e.ctrlKey || e.metaKey) && onSearchAll && state.term.trim()) {
            e.preventDefault();
            onSearchAll(state.term.trim());
          } else if (e.key === "Enter") {
            e.preventDefault();
            step(e.shiftKey ? -1 : 1);
          } else if (e.key === "Escape") {
            e.preventDefault();
            onClose();
          }
        }}
        className="h-7 w-52 border-0 bg-transparent px-1 text-[12.5px] focus-visible:ring-0 dark:bg-transparent"
      />
      <span aria-live="polite" className={cn("min-w-14 text-right font-mono text-[11px] tabular-nums", state.term && !count ? "text-danger" : "text-fg-subtle")}>
        {state.term ? (count ? `${Math.min(state.index + 1, count)} of ${count}` : "No results") : ""}
      </span>
      <Tip label="Match case">
        <Toggle
          size="sm"
          aria-label="Match case"
          pressed={state.matchCase}
          onPressedChange={(v) => onChange({ ...state, matchCase: v, index: 0 })}
          className="size-7 min-w-7 cursor-pointer p-0 text-fg-muted data-[state=on]:bg-brand-soft data-[state=on]:text-brand-text"
        >
          <TextAa className="size-4" />
        </Toggle>
      </Tip>
      <IconButton label="Previous match (Shift+Enter)" disabled={!count} onClick={() => step(-1)}>
        <CaretUp className="size-3.5" />
      </IconButton>
      <IconButton label="Next match (Enter)" disabled={!count} onClick={() => step(1)}>
        <CaretDown className="size-3.5" />
      </IconButton>
      {onSearchAll && (
        <Tip label="Search every row in the table, not just this page (Ctrl+Enter)">
          <button
            disabled={!state.term.trim()}
            onClick={() => onSearchAll(state.term.trim())}
            className="ml-0.5 flex h-7 cursor-pointer items-center gap-1.5 rounded-md bg-bg-2 px-2 text-[12px] font-medium text-fg transition-colors duration-150 hover:bg-bg-3 disabled:cursor-default disabled:opacity-45"
          >
            <Database aria-hidden className="size-3.5 text-brand-text" /> All rows
          </button>
        </Tip>
      )}
      <IconButton label="Close (Esc)" onClick={onClose}>
        <X className="size-3.5" />
      </IconButton>
    </div>
  );
}

/** Find every [row, col] whose text contains term. */
export function findMatches(rows: unknown[][], term: string, matchCase: boolean): [number, number][] {
  if (!term) return [];
  const needle = matchCase ? term : term.toLowerCase();
  const out: [number, number][] = [];
  for (let r = 0; r < rows.length; r++) {
    const row = rows[r];
    for (let c = 0; c < row.length; c++) {
      const v = row[c];
      if (v === null || v === undefined) continue;
      const s = matchCase ? String(v) : String(v).toLowerCase();
      if (s.includes(needle)) out.push([r, c]);
    }
    if (out.length > 50_000) break;
  }
  return out;
}
