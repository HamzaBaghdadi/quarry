"use client";

import { create } from "zustand";
import { toast } from "sonner";
import { api, type Connection, type DriverInfo, type Profile, type TreeNode } from "./api";

export type TabKind = "query" | "table";

export interface Tab {
  id: string;
  connId: string;
  kind: TabKind;
  title: string;
  /** Explorer path for table tabs. */
  path?: string[];
  /** Database the tab runs against ("" = connection default). */
  scope: string;
  /** Initial editor text for query tabs. */
  query?: string;
}

interface TreeEntry {
  nodes?: TreeNode[];
  loading?: boolean;
  error?: string;
}

export interface Inspected {
  column: string;
  type: string;
  value: unknown;
  /** Present when the value is editable; receives the new text (null = NULL). */
  onSave?: (text: string | null) => void;
}

interface State {
  ready: boolean;
  drivers: DriverInfo[];
  connections: Connection[];
  activeId: string | null;
  connecting: string | null;
  tree: Record<string, TreeEntry>; // key: connId + "\u0000" + path
  expanded: Record<string, boolean>;
  tabs: Tab[];
  activeTab: string | null;
  dialog: { open: boolean; profile?: Profile };
  palette: boolean;
  backup: { connId: string; scope: string; mode: "backup" | "restore" } | null;
  inspected: Inspected | null;
  theme: "dark" | "light";

  init(): Promise<void>;
  refreshConnections(): Promise<void>;
  openDialog(profile?: Profile): void;
  closeDialog(): void;
  setPalette(open: boolean): void;
  openBackup(connId: string, mode: "backup" | "restore", scope?: string): void;
  closeBackup(): void;
  connect(id: string): Promise<void>;
  disconnect(id: string): Promise<void>;
  removeConnection(id: string): Promise<void>;
  loadChildren(connId: string, path: string[], force?: boolean): Promise<void>;
  toggle(connId: string, node: TreeNode): void;
  openTable(connId: string, node: TreeNode): void;
  newQuery(connId?: string, scope?: string, query?: string): void;
  closeTab(id: string): void;
  setActiveTab(id: string): void;
  updateTab(id: string, patch: Partial<Tab>): void;
  inspect(v: Inspected | null): void;
  toggleTheme(): void;
}

export const treeKey = (connId: string, path: string[]) => connId + "\u0000" + path.join("\u0000");

const uid = () => Math.random().toString(36).slice(2, 10);

function readTheme(): "dark" | "light" {
  try {
    return localStorage.getItem("quarry.theme") === "light" ? "light" : "dark";
  } catch {
    return "dark";
  }
}

export const useStore = create<State>((set, get) => ({
  ready: false,
  drivers: [],
  connections: [],
  activeId: null,
  connecting: null,
  tree: {},
  expanded: {},
  tabs: [],
  activeTab: null,
  dialog: { open: false },
  palette: false,
  backup: null,
  inspected: null,
  theme: "dark",

  async init() {
    const theme = readTheme();
    document.documentElement.dataset.theme = theme;
    try {
      const [drivers, connections] = await Promise.all([api.drivers(), api.listConnections()]);
      set({ drivers, connections, ready: true, theme });
    } catch (e) {
      set({ ready: true, theme });
      toast.error("Couldn't load connections", { description: (e as Error).message });
    }
  },

  async refreshConnections() {
    set({ connections: await api.listConnections() });
  },

  openDialog: (profile) => set({ dialog: { open: true, profile } }),
  closeDialog: () => set({ dialog: { open: false } }),
  setPalette: (palette) => set({ palette }),
  openBackup: (connId, mode, scope = "") => set({ backup: { connId, mode, scope } }),
  closeBackup: () => set({ backup: null }),

  async connect(id) {
    const conn = get().connections.find((c) => c.id === id);
    if (!conn) return;
    if (conn.connected) {
      set({ activeId: id });
      return;
    }
    set({ connecting: id });
    try {
      await api.connect(id);
      set((s) => ({
        activeId: id,
        connections: s.connections.map((c) => (c.id === id ? { ...c, connected: true } : c)),
      }));
      await get().loadChildren(id, []);
      // Land the user somewhere useful: open a query tab if none exists.
      if (!get().tabs.some((t) => t.connId === id)) get().newQuery(id);
    } catch (e) {
      toast.error(`Couldn't connect to ${conn.name}`, { description: (e as Error).message, duration: 8000 });
    } finally {
      set({ connecting: null });
    }
  },

  async disconnect(id) {
    await api.disconnect(id);
    set((s) => {
      const tabs = s.tabs.filter((t) => t.connId !== id);
      const tree = Object.fromEntries(Object.entries(s.tree).filter(([k]) => !k.startsWith(id + "\u0000")));
      return {
        connections: s.connections.map((c) => (c.id === id ? { ...c, connected: false } : c)),
        tabs,
        tree,
        activeTab: tabs.some((t) => t.id === s.activeTab) ? s.activeTab : tabs.at(-1)?.id ?? null,
        activeId: s.activeId === id ? s.connections.find((c) => c.connected && c.id !== id)?.id ?? null : s.activeId,
      };
    });
  },

  async removeConnection(id) {
    await get().disconnect(id);
    await api.deleteConnection(id);
    await get().refreshConnections();
  },

  async loadChildren(connId, path, force) {
    const key = treeKey(connId, path);
    const cur = get().tree[key];
    if (cur?.loading || (cur?.nodes && !force)) return;
    set((s) => ({ tree: { ...s.tree, [key]: { ...cur, loading: true, error: undefined } } }));
    try {
      const nodes = await api.children(connId, path);
      set((s) => ({ tree: { ...s.tree, [key]: { nodes } } }));
    } catch (e) {
      set((s) => ({ tree: { ...s.tree, [key]: { error: (e as Error).message } } }));
    }
  },

  toggle(connId, node) {
    const key = treeKey(connId, node.path);
    const open = !get().expanded[key];
    set((s) => ({ expanded: { ...s.expanded, [key]: open } }));
    if (open) void get().loadChildren(connId, node.path);
  },

  openTable(connId, node) {
    const existing = get().tabs.find((t) => t.kind === "table" && t.connId === connId && t.path?.join("/") === node.path.join("/"));
    if (existing) {
      set({ activeTab: existing.id });
      return;
    }
    const tab: Tab = { id: uid(), connId, kind: "table", title: node.name, path: node.path, scope: node.path[0] ?? "" };
    set((s) => ({ tabs: [...s.tabs, tab], activeTab: tab.id }));
  },

  newQuery(connId, scope, query) {
    const id = connId ?? get().activeId;
    if (!id) {
      toast("Connect to a database first");
      return;
    }
    const conn = get().connections.find((c) => c.id === id);
    const drv = get().drivers.find((d) => d.id === conn?.driver);
    const n = get().tabs.filter((t) => t.kind === "query" && t.connId === id).length + 1;
    const tab: Tab = {
      id: uid(),
      connId: id,
      kind: "query",
      title: `Query ${n}`,
      scope: scope ?? conn?.config.database ?? "",
      query: query ?? drv?.example ?? "",
    };
    set((s) => ({ tabs: [...s.tabs, tab], activeTab: tab.id, activeId: id }));
  },

  closeTab(id) {
    set((s) => {
      const i = s.tabs.findIndex((t) => t.id === id);
      const tabs = s.tabs.filter((t) => t.id !== id);
      let activeTab = s.activeTab;
      if (activeTab === id) activeTab = tabs[Math.min(i, tabs.length - 1)]?.id ?? null;
      return { tabs, activeTab };
    });
  },

  setActiveTab(id) {
    const tab = get().tabs.find((t) => t.id === id);
    set({ activeTab: id, activeId: tab?.connId ?? get().activeId });
  },

  updateTab(id, patch) {
    set((s) => ({ tabs: s.tabs.map((t) => (t.id === id ? { ...t, ...patch } : t)) }));
  },

  inspect: (inspected) => set({ inspected }),

  toggleTheme() {
    const theme = get().theme === "dark" ? "light" : "dark";
    document.documentElement.dataset.theme = theme;
    try {
      localStorage.setItem("quarry.theme", theme);
    } catch {}
    set({ theme });
  },
}));

export const useDriver = (driverId?: string) => useStore((s) => s.drivers.find((d) => d.id === driverId));
