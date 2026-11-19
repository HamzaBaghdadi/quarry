// In-memory demo backend used when the UI runs outside the desktop shell
// (e.g. `pnpm dev` in a browser). It mimics the Go API closely enough to
// design and test every screen without a database.
import type { BackupReport, Cell, ChangeSet, Connection, DriverInfo, Profile, Result, TreeNode } from "./api";

const all = ["host", "port", "user", "password", "database", "ssl", "uri"] as const;
const rawDrivers: Omit<DriverInfo, "capabilities">[] = [
  { id: "clickhouse", name: "ClickHouse", family: "sql", language: "sql", defaultPort: 9000, defaultUser: "default", fields: [...all], uriPlaceholder: "clickhouse://default:@localhost:9000/default", sslModes: ["false", "true"], example: "SELECT version();" },
  { id: "cockroachdb", name: "CockroachDB", family: "sql", language: "sql", defaultPort: 26257, defaultUser: "root", fields: [...all], uriPlaceholder: "postgresql://root@localhost:26257/defaultdb", sslModes: ["disable", "require"], example: "SHOW DATABASES;" },
  { id: "mariadb", name: "MariaDB", family: "sql", language: "sql", defaultPort: 3306, defaultUser: "root", fields: [...all], uriPlaceholder: "user:pass@tcp(localhost:3306)/db", sslModes: ["false", "true"], example: "SELECT VERSION();" },
  { id: "mongodb", name: "MongoDB", family: "document", language: "mongodb", defaultPort: 27017, defaultUser: "", fields: [...all], uriPlaceholder: "mongodb+srv://user:pass@cluster.example.net/app", sslModes: ["false", "true"], example: 'db.getCollection("users").find({}).limit(20)' },
  { id: "mysql", name: "MySQL", family: "sql", language: "sql", defaultPort: 3306, defaultUser: "root", fields: [...all], uriPlaceholder: "user:pass@tcp(localhost:3306)/db", sslModes: ["false", "true"], example: "SELECT VERSION();" },
  { id: "postgres", name: "PostgreSQL", family: "sql", language: "sql", defaultPort: 5432, defaultUser: "postgres", fields: [...all], uriPlaceholder: "postgres://user:pass@localhost:5432/db", sslModes: ["prefer", "disable", "require", "verify-full"], example: "SELECT now(), version();" },
  { id: "redis", name: "Redis", family: "keyvalue", language: "redis", defaultPort: 6379, defaultUser: "", fields: [...all], uriPlaceholder: "redis://:password@localhost:6379/0", sslModes: ["false", "true"], example: "INFO server" },
  { id: "mssql", name: "SQL Server", family: "sql", language: "sql", defaultPort: 1433, defaultUser: "sa", fields: [...all], uriPlaceholder: "sqlserver://sa:pass@localhost:1433", sslModes: ["false", "true"], example: "SELECT @@VERSION;" },
  { id: "sqlite", name: "SQLite", family: "sql", language: "sql", defaultPort: 0, defaultUser: "", fields: ["file"], uriPlaceholder: "file:app.db", sslModes: null, example: "SELECT sqlite_version();" },
  { id: "tidb", name: "TiDB", family: "sql", language: "sql", defaultPort: 4000, defaultUser: "root", fields: [...all], uriPlaceholder: "user:pass@tcp(localhost:4000)/db", sslModes: ["false", "true"], example: "SELECT tidb_version();" },
  { id: "valkey", name: "Valkey", family: "keyvalue", language: "redis", defaultPort: 6379, defaultUser: "", fields: [...all], uriPlaceholder: "redis://localhost:6379/0", sslModes: ["false", "true"], example: "INFO server" },
];
const noBackup = new Set(["clickhouse", "mssql", "cockroachdb"]);
const drivers: DriverInfo[] = rawDrivers.map((d) => ({
  ...d,
  capabilities: d.id === "clickhouse" ? [] : noBackup.has(d.id) ? ["edit"] : ["edit", "backup"],
}));

const firstNames = ["Ada", "Linus", "Grace", "Ken", "Barbara", "Dennis", "Margaret", "Guido", "Anders", "Radia", "Edsger", "Frances"];
const cities = ["Amman", "Berlin", "Lagos", "Lisbon", "Osaka", "Toronto", "Austin", "Cairo"];

function users(n: number): Cell[][] {
  return Array.from({ length: n }, (_, i) => [
    i + 1,
    `${firstNames[i % firstNames.length]} ${String.fromCharCode(65 + (i % 26))}.`,
    `${firstNames[i % firstNames.length].toLowerCase()}${i}@example.com`,
    cities[i % cities.length],
    i % 7 === 0 ? null : (i * 37) % 100,
    i % 3 !== 0,
    JSON.stringify({ plan: ["free", "pro", "team"][i % 3], seats: (i % 5) + 1 }),
    new Date(Date.UTC(2025, i % 12, (i % 27) + 1, 9, i % 60)).toISOString(),
  ]);
}

const tables: Record<string, { columns: { name: string; type: string }[]; rows: Cell[][] }> = {
  users: {
    columns: [
      { name: "id", type: "int8" }, { name: "name", type: "text" }, { name: "email", type: "text" },
      { name: "city", type: "text" }, { name: "score", type: "int4" }, { name: "active", type: "bool" },
      { name: "meta", type: "jsonb" }, { name: "created_at", type: "timestamptz" },
    ],
    rows: users(1200),
  },
  orders: {
    columns: [{ name: "id", type: "int8" }, { name: "user_id", type: "int8" }, { name: "total", type: "numeric" }, { name: "status", type: "text" }],
    rows: Array.from({ length: 340 }, (_, i) => [i + 1, (i * 7) % 1200, ((i * 1337) % 50000) / 100, ["paid", "pending", "refunded"][i % 3]]),
  },
};

const profiles: Connection[] = [
  { id: "demo-pg", name: "Local Postgres", driver: "postgres", color: "#3B82F6", readOnly: false, connected: false,
    config: { host: "localhost", port: 5432, user: "postgres", password: "", database: "app", file: "", uri: "", sslMode: "disable" } },
  { id: "demo-mongo", name: "Analytics Mongo", driver: "mongodb", color: "#22C55E", readOnly: true, connected: false,
    config: { host: "localhost", port: 27017, user: "", password: "", database: "analytics", file: "", uri: "", sslMode: "" } },
  { id: "demo-redis", name: "Cache", driver: "redis", color: "#EF4444", readOnly: false, connected: false,
    config: { host: "localhost", port: 6379, user: "", password: "", database: "0", file: "", uri: "", sslMode: "" } },
];

const wait = (ms: number) => new Promise((r) => setTimeout(r, ms));

function page(name: string, offset: number, limit: number, filter: string, orderBy: string, desc: boolean, search = ""): Result {
  const t = tables[name] ?? tables.users;
  let rows = t.rows;
  if (filter) {
    const needle = filter.replace(/.*=\s*'?|'$/g, "").toLowerCase();
    rows = rows.filter((r) => r.some((c) => String(c).toLowerCase().includes(needle)));
  }
  if (search) {
    const s = search.toLowerCase();
    rows = rows.filter((r) => r.some((c) => c !== null && String(c).toLowerCase().includes(s)));
  }
  if (orderBy) {
    const i = t.columns.findIndex((c) => c.name === orderBy);
    rows = [...rows].sort((a, b) => (String(a[i]) > String(b[i]) ? 1 : -1) * (desc ? -1 : 1));
  }
  return {
    columns: t.columns, rows: rows.slice(offset, offset + limit), rowsAffected: 0,
    hasMore: offset + limit < rows.length, durationMs: 3 + Math.random() * 8,
    statement: `SELECT * FROM "public"."${name}"${filter ? ` WHERE ${filter}` : ""} LIMIT ${limit} OFFSET ${offset}`,
  };
}

export function createMockBackend() {
  return {
    Version: async () => "0.1.0-demo",
    Drivers: async () => drivers,
    ListConnections: async () => profiles.map((p) => ({ ...p })),
    SaveConnection: async (p: Profile) => {
      const saved = { ...p, id: p.id || crypto.randomUUID() };
      const i = profiles.findIndex((x) => x.id === saved.id);
      if (i >= 0) profiles[i] = { ...saved, connected: profiles[i].connected };
      else profiles.push({ ...saved, connected: false });
      return saved;
    },
    DeleteConnection: async (id: string) => {
      const i = profiles.findIndex((x) => x.id === id);
      if (i >= 0) profiles.splice(i, 1);
    },
    TestConnection: async () => {
      await wait(400);
      return "Connected in 12 ms (demo)";
    },
    Connect: async (id: string) => {
      await wait(250);
      const p = profiles.find((x) => x.id === id);
      if (p) p.connected = true;
    },
    Disconnect: async (id: string) => {
      const p = profiles.find((x) => x.id === id);
      if (p) p.connected = false;
    },
    Children: async (id: string, path: string[]): Promise<TreeNode[]> => {
      await wait(120);
      const p = profiles.find((x) => x.id === id);
      if (p?.driver === "redis") {
        if (!path.length) return [0, 1, 2].map((i) => ({ name: `db${i}`, kind: "database", path: [`db${i}`], hasChildren: i === 0, browsable: false, detail: i === 0 ? "4 keys" : undefined }));
        return ["session:42", "user:1", "queue:emails", "leaderboard"].map((k, i) => ({ name: k, kind: "key", path: [path[0], k], hasChildren: false, browsable: true, detail: ["string", "hash", "list", "zset"][i] }));
      }
      if (p?.driver === "mongodb") {
        if (!path.length) return ["analytics", "admin"].map((d) => ({ name: d, kind: "database", path: [d], hasChildren: true, browsable: false }));
        return ["events", "users", "daily_rollup"].map((c) => ({ name: c, kind: c === "daily_rollup" ? "view" : "collection", path: [path[0], c], hasChildren: false, browsable: true }));
      }
      if (!path.length) return ["app", "postgres"].map((d) => ({ name: d, kind: "database", path: [d], hasChildren: true, browsable: false }));
      if (path.length === 1) return ["public", "audit"].map((s) => ({ name: s, kind: "schema", path: [path[0], s], hasChildren: true, browsable: false }));
      return [
        { name: "orders", kind: "table", path: [...path, "orders"], hasChildren: false, browsable: true },
        { name: "users", kind: "table", path: [...path, "users"], hasChildren: false, browsable: true },
        { name: "active_users", kind: "view", path: [...path, "active_users"], hasChildren: false, browsable: true },
      ];
    },
    Browse: async (_id: string, req: { path: string[]; offset: number; limit: number; filter: string; search?: string; orderBy: string; desc: boolean }) => {
      await wait(150);
      const name = req.path[req.path.length - 1];
      const r = page(name === "orders" ? "orders" : "users", req.offset, req.limit, req.filter, req.orderBy, req.desc, req.search);
      if (name === "active_users") return { ...r, editBlocked: "Views can't be edited." };
      const conn = profiles.find((p) => p.id === _id);
      if (conn?.readOnly) return { ...r, editBlocked: "This connection is read-only." };
      return { ...r, keys: ["id"] };
    },
    Execute: async (_id: string, _scope: string, query: string): Promise<Result> => {
      await wait(300);
      const q = query.trim().toLowerCase();
      if (q.includes("error")) throw new Error('syntax error at or near "error"');
      if (!q.startsWith("select") && !q.startsWith("db.") && !q.startsWith("with")) {
        return { columns: null, rows: null, rowsAffected: 3, hasMore: false, durationMs: 4.2, message: "3 row(s) affected" };
      }
      return page(q.includes("orders") ? "orders" : "users", 0, 500, "", "", false);
    },
    Cancel: async () => true,
    PickFile: async () => "C:\\data\\demo.sqlite",
    Export: async (_f: string, _r: Result, name: string) => `~/Downloads/${name}.csv`,
    ApplyChanges: async (_id: string, cs: ChangeSet): Promise<Result> => {
      await wait(250);
      const t = tables[cs.path.at(-1) === "orders" ? "orders" : "users"];
      const col = (n: string) => t.columns.findIndex((c) => c.name === n);
      const find = (key: Record<string, Cell>) => t.rows.findIndex((r) => String(r[0]) === String(key.id));
      for (const d of cs.deletes) {
        const i = find(d);
        if (i < 0) throw new Error(`Row ${d.id} not found — nothing was saved`);
        t.rows.splice(i, 1);
      }
      for (const u of cs.updates) {
        const i = find(u.key);
        if (i < 0) throw new Error(`Row ${u.key.id} not found — nothing was saved`);
        for (const [k, v] of Object.entries(u.values)) t.rows[i][col(k)] = v;
      }
      for (const ins of cs.inserts) {
        const row: Cell[] = t.columns.map(() => null);
        row[0] = Math.max(...t.rows.map((r) => Number(r[0]))) + 1;
        for (const [k, v] of Object.entries(ins)) row[col(k)] = v;
        t.rows.push(row);
      }
      const n = cs.updates.length + cs.inserts.length + cs.deletes.length;
      return { columns: null, rows: null, rowsAffected: n, hasMore: false, durationMs: 6, message: `Saved ${n} change(s)` };
    },
    Backup: async (_id: string, scope: string, runId: string) => simulate(runId, `~/Backups/demo-${scope || "db"}.sql`, false),
    PickBackupFile: async () => "~/Backups/demo-app-20261119.sql",
    CheckForUpdate: async () => ({
      version: "0.3.0",
      name: "Quarry 0.3.0",
      notes:
        "## Download\n\n| Platform | File |\n| --- | --- |\n| Windows (installer) | `quarry-windows-amd64-installer.exe` |\n\n**macOS:** run `xattr -cr Quarry.app` once.\n\n## What's Changed\n* Fit window to the screen on launch by @HamzaBaghdadi in https://github.com/HamzaBaghdadi/quarry/pull/12\n* **Search** across all rows with `Ctrl+F`\n\n| Engine | Status |\n| --- | --- |\n| PostgreSQL | ✓ |\n\n**Full Changelog**: https://github.com/HamzaBaghdadi/quarry/compare/v0.2.1...v0.2.2",
      method: "installer" as const,
      url: "https://github.com/HamzaBaghdadi/quarry/releases",
      publishedAt: new Date().toISOString(),
      size: 14_000_000,
    }),
    InstallUpdate: async () => {
      for (let i = 1; i <= 20; i++) {
        await wait(80);
        window.dispatchEvent(new CustomEvent("quarry:update-progress", { detail: { done: i * 700_000, total: 14_000_000 } }));
      }
      return "restart" as const;
    },
    RestartApp: async () => location.reload(),
    CloseWindow: async () => {},
    Restore: async (_id: string, _scope: string, path: string, runId: string) => simulate(runId, path, true),
  };
}

async function simulate(runId: string, path: string, restore: boolean): Promise<BackupReport> {
  const total = 2_400_000;
  const steps = ["public.orders", "public.users", "audit.events"];
  for (let i = 1; i <= 12; i++) {
    await wait(180);
    const msg = restore ? `${i * 40} statements · ${i * 4000} rows` : `Table ${Math.ceil(i / 4)}/3 · ${steps[Math.ceil(i / 4) - 1]}`;
    window.dispatchEvent(new CustomEvent("quarry:progress", { detail: { runId, message: msg, done: (total / 12) * i, total: restore ? total : 0 } }));
  }
  return { path, bytes: total, stats: { objects: 3, rows: 48_000, statements: restore ? 480 : 0 }, durationMs: 2160 };
}
