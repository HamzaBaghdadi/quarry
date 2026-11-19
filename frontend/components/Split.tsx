"use client";

import clsx from "clsx";
import { useCallback, useEffect, useRef, useState, type ReactNode } from "react";

/**
 * Two-pane resizable split. The first pane gets a size in px (horizontal) or a
 * fraction (vertical); the separator is draggable and keyboard-operable
 * (arrow keys), and the size is remembered per `storageKey`.
 */
export function Split({
  direction,
  first,
  second,
  initial,
  min,
  max,
  storageKey,
  className,
}: {
  direction: "horizontal" | "vertical";
  first: ReactNode;
  second: ReactNode;
  initial: number; // px for horizontal, fraction 0..1 for vertical
  min: number;
  max: number;
  storageKey?: string;
  className?: string;
}) {
  const [size, setSize] = useState(initial);
  const ref = useRef<HTMLDivElement>(null);
  const horizontal = direction === "horizontal";

  useEffect(() => {
    if (!storageKey) return;
    try {
      const v = Number(localStorage.getItem("quarry.split." + storageKey));
      if (v) setSize(Math.min(max, Math.max(min, v)));
    } catch {}
  }, [storageKey, min, max]);

  const commit = useCallback(
    (v: number) => {
      const clamped = Math.min(max, Math.max(min, v));
      setSize(clamped);
      if (storageKey)
        try {
          localStorage.setItem("quarry.split." + storageKey, String(clamped));
        } catch {}
    },
    [min, max, storageKey],
  );

  const onPointerDown = (e: React.PointerEvent) => {
    e.preventDefault();
    const el = ref.current;
    if (!el) return;
    const rect = el.getBoundingClientRect();
    const move = (ev: PointerEvent) => {
      commit(horizontal ? ev.clientX - rect.left : (ev.clientY - rect.top) / rect.height);
    };
    const up = () => {
      window.removeEventListener("pointermove", move);
      window.removeEventListener("pointerup", up);
      document.body.style.cursor = "";
    };
    document.body.style.cursor = horizontal ? "col-resize" : "row-resize";
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", up);
  };

  const onKeyDown = (e: React.KeyboardEvent) => {
    const step = horizontal ? 16 : 0.04;
    if (e.key === (horizontal ? "ArrowLeft" : "ArrowUp")) commit(size - step);
    else if (e.key === (horizontal ? "ArrowRight" : "ArrowDown")) commit(size + step);
    else return;
    e.preventDefault();
  };

  const firstStyle = horizontal ? { width: size } : { height: `${size * 100}%` };

  return (
    <div ref={ref} className={clsx("flex min-h-0 min-w-0", horizontal ? "flex-row" : "flex-col", className)}>
      <div className="relative flex min-h-0 min-w-0 shrink-0 flex-col" style={firstStyle}>
        {first}
      </div>
      <div
        role="separator"
        aria-orientation={horizontal ? "vertical" : "horizontal"}
        aria-valuenow={Math.round(horizontal ? size : size * 100)}
        tabIndex={0}
        onPointerDown={onPointerDown}
        onKeyDown={onKeyDown}
        className={clsx(
          "group relative z-10 shrink-0 bg-line transition-colors duration-150 hover:bg-accent focus-visible:bg-accent focus-visible:outline-none",
          horizontal ? "w-px cursor-col-resize" : "h-px cursor-row-resize",
        )}
      >
        {/* Wider invisible hit area. */}
        <span className={clsx("absolute", horizontal ? "inset-y-0 -left-1.5 -right-1.5" : "inset-x-0 -top-1.5 -bottom-1.5")} />
      </div>
      <div className="relative flex min-h-0 min-w-0 flex-1 flex-col">{second}</div>
    </div>
  );
}
