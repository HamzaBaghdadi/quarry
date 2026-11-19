"use client";

// App-level building blocks layered on shadcn/ui primitives, so every screen
// uses the same buttons, fields, menus and tooltips (no native controls).

import { forwardRef, useEffect, useState, type ComponentProps, type ReactNode } from "react";
import { create } from "zustand";
import { Button as UIButton } from "@/components/ui/button";
import { Input as UIInput } from "@/components/ui/input";
import { Label as UILabel } from "@/components/ui/label";
import {
  Select as UISelect,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { engineMark, onColor } from "@/lib/engines";
import { cn } from "@/lib/utils";

/* ---------------------------------------------------------------- Button */

type Variant = "primary" | "run" | "ghost" | "subtle" | "danger" | "outline";

const variantClass: Record<Variant, string> = {
  primary: "bg-brand text-brand-fg hover:bg-brand/90 shadow-sm",
  run: "bg-run text-run-fg font-semibold hover:bg-run/90 shadow-sm",
  ghost: "bg-transparent text-fg-muted hover:bg-bg-2 hover:text-fg",
  subtle: "bg-bg-2 text-fg hover:bg-bg-3 border border-line",
  outline: "bg-transparent text-fg border border-line-strong hover:bg-bg-2",
  danger: "bg-transparent text-danger hover:bg-danger-soft",
};

export const Button = forwardRef<
  HTMLButtonElement,
  Omit<ComponentProps<"button">, "ref"> & { variant?: Variant; size?: "sm" | "md" }
>(function Button({ variant = "subtle", size = "md", className, ...props }, ref) {
  return (
    <UIButton
      ref={ref}
      variant="ghost"
      size={size === "sm" ? "sm" : "default"}
      className={cn(
        "cursor-pointer gap-1.5 font-medium transition-colors duration-150",
        size === "sm" ? "h-7 px-2.5 text-[12px]" : "h-8 px-3 text-[13px]",
        variantClass[variant],
        className,
      )}
      {...props}
    />
  );
});

/** Square icon-only button. `label` is the accessible name and the tooltip. */
export function IconButton({
  label,
  className,
  children,
  active,
  side = "bottom",
  ...props
}: Omit<ComponentProps<"button">, "ref"> & { label: string; active?: boolean; side?: "top" | "bottom" | "left" | "right" }) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <UIButton
          aria-label={label}
          variant="ghost"
          size="icon"
          className={cn(
            "size-7 shrink-0 cursor-pointer rounded-md text-fg-muted hover:bg-bg-2 hover:text-fg",
            active && "bg-bg-3 text-fg",
            className,
          )}
          {...props}
        >
          {children}
        </UIButton>
      </TooltipTrigger>
      <TooltipContent side={side}>{label}</TooltipContent>
    </Tooltip>
  );
}

/** Wrap anything in a tooltip. */
export function Tip({ label, children, side = "bottom" }: { label: ReactNode; children: ReactNode; side?: "top" | "bottom" | "left" | "right" }) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>{children}</TooltipTrigger>
      <TooltipContent side={side}>{label}</TooltipContent>
    </Tooltip>
  );
}

/* ----------------------------------------------------------------- Field */

export const Input = forwardRef<HTMLInputElement, ComponentProps<"input">>(function Input({ className, ...props }, ref) {
  return (
    <UIInput
      ref={ref}
      spellCheck={false}
      autoComplete="off"
      className={cn(
        "h-8 rounded-md border-line-strong bg-bg-0 px-2.5 text-[13px] text-fg shadow-none placeholder:text-fg-subtle dark:bg-bg-0",
        "focus-visible:border-brand focus-visible:ring-[3px] focus-visible:ring-brand/25 focus-visible:outline-none",
        className,
      )}
      {...props}
    />
  );
});

export function Label({ children, htmlFor, hint }: { children: ReactNode; htmlFor?: string; hint?: ReactNode }) {
  return (
    <UILabel htmlFor={htmlFor} className="mb-1.5 flex items-baseline justify-between text-[12px] font-medium text-fg-muted">
      <span>{children}</span>
      {hint && <span className="font-normal text-fg-subtle">{hint}</span>}
    </UILabel>
  );
}

/* ---------------------------------------------------------------- Select */

const EMPTY = "\u0000empty"; // Radix Select doesn't allow "" as an item value.

export interface Option {
  value: string;
  label: ReactNode;
  hint?: ReactNode;
}

export function Select({
  value,
  onChange,
  options,
  placeholder,
  id,
  size = "md",
  disabled,
  className,
  "aria-label": ariaLabel,
  icon,
}: {
  value: string;
  onChange: (v: string) => void;
  options: Option[];
  placeholder?: string;
  id?: string;
  size?: "sm" | "md";
  disabled?: boolean;
  className?: string;
  "aria-label"?: string;
  icon?: ReactNode;
}) {
  return (
    <UISelect value={value === "" ? EMPTY : value} onValueChange={(v) => onChange(v === EMPTY ? "" : v)} disabled={disabled}>
      <SelectTrigger
        id={id}
        aria-label={ariaLabel}
        size="sm"
        className={cn(
          "w-full cursor-pointer gap-1.5 rounded-md border-line-strong bg-bg-0 text-fg shadow-none dark:bg-bg-0 dark:hover:bg-bg-2",
          size === "sm" ? "!h-7 px-2 text-[12px]" : "!h-8 px-2.5 text-[13px]",
          "focus-visible:border-brand focus-visible:ring-[3px] focus-visible:ring-brand/25",
          className,
        )}
      >
        {icon}
        <SelectValue placeholder={placeholder} />
      </SelectTrigger>
      <SelectContent className="border-line-strong bg-popover">
        {options.map((o) => (
          <SelectItem key={o.value || EMPTY} value={o.value === "" ? EMPTY : o.value} className="cursor-pointer text-[13px]">
            {o.label}
            {o.hint && <span className="ml-2 text-fg-subtle">{o.hint}</span>}
          </SelectItem>
        ))}
      </SelectContent>
    </UISelect>
  );
}

/* --------------------------------------------------------------- Confirm */

interface ConfirmRequest {
  title: string;
  description?: ReactNode;
  confirmLabel?: string;
  destructive?: boolean;
  resolve: (ok: boolean) => void;
}

const useConfirmStore = create<{ req: ConfirmRequest | null }>(() => ({ req: null }));

/** Promise-based replacement for window.confirm, rendered with AlertDialog. */
export function confirmDialog(opts: Omit<ConfirmRequest, "resolve">): Promise<boolean> {
  return new Promise((resolve) => useConfirmStore.setState({ req: { ...opts, resolve } }));
}

export function ConfirmHost() {
  const req = useConfirmStore((s) => s.req);
  const [open, setOpen] = useState(false);
  useEffect(() => setOpen(!!req), [req]);
  const finish = (ok: boolean) => {
    req?.resolve(ok);
    setOpen(false);
    setTimeout(() => useConfirmStore.setState({ req: null }), 150);
  };
  return (
    <AlertDialog open={open} onOpenChange={(o) => !o && finish(false)}>
      <AlertDialogContent className="max-w-[420px] border-line-strong bg-bg-1 shadow-pop">
        <AlertDialogHeader>
          <AlertDialogTitle className="text-[15px]">{req?.title}</AlertDialogTitle>
          {req?.description && <AlertDialogDescription className="text-[13px] text-fg-muted">{req.description}</AlertDialogDescription>}
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel className="h-8 cursor-pointer text-[13px]" onClick={() => finish(false)}>
            Cancel
          </AlertDialogCancel>
          <AlertDialogAction
            variant={req?.destructive ? "destructive" : "default"}
            className="h-8 cursor-pointer text-[13px]"
            onClick={() => finish(true)}
          >
            {req?.confirmLabel ?? "Continue"}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

/* ------------------------------------------------------------ Small bits */

export function Kbd({ children }: { children: ReactNode }) {
  return <kbd className="rounded border border-line-strong bg-bg-2 px-1 font-mono text-[11px] text-fg-muted">{children}</kbd>;
}

export function Spinner({ className }: { className?: string }) {
  return (
    <span
      role="status"
      aria-label="Loading"
      className={cn("inline-block size-3.5 animate-spin rounded-full border-2 border-current border-r-transparent", className)}
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
