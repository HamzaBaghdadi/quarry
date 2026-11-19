"use client";

import clsx from "clsx";
import { forwardRef, type ButtonHTMLAttributes, type InputHTMLAttributes, type ReactNode, type SelectHTMLAttributes } from "react";
import { engineMark, onColor } from "@/lib/engines";

type Variant = "primary" | "run" | "ghost" | "subtle" | "danger";

const variants: Record<Variant, string> = {
  primary: "bg-accent text-accent-fg hover:brightness-110",
  run: "bg-run text-run-fg hover:brightness-110 font-semibold",
  ghost: "text-fg-muted hover:text-fg hover:bg-bg-2",
  subtle: "bg-bg-2 text-fg hover:bg-bg-3 border border-line",
  danger: "text-danger hover:bg-danger-soft",
};

export const Button = forwardRef<HTMLButtonElement, ButtonHTMLAttributes<HTMLButtonElement> & { variant?: Variant; size?: "sm" | "md" }>(
  function Button({ variant = "subtle", size = "md", className, ...props }, ref) {
    return (
      <button
        ref={ref}
        className={clsx(
          "inline-flex select-none items-center justify-center gap-1.5 whitespace-nowrap rounded-md font-medium transition-[background-color,filter,color] duration-150 disabled:pointer-events-none disabled:opacity-45 cursor-pointer",
          size === "sm" ? "h-7 px-2.5 text-[12px]" : "h-8 px-3 text-[13px]",
          variants[variant],
          className,
        )}
        {...props}
      />
    );
  },
);

/** Square icon-only button. `label` is required: it's the accessible name and tooltip. */
export function IconButton({ label, className, children, active, ...props }: ButtonHTMLAttributes<HTMLButtonElement> & { label: string; active?: boolean }) {
  return (
    <button
      aria-label={label}
      title={label}
      className={clsx(
        "inline-flex size-7 shrink-0 items-center justify-center rounded-md text-fg-muted transition-colors duration-150 hover:bg-bg-2 hover:text-fg disabled:pointer-events-none disabled:opacity-40 cursor-pointer",
        active && "bg-bg-3 text-fg",
        className,
      )}
      {...props}
    >
      {children}
    </button>
  );
}

export const Input = forwardRef<HTMLInputElement, InputHTMLAttributes<HTMLInputElement>>(function Input({ className, ...props }, ref) {
  return (
    <input
      ref={ref}
      spellCheck={false}
      autoComplete="off"
      className={clsx(
        "h-8 w-full rounded-md border border-line bg-bg-0 px-2.5 text-[13px] text-fg placeholder:text-fg-subtle transition-colors duration-150 hover:border-line-strong focus:border-accent focus:outline-none focus-visible:outline-none focus:ring-2 focus:ring-accent-soft",
        className,
      )}
      {...props}
    />
  );
});

export function Select({ className, children, ...props }: SelectHTMLAttributes<HTMLSelectElement>) {
  return (
    <select
      className={clsx(
        "h-8 w-full cursor-pointer rounded-md border border-line bg-bg-0 px-2 text-[13px] text-fg transition-colors duration-150 hover:border-line-strong focus:border-accent focus:outline-none focus:ring-2 focus:ring-accent-soft",
        className,
      )}
      {...props}
    >
      {children}
    </select>
  );
}

export function Label({ children, htmlFor, hint }: { children: ReactNode; htmlFor?: string; hint?: ReactNode }) {
  return (
    <label htmlFor={htmlFor} className="mb-1 flex items-baseline justify-between text-[12px] font-medium text-fg-muted">
      <span>{children}</span>
      {hint && <span className="font-normal text-fg-subtle">{hint}</span>}
    </label>
  );
}

export function Kbd({ children }: { children: ReactNode }) {
  return <kbd className="rounded border border-line-strong bg-bg-2 px-1 font-mono text-[11px] text-fg-muted">{children}</kbd>;
}

export function Spinner({ className }: { className?: string }) {
  return (
    <span
      role="status"
      aria-label="Loading"
      className={clsx("inline-block size-3.5 animate-spin rounded-full border-2 border-current border-r-transparent", className)}
    />
  );
}

/**
 * Engine logo on a brand-coloured tile. `color` (the connection's own colour)
 * is shown as a small tag in the corner so saved connections stay distinguishable.
 */
export function EngineBadge({ driver, size = 28, color }: { driver?: string; size?: number; color?: string }) {
  const { mark, hue, path } = engineMark(driver);
  const fg = onColor(hue);
  return (
    <span
      aria-hidden
      className="relative inline-flex shrink-0 items-center justify-center rounded-md font-mono font-semibold"
      style={{
        width: size,
        height: size,
        fontSize: Math.round(size * 0.4),
        color: fg,
        background: hue,
        boxShadow: "inset 0 0 0 1px rgb(255 255 255 / 0.12)",
      }}
    >
      {path ? (
        <svg viewBox="0 0 24 24" width={Math.round(size * 0.62)} height={Math.round(size * 0.62)} fill="currentColor">
          <path d={path} />
        </svg>
      ) : (
        mark
      )}
      {color && (
        <span
          className="absolute -bottom-0.5 -left-0.5 rounded-full border-2 border-bg-0"
          style={{ width: Math.max(8, size * 0.32), height: Math.max(8, size * 0.32), background: color }}
        />
      )}
    </span>
  );
}

export const isMac = () => typeof navigator !== "undefined" && /Mac/.test(navigator.platform);
export const mod = () => (isMac() ? "⌘" : "Ctrl");
