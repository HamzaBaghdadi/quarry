"use client";

import clsx from "clsx";
import { useEffect, useMemo, useRef, useState } from "react";
import { CheckCircle, FolderOpen, LinkSimple, MagnifyingGlass, WarningCircle, X } from "@phosphor-icons/react";
import { toast } from "sonner";
import { api, emptyConfig, type DriverInfo, type Profile } from "@/lib/api";
import { swatches } from "@/lib/engines";
import { useStore } from "@/lib/store";
import { Button, EngineBadge, IconButton, Input, Label, Select, Spinner } from "./common";
import { Dialog, DialogContent, DialogTitle } from "@/components/ui/dialog";
import { Switch } from "@/components/ui/switch";

const familyLabel = { sql: "Relational", document: "Document", keyvalue: "Key-value" } as const;

export function ConnectionDialog() {
  const { open, profile } = useStore((s) => s.dialog);
  if (!open) return null;
  return <DialogBody key={profile?.id ?? "new"} initial={profile} />;
}

function DialogBody({ initial }: { initial?: Profile }) {
  const drivers = useStore((s) => s.drivers);
  const close = useStore((s) => s.closeDialog);
  const refresh = useStore((s) => s.refreshConnections);
  const connect = useStore((s) => s.connect);
  const editing = !!initial?.id;

  const [step, setStep] = useState<"engine" | "form">(editing ? "form" : "engine");
  const [search, setSearch] = useState("");
  const [p, setP] = useState<Profile>(
    () => initial ?? { id: "", name: "", driver: "", color: swatches[0], config: emptyConfig(), readOnly: false },
  );
  const [useUri, setUseUri] = useState(!!initial?.config.uri);
  const [pwTouched, setPwTouched] = useState(false);
  const [test, setTest] = useState<{ state: "idle" | "busy" | "ok" | "fail"; msg?: string }>({ state: "idle" });
  const [saving, setSaving] = useState(false);
  const dialogRef = useRef<HTMLDivElement>(null);

  const drv = drivers.find((d) => d.id === p.driver);
  const has = (f: string) => !!drv?.fields.includes(f as never);


  const grouped = useMemo(() => {
    const q = search.toLowerCase();
    const list = drivers.filter((d) => d.name.toLowerCase().includes(q) || d.id.includes(q));
    return (["sql", "document", "keyvalue"] as const).map((f) => [f, list.filter((d) => d.family === f)] as const).filter(([, l]) => l.length);
  }, [drivers, search]);

  const pick = (d: DriverInfo) => {
    setP((cur) => ({
      ...cur,
      driver: d.id,
      name: cur.name || `Local ${d.name}`,
      config: { ...cur.config, host: d.fields.includes("host") ? cur.config.host || "localhost" : "", port: d.defaultPort, user: cur.config.user || d.defaultUser, sslMode: d.sslModes?.[0] ?? "" },
    }));
    setUseUri(false);
    setStep("form");
  };

  const set = (k: keyof Profile["config"], v: string | number) => {
    setP((cur) => ({ ...cur, config: { ...cur.config, [k]: v } }));
    setTest({ state: "idle" });
  };

  const payload = (): Profile => ({
    ...p,
    config: useUri ? { ...emptyConfig(), uri: p.config.uri, password: p.config.password } : { ...p.config, uri: "" },
  });
  const keepPassword = editing && !pwTouched;

  const runTest = async () => {
    setTest({ state: "busy" });
    try {
      setTest({ state: "ok", msg: await api.testConnection(payload(), keepPassword) });
    } catch (e) {
      setTest({ state: "fail", msg: (e as Error).message });
    }
  };

  const save = async (andConnect: boolean) => {
    setSaving(true);
    try {
      const saved = await api.saveConnection(payload(), keepPassword);
      await refresh();
      close();
      toast.success(editing ? "Connection updated" : "Connection saved");
      if (andConnect) await connect(saved.id);
    } catch (e) {
      toast.error("Couldn't save", { description: (e as Error).message });
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog open onOpenChange={(o) => !o && close()}>
      <DialogContent
        ref={dialogRef}
        showCloseButton={false}
        aria-describedby={undefined}
        className="flex max-h-[calc(100vh-48px)] w-full max-w-[640px] flex-col gap-0 overflow-hidden rounded-xl border-line-strong bg-bg-1 p-0 shadow-pop sm:max-w-[640px]"
      >
        <header className="flex h-12 shrink-0 items-center gap-2.5 border-b border-line px-4">
          {step === "form" && drv && <EngineBadge driver={drv.id} size={24} />}
          <DialogTitle className="text-[14px] font-semibold">
            {editing ? `Edit ${initial?.name}` : step === "engine" ? "New connection" : `New ${drv?.name} connection`}
          </DialogTitle>
          {step === "form" && !editing && (
            <button onClick={() => setStep("engine")} className="cursor-pointer text-[12px] text-brand-text hover:underline">
              Change engine
            </button>
          )}
          <IconButton label="Close" className="ml-auto" onClick={close}>
            <X className="size-4" />
          </IconButton>
        </header>

        {step === "engine" ? (
          <div className="min-h-0 overflow-auto p-4">
            <div className="relative mb-4">
              <MagnifyingGlass aria-hidden className="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-fg-subtle" />
              <Input autoFocus value={search} onChange={(e) => setSearch(e.target.value)} placeholder="Search engines" className="pl-8" aria-label="Search engines" />
            </div>
            {grouped.map(([fam, list]) => (
              <section key={fam} className="mb-4">
                <h3 className="mb-2 text-[11px] font-semibold uppercase tracking-wider text-fg-subtle">{familyLabel[fam]}</h3>
                <div className="grid grid-cols-2 gap-2 sm:grid-cols-3">
                  {list.map((d) => (
                    <button
                      key={d.id}
                      onClick={() => pick(d)}
                      className="flex cursor-pointer items-center gap-2.5 rounded-lg border border-line bg-bg-0 p-2.5 text-left transition-colors duration-150 hover:border-brand hover:bg-bg-2"
                    >
                      <EngineBadge driver={d.id} size={30} />
                      <span className="min-w-0">
                        <span className="block truncate text-[13px] font-medium">{d.name}</span>
                        <span className="block text-[11px] text-fg-subtle">{d.defaultPort ? `:${d.defaultPort}` : "Local file"}</span>
                      </span>
                    </button>
                  ))}
                </div>
              </section>
            ))}
            <p className="text-[12px] text-fg-subtle">
              Wire-compatible services work too: Supabase, Neon, Timescale (PostgreSQL) · PlanetScale, Vitess (MySQL) · DocumentDB, FerretDB (MongoDB) · Dragonfly, KeyDB (Redis).
            </p>
          </div>
        ) : (
          <form
            className="flex min-h-0 flex-col"
            onSubmit={(e) => {
              e.preventDefault();
              void save(true);
            }}
          >
            <div className="min-h-0 space-y-3.5 overflow-auto p-4">
              <div className="grid grid-cols-[1fr_auto] gap-3">
                <div>
                  <Label htmlFor="c-name">Name</Label>
                  <Input id="c-name" autoFocus required value={p.name} onChange={(e) => setP({ ...p, name: e.target.value })} />
                </div>
                <div>
                  <Label>Color</Label>
                  <div role="radiogroup" aria-label="Connection color" className="flex h-8 items-center gap-1">
                    {swatches.map((c) => (
                      <button
                        type="button"
                        key={c}
                        role="radio"
                        aria-checked={p.color === c}
                        aria-label={c}
                        onClick={() => setP({ ...p, color: c })}
                        className={clsx("size-5 cursor-pointer rounded-full transition-transform duration-150", p.color === c ? "scale-110 ring-2 ring-fg ring-offset-2 ring-offset-bg-1" : "hover:scale-110")}
                        style={{ background: c }}
                      />
                    ))}
                  </div>
                </div>
              </div>

              {has("uri") && (
                <div role="tablist" aria-label="Connection method" className="flex w-fit rounded-lg border border-line bg-bg-0 p-0.5">
                  {[
                    [false, "Parameters"],
                    [true, "Connection URL"],
                  ].map(([v, l]) => (
                    <button
                      type="button"
                      role="tab"
                      key={String(v)}
                      aria-selected={useUri === v}
                      onClick={() => setUseUri(v as boolean)}
                      className={clsx("h-7 cursor-pointer rounded-md px-3 text-[12px] font-medium transition-colors duration-150", useUri === v ? "bg-bg-3 text-fg" : "text-fg-muted hover:text-fg")}
                    >
                      {l as string}
                    </button>
                  ))}
                </div>
              )}

              {has("file") && (
                <div>
                  <Label htmlFor="c-file">Database file</Label>
                  <div className="flex gap-2">
                    <Input id="c-file" required value={p.config.file} onChange={(e) => set("file", e.target.value)} placeholder="C:\data\app.db" className="font-mono" />
                    <Button
                      type="button"
                      onClick={async () => {
                        const f = await api.pickFile();
                        if (f) set("file", f);
                      }}
                    >
                      <FolderOpen aria-hidden className="size-4" /> Browse
                    </Button>
                  </div>
                  <p className="mt-1 text-[11px] text-fg-subtle">A new file is created if it doesn’t exist.</p>
                </div>
              )}

              {useUri && has("uri") ? (
                <>
                  <div>
                    <Label htmlFor="c-uri">Connection URL</Label>
                    <div className="relative">
                      <LinkSimple aria-hidden className="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-fg-subtle" />
                      <Input id="c-uri" required value={p.config.uri} onChange={(e) => set("uri", e.target.value)} placeholder={drv?.uriPlaceholder} className="pl-8 font-mono" />
                    </div>
                    <p className="mt-1 text-[11px] text-fg-subtle">The whole URL is stored in your config file; prefer the Password field below for secrets.</p>
                  </div>
                  <PasswordField p={p} editing={editing} touched={pwTouched} onChange={(v) => { setPwTouched(true); set("password", v); }} />
                </>
              ) : (
                !has("file") && (
                  <>
                    <div className="grid grid-cols-[1fr_110px] gap-3">
                      <div>
                        <Label htmlFor="c-host">Host</Label>
                        <Input id="c-host" value={p.config.host} onChange={(e) => set("host", e.target.value)} placeholder="localhost" className="font-mono" />
                      </div>
                      <div>
                        <Label htmlFor="c-port">Port</Label>
                        <Input id="c-port" inputMode="numeric" value={p.config.port || ""} onChange={(e) => set("port", Number(e.target.value.replace(/\D/g, "")) || 0)} placeholder={String(drv?.defaultPort ?? "")} className="font-mono" />
                      </div>
                    </div>
                    <div className="grid grid-cols-2 gap-3">
                      <div>
                        <Label htmlFor="c-user">User</Label>
                        <Input id="c-user" value={p.config.user} onChange={(e) => set("user", e.target.value)} className="font-mono" />
                      </div>
                      <PasswordField p={p} editing={editing} touched={pwTouched} onChange={(v) => { setPwTouched(true); set("password", v); }} />
                    </div>
                    <div className="grid grid-cols-2 gap-3">
                      <div>
                        <Label htmlFor="c-db" hint="optional">
                          {drv?.family === "keyvalue" ? "Database index" : "Database"}
                        </Label>
                        <Input id="c-db" value={p.config.database} onChange={(e) => set("database", e.target.value)} placeholder={drv?.family === "keyvalue" ? "0" : ""} className="font-mono" />
                      </div>
                      {has("ssl") && drv?.sslModes && (
                        <div>
                          <Label htmlFor="c-ssl">{drv.id === "postgres" || drv.id === "cockroachdb" ? "SSL mode" : "TLS"}</Label>
                          <Select id="c-ssl" value={p.config.sslMode} onChange={(v) => set("sslMode", v)} options={drv.sslModes.map((m) => ({ value: m, label: m }))} />
                        </div>
                      )}
                    </div>
                  </>
                )
              )}

              <label htmlFor="c-ro" className="flex cursor-pointer items-center gap-3 rounded-lg border border-line bg-bg-0 p-3">
                <span className="min-w-0 flex-1">
                  <span className="block text-[13px] font-medium">Read-only</span>
                  <span className="block text-[12px] text-fg-subtle">Block statements that write. Good for production.</span>
                </span>
                <Switch id="c-ro" checked={p.readOnly} onCheckedChange={(v) => setP({ ...p, readOnly: v })} className="cursor-pointer data-[state=checked]:bg-brand" />
              </label>

              {test.state !== "idle" && test.state !== "busy" && (
                <div
                  role="status"
                  className={clsx(
                    "flex items-start gap-2 rounded-lg border p-2.5 text-[12px]",
                    test.state === "ok" ? "border-run/30 bg-run/10 text-fg" : "border-danger/30 bg-danger-soft text-fg",
                  )}
                >
                  {test.state === "ok" ? <CheckCircle aria-hidden weight="fill" className="size-4 shrink-0 text-run" /> : <WarningCircle aria-hidden weight="fill" className="size-4 shrink-0 text-danger" />}
                  <span className="selectable min-w-0 break-words font-mono">{test.msg}</span>
                </div>
              )}
            </div>

            <footer className="flex h-14 shrink-0 items-center gap-2 border-t border-line bg-bg-0 px-4">
              <Button type="button" onClick={runTest} disabled={test.state === "busy"}>
                {test.state === "busy" ? <Spinner /> : null} Test
              </Button>
              <div className="ml-auto flex gap-2">
                <Button type="button" variant="ghost" onClick={() => save(false)} disabled={saving}>
                  Save
                </Button>
                <Button type="submit" variant="primary" disabled={saving}>
                  {saving && <Spinner />} Save & connect
                </Button>
              </div>
            </footer>
          </form>
        )}
      </DialogContent>
    </Dialog>
  );
}

function PasswordField({ p, editing, touched, onChange }: { p: Profile; editing: boolean; touched: boolean; onChange: (v: string) => void }) {
  return (
    <div>
      <Label htmlFor="c-pw" hint="stored in OS keychain">
        Password
      </Label>
      <Input id="c-pw" type="password" value={p.config.password} onChange={(e) => onChange(e.target.value)} placeholder={editing && !touched ? "•••••• unchanged" : ""} />
    </div>
  );
}
