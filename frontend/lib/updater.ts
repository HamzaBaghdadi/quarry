"use client";

import { create } from "zustand";
import { api, isDesktop, type UpdateInfo } from "./api";

type Status = "idle" | "checking" | "downloading" | "ready" | "handoff" | "error";

interface UpdaterState {
  available: UpdateInfo | null;
  status: Status;
  progress: { done: number; total: number };
  error: string | null;
  dialog: boolean;
  lastChecked: number;
  check(manual?: boolean): Promise<void>;
  install(): Promise<void>;
  restart(): Promise<void>;
  openDialog(): void;
  closeDialog(): void;
}

const SKIP_KEY = "quarry.update.skip";

export const useUpdater = create<UpdaterState>((set, get) => ({
  available: null,
  status: "idle",
  progress: { done: 0, total: 0 },
  error: null,
  dialog: false,
  lastChecked: 0,

  async check(manual = false) {
    if (get().status === "checking" || get().status === "downloading") return;
    set({ status: "checking", error: null });
    try {
      const rel = await api.checkForUpdate();
      let skipped = "";
      try {
        skipped = localStorage.getItem(SKIP_KEY) ?? "";
      } catch {}
      const show = rel && (manual || rel.version !== skipped) ? rel : null;
      set({ available: show, status: "idle", lastChecked: Date.now(), dialog: manual && !!show ? true : get().dialog });
    } catch (e) {
      set({ status: manual ? "error" : "idle", error: (e as Error).message });
    }
  },

  async install() {
    set({ status: "downloading", progress: { done: 0, total: get().available?.size ?? 0 }, error: null });
    try {
      const mode = await api.installUpdate();
      if (mode === "restart") set({ status: "ready" });
      else if (mode === "package") set({ status: "handoff" });
    } catch (e) {
      set({ status: "error", error: (e as Error).message });
    }
  },

  async restart() {
    await api.restartApp();
  },

  openDialog: () => set({ dialog: true }),
  closeDialog: () => set({ dialog: get().status === "downloading" }),
}));

export function skipVersion(v: string) {
  try {
    localStorage.setItem(SKIP_KEY, v);
  } catch {}
  useUpdater.setState({ available: null, dialog: false });
}

/** Start background checks: shortly after launch, then every 6 hours. */
export function startUpdateChecks() {
  const onProgress = (d: { done: number; total: number }) => useUpdater.setState({ progress: d });
  let off: (() => void) | undefined;
  const rt = (window as unknown as { runtime?: { EventsOn(n: string, cb: (d: unknown) => void): () => void } }).runtime;
  if (rt?.EventsOn) off = rt.EventsOn("update:progress", (d) => onProgress(d as { done: number; total: number }));
  else {
    const h = (e: Event) => onProgress((e as CustomEvent).detail);
    window.addEventListener("quarry:update-progress", h);
    off = () => window.removeEventListener("quarry:update-progress", h);
  }
  const first = setTimeout(() => void useUpdater.getState().check(), isDesktop() ? 4000 : 1500);
  const every = setInterval(() => void useUpdater.getState().check(), 6 * 60 * 60 * 1000);
  return () => {
    clearTimeout(first);
    clearInterval(every);
    off?.();
  };
}
