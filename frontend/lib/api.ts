// Typed bridge to the Go backend (app.go). Wails exposes bound methods on
// window.go.main.App; when that's missing (plain browser during UI work) we
// fall back to an in-memory demo backend so the UI is still explorable.

export type Family = "sql" | "document" | "keyvalue";
export type Field = "host" | "port" | "user" | "password" | "database" | "file" | "uri" | "ssl";

export interface DriverInfo {
  id: string;
  name: string;
  family: Family;
  language: "sql" | "mongodb" | "redis";
  defaultPort: number;
  defaultUser: string;
  fields: Field[];
  uriPlaceholder: string;
  sslModes: string[] | null;
  example: string;
  capabilities: ("edit" | "backup")[] | null;
}

export interface ConnConfig {
  host: string;
  port: number;
  user: string;
  password: string;
  database: string;
  file: string;
  uri: string;
  sslMode: string;
  options?: Record<string, string>;
}

export interface Profile {
  id: string;
  name: string;
  driver: string;
  color: string;
  config: ConnConfig;
  readOnly: boolean;
  createdAt?: string;
  updatedAt?: string;
}

export interface Connection extends Profile {
  connected: boolean;
}

export type NodeKind = "database" | "schema" | "table" | "view" | "collection" | "key" | "group";

export interface TreeNode {
  name: string;
  kind: NodeKind;
  path: string[];
  hasChildren: boolean;
  browsable: boolean;
  detail?: string;
}

export interface BrowseRequest {
  path: string[];
  limit: number;
  offset: number;
  filter: string;
  orderBy: string;
  desc: boolean;
}

export interface Column {
  name: string;
  type: string;
}

export type Cell = string | number | boolean | null;

export interface Result {
  columns: Column[] | null;
  rows: Cell[][] | null;
  rowsAffected: number;
  hasMore: boolean;
  message?: string;
  durationMs: number;
  statement?: string;
  documents?: string[] | null;
  keys?: string[] | null;
  editBlocked?: string;
}

export interface RowChange {
  key: Record<string, Cell>;
  values: Record<string, Cell>;
}

export interface ChangeSet {
  path: string[];
  updates: RowChange[];
  inserts: Record<string, Cell>[];
  deletes: Record<string, Cell>[];
  types: Record<string, string>;
}

export interface BackupReport {
  path: string;
  bytes: number;
  stats: { objects: number; rows: number; statements: number };
  durationMs: number;
}

export interface ProgressEvent {
  runId: string;
  message: string;
  done: number;
  total: number;
}

interface Backend {
  Version(): Promise<string>;
  Drivers(): Promise<DriverInfo[]>;
  ListConnections(): Promise<Connection[]>;
  SaveConnection(p: Profile, keepPassword: boolean): Promise<Profile>;
  DeleteConnection(id: string): Promise<void>;
  TestConnection(p: Profile, useStored: boolean): Promise<string>;
  Connect(id: string): Promise<void>;
  Disconnect(id: string): Promise<void>;
  Children(id: string, path: string[]): Promise<TreeNode[]>;
  Browse(id: string, req: BrowseRequest, runId: string): Promise<Result>;
  Execute(id: string, scope: string, query: string, runId: string): Promise<Result>;
  Cancel(runId: string): Promise<boolean>;
  PickFile(): Promise<string>;
  Export(format: "csv" | "json", res: Result, name: string): Promise<string>;
  ApplyChanges(id: string, cs: ChangeSet): Promise<Result>;
  Backup(id: string, scope: string, runId: string): Promise<BackupReport>;
  PickBackupFile(id: string): Promise<string>;
  Restore(id: string, scope: string, path: string, runId: string): Promise<BackupReport>;
}

declare global {
  interface Window {
    go?: { main?: { App?: Backend } };
    runtime?: { EventsOn(name: string, cb: (...data: unknown[]) => void): () => void };
  }
}

let mock: Backend | null = null;

async function backend(): Promise<Backend> {
  if (typeof window !== "undefined" && window.go?.main?.App) return window.go.main.App;
  if (!mock) mock = (await import("./mock")).createMockBackend();
  return mock;
}

export const isDesktop = () => typeof window !== "undefined" && !!window.go?.main?.App;

// Wails rejects with a plain string; normalise to Error.
async function call<K extends keyof Backend>(
  name: K,
  ...args: Parameters<Backend[K]>
): Promise<Awaited<ReturnType<Backend[K]>>> {
  const b = await backend();
  try {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    return await (b[name] as any)(...args);
  } catch (e) {
    throw e instanceof Error ? e : new Error(String(e));
  }
}

export const api = {
  version: () => call("Version"),
  drivers: () => call("Drivers"),
  listConnections: () => call("ListConnections"),
  saveConnection: (p: Profile, keepPassword: boolean) => call("SaveConnection", p, keepPassword),
  deleteConnection: (id: string) => call("DeleteConnection", id),
  testConnection: (p: Profile, useStored: boolean) => call("TestConnection", p, useStored),
  connect: (id: string) => call("Connect", id),
  disconnect: (id: string) => call("Disconnect", id),
  children: (id: string, path: string[]) => call("Children", id, path),
  browse: (id: string, req: BrowseRequest, runId: string) => call("Browse", id, req, runId),
  execute: (id: string, scope: string, query: string, runId: string) => call("Execute", id, scope, query, runId),
  cancel: (runId: string) => call("Cancel", runId),
  pickFile: () => call("PickFile"),
  exportResult: (format: "csv" | "json", res: Result, name: string) => call("Export", format, res, name),
  applyChanges: (id: string, cs: ChangeSet) => call("ApplyChanges", id, cs),
  backup: (id: string, scope: string, runId: string) => call("Backup", id, scope, runId),
  pickBackupFile: (id: string) => call("PickBackupFile", id),
  restore: (id: string, scope: string, path: string, runId: string) => call("Restore", id, scope, path, runId),
};

export const emptyConfig = (): ConnConfig => ({
  host: "",
  port: 0,
  user: "",
  password: "",
  database: "",
  file: "",
  uri: "",
  sslMode: "",
});

export const newRunId = () => Math.random().toString(36).slice(2);

/** Subscribe to backup/restore progress. Returns an unsubscribe function. */
export function onProgress(cb: (e: ProgressEvent) => void): () => void {
  if (typeof window === "undefined") return () => {};
  if (window.runtime?.EventsOn) return window.runtime.EventsOn("progress", (e) => cb(e as ProgressEvent));
  const h = (e: Event) => cb((e as CustomEvent<ProgressEvent>).detail);
  window.addEventListener("quarry:progress", h);
  return () => window.removeEventListener("quarry:progress", h);
}
