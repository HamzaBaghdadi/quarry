"use client";

import clsx from "clsx";
import { useEffect, useState } from "react";
import { ArrowCircleUp, CaretRight, MagnifyingGlass, Moon, Sun } from "@phosphor-icons/react";
import { getPlatform, win, type Platform } from "@/lib/platform";
import { useStore } from "@/lib/store";
import { useUpdater } from "@/lib/updater";
import { IconButton, Kbd, mod } from "./common";

/**
 * Custom title bar. The whole bar drags the window (double-click maximises);
 * interactive pieces opt out with `no-drag`. Windows/Linux get our own
 * minimise / maximise / close buttons; macOS keeps its traffic lights.
 */
export function TitleBar() {
  const [platform, setPlatform] = useState<Platform>("browser");
  const [maxed, setMaxed] = useState(false);
  const theme = useStore((s) => s.theme);
  const conn = useStore((s) => s.connections.find((c) => c.id === s.activeId));
  const tab = useStore((s) => s.tabs.find((t) => t.id === s.activeTab));
  const { toggleTheme, setPalette } = useStore.getState();
  const update = useUpdater((s) => s.available);
  const openUpdate = useUpdater((s) => s.openDialog);

  useEffect(() => {
    void getPlatform().then(setPlatform);
    const sync = () => void win.isMaximised().then(setMaxed);
    sync();
    window.addEventListener("resize", sync);
    return () => window.removeEventListener("resize", sync);
  }, []);

  const mac = platform === "darwin";
  const crumbs = [conn?.name, tab?.kind === "table" ? tab.path?.slice(0, -1).join(".") : tab?.scope, tab?.title].filter(Boolean) as string[];

  return (
    <header
      className="drag relative flex h-10 shrink-0 select-none items-center border-b border-line bg-bg-0"
      onDoubleClick={(e) => {
        if ((e.target as HTMLElement).closest(".no-drag")) return;
        win.toggleMaximise();
      }}
    >
      {/* Left: app identity + where you are. */}
      <div className={clsx("flex min-w-0 flex-1 items-center gap-2", mac ? "pl-[78px]" : "pl-3")}>
        <svg viewBox="2 1 28 30" className="size-5 shrink-0" aria-hidden>
          <path d="M16 3 28 10 16 17 4 10Z" fill="#60a5fa" />
          <path d="M16 17v12L4 22V10Z" fill="#2563eb" />
          <path d="M16 17 28 10v12L16 29Z" fill="#1d4ed8" />
          <path d="M4 14.1 16 21.1 28 14.1M4 18.1 16 25.1 28 18.1" stroke="var(--bg-0)" strokeWidth="1.1" fill="none" />
        </svg>
        <span className="text-[13px] font-semibold tracking-tight text-fg">Quarry</span>
        {crumbs.length > 0 && (
          <nav aria-label="Location" className="flex min-w-0 items-center gap-1 text-[12.5px] text-fg-subtle">
            {crumbs.map((c, i) => (
              <span key={i} className="flex min-w-0 items-center gap-1">
                <CaretRight aria-hidden className="size-3 shrink-0 opacity-60" />
                <span className={clsx("truncate", i === crumbs.length - 1 && "text-fg-muted")}>{c}</span>
              </span>
            ))}
          </nav>
        )}
      </div>

      {/* Centre: search / command palette. */}
      <button
        onClick={() => setPalette(true)}
        className="no-drag absolute left-1/2 hidden h-7 w-[min(400px,34vw)] -translate-x-1/2 cursor-pointer items-center gap-2 rounded-md border border-line bg-bg-1 px-2.5 text-[12px] text-fg-subtle transition-colors duration-150 hover:border-line-strong hover:text-fg-muted md:flex"
      >
        <MagnifyingGlass aria-hidden className="size-3.5" />
        <span className="flex-1 text-left">Search connections, tables, actions</span>
        <span className="flex gap-0.5">
          <Kbd>{mod()}</Kbd>
          <Kbd>K</Kbd>
        </span>
      </button>

      {/* Right: status & window controls. */}
      <div className="no-drag flex h-full items-center gap-1 pl-2">
        {update && (
          <button
            onClick={openUpdate}
            className="mr-1.5 flex h-7 cursor-pointer items-center gap-1.5 rounded-full bg-brand-soft px-3 text-[12px] font-medium text-brand-text transition-colors duration-150 hover:bg-brand hover:text-brand-fg"
          >
            <ArrowCircleUp aria-hidden weight="fill" className="size-4" />
            Update {update.version}
          </button>
        )}
        <IconButton label={theme === "dark" ? "Light theme" : "Dark theme"} onClick={toggleTheme} className="size-8">
          {theme === "dark" ? <Sun className="size-[18px]" /> : <Moon className="size-[18px]" />}
        </IconButton>
        {!mac && (
          <div className="ml-1 flex h-full">
            <WinButton label="Minimise" onClick={win.minimise}>
              <path d="M2 6h8" />
            </WinButton>
            <WinButton label={maxed ? "Restore" : "Maximise"} onClick={win.toggleMaximise}>
              {maxed ? <path d="M3.5 4.5h5v5h-5zM4.5 4.5V2.5h5v5h-1" /> : <path d="M2.5 2.5h7v7h-7z" />}
            </WinButton>
            <WinButton label="Close" onClick={win.close} danger>
              <path d="M2.5 2.5l7 7M9.5 2.5l-7 7" />
            </WinButton>
          </div>
        )}
      </div>
    </header>
  );
}

function WinButton({ label, onClick, danger, children }: { label: string; onClick: () => void; danger?: boolean; children: React.ReactNode }) {
  return (
    <button
      aria-label={label}
      onClick={onClick}
      className={clsx(
        "flex h-full w-12 cursor-default items-center justify-center text-fg transition-colors duration-100",
        danger ? "hover:bg-[#e81123] hover:text-white" : "hover:bg-bg-3 hover:text-fg",
      )}
    >
      <svg viewBox="0 0 12 12" className="size-[14px]" fill="none" stroke="currentColor" strokeWidth="1.15" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
        {children}
      </svg>
    </button>
  );
}
