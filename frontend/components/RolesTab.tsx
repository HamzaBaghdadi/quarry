"use client";

import clsx from "clsx";
import { useCallback, useEffect, useMemo, useState } from "react";
import { ArrowClockwise, Crown, DotsThree, MagnifyingGlass, PencilSimple, Plus, ShieldCheck, Trash, User, UsersThree, WarningOctagon } from "@phosphor-icons/react";
import { toast } from "sonner";
import { Switch } from "@/components/ui/switch";
import { Checkbox } from "@/components/ui/checkbox";
import { Dialog, DialogContent, DialogDescription, DialogTitle } from "@/components/ui/dialog";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { api, PATH_ROLES, type Role } from "@/lib/api";
import { useStore, type Tab } from "@/lib/store";
import { Button, IconButton, Input, Label, Spinner, confirmDialog } from "./common";

const blank = (): Role => ({
  name: "",
  canLogin: true,
  superuser: false,
  createDb: false,
  createRole: false,
  inherit: true,
  replication: false,
  bypassRls: false,
  connLimit: -1,
  validUntil: "",
  memberOf: [],
  comment: "",
  builtIn: false,
});

const privileges: { key: keyof Role; label: string; hint: string }[] = [
  { key: "canLogin", label: "Can log in", hint: "Login role (a user). Off = group role." },
  { key: "superuser", label: "Superuser", hint: "Bypasses every permission check." },
  { key: "createDb", label: "Create databases", hint: "Can run CREATE DATABASE." },
  { key: "createRole", label: "Create roles", hint: "Can create, alter and drop other roles." },
  { key: "inherit", label: "Inherit privileges", hint: "Automatically gets the privileges of its groups." },
  { key: "replication", label: "Replication", hint: "Can start streaming replication and backups." },
  { key: "bypassRls", label: "Bypass row-level security", hint: "Ignores RLS policies." },
];

function Badge({ children, tone = "muted" }: { children: React.ReactNode; tone?: "muted" | "warn" | "brand" | "run" }) {
  return (
    <span
      className={clsx(
        "inline-flex h-5 items-center rounded px-1.5 text-[11px] font-medium",
        tone === "warn" && "bg-warn/15 text-warn",
        tone === "brand" && "bg-brand-soft text-brand-text",
        tone === "run" && "bg-run/15 text-run",
        tone === "muted" && "bg-bg-3 text-fg-muted",
      )}
    >
      {children}
    </span>
  );
}

export function RolesTab({ tab }: { tab: Tab }) {
  const conn = useStore((s) => s.connections.find((c) => c.id === tab.connId));
  const updateTab = useStore((s) => s.updateTab);
  const loadChildren = useStore((s) => s.loadChildren);
  const [roles, setRoles] = useState<Role[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [filter, setFilter] = useState("");
  const [showBuiltIn, setShowBuiltIn] = useState(false);
  const [editing, setEditing] = useState<{ role: Role; isNew: boolean } | null>(null);
  const readOnly = !!conn?.readOnly;

  const load = useCallback(async () => {
    setError(null);
    try {
      setRoles(await api.listRoles(tab.connId));
    } catch (e) {
      setError((e as Error).message);
    }
  }, [tab.connId]);

  useEffect(() => {
    void load();
  }, [load]);

  // Opened from a role in the explorer: jump straight into editing it.
  useEffect(() => {
    if (!tab.focus || !roles) return;
    const r = roles.find((x) => x.name === tab.focus);
    if (r) {
      if (r.builtIn) setShowBuiltIn(true);
      setEditing({ role: { ...r, memberOf: [...r.memberOf] }, isNew: false });
    }
    updateTab(tab.id, { focus: undefined });
  }, [tab.focus, tab.id, roles, updateTab]);

  const refreshTree = () => void loadChildren(tab.connId, [PATH_ROLES], true);

  const visible = useMemo(
    () => (roles ?? []).filter((r) => (showBuiltIn || !r.builtIn) && r.name.toLowerCase().includes(filter.toLowerCase())),
    [roles, showBuiltIn, filter],
  );

  const drop = async (r: Role) => {
    const ok = await confirmDialog({
      title: `Drop role “${r.name}”?`,
      description: "This runs DROP ROLE. It fails if the role still owns objects or has privileges granted.",
      confirmLabel: "Drop role",
      destructive: true,
    });
    if (!ok) return;
    try {
      await api.dropRole(tab.connId, r.name);
      toast.success(`Dropped role “${r.name}”`);
      void load();
      refreshTree();
    } catch (e) {
      toast.error("Couldn't drop the role", { description: (e as Error).message, duration: 10000 });
    }
  };

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="flex h-11 shrink-0 items-center gap-2 border-b border-line bg-bg-1 px-3">
        <h2 className="flex items-center gap-2 text-[13px] font-semibold">
          <UsersThree aria-hidden className="size-4 text-brand-text" /> Login/Group Roles
        </h2>
        {roles && <span className="text-[12px] text-fg-subtle">{visible.length} shown</span>}
        <div className="relative ml-auto w-56">
          <MagnifyingGlass aria-hidden className="pointer-events-none absolute left-2 top-1/2 size-3.5 -translate-y-1/2 text-fg-subtle" />
          <Input value={filter} onChange={(e) => setFilter(e.target.value)} placeholder="Filter roles" aria-label="Filter roles" className="h-7 pl-7 text-[12px]" />
        </div>
        <label className="flex cursor-pointer items-center gap-2 text-[12px] text-fg-muted">
          <Switch checked={showBuiltIn} onCheckedChange={setShowBuiltIn} className="cursor-pointer data-[state=checked]:bg-brand" />
          Built-in roles
        </label>
        <IconButton label="Refresh" onClick={() => void load()}>
          <ArrowClockwise className="size-4" />
        </IconButton>
        <Button size="sm" variant="primary" disabled={readOnly} onClick={() => setEditing({ role: blank(), isNew: true })}>
          <Plus aria-hidden className="size-3.5" /> New role
        </Button>
      </div>

      <div className="min-h-0 flex-1 overflow-auto">
        {error ? (
          <div role="alert" className="m-3 flex gap-2.5 rounded-lg border border-danger/30 bg-danger-soft p-3 text-[13px]">
            <WarningOctagon aria-hidden weight="fill" className="mt-0.5 size-4 shrink-0 text-danger" />
            <span className="selectable">{error}</span>
          </div>
        ) : !roles ? (
          <div className="flex items-center justify-center p-10">
            <Spinner className="text-brand-text" />
          </div>
        ) : (
          <table className="w-full border-collapse text-[13px]">
            <thead className="sticky top-0 z-10 bg-bg-0 text-left text-[11.5px] font-semibold text-fg-muted">
              <tr className="border-b border-line-strong">
                <th className="px-3 py-2 font-semibold">Role</th>
                <th className="px-3 py-2 font-semibold">Attributes</th>
                <th className="px-3 py-2 font-semibold">Member of</th>
                <th className="px-3 py-2 font-semibold">Connections</th>
                <th className="px-3 py-2 font-semibold">Expires</th>
                <th className="w-10" />
              </tr>
            </thead>
            <tbody>
              {visible.map((r) => (
                <tr
                  key={r.name}
                  onDoubleClick={() => setEditing({ role: { ...r, memberOf: [...r.memberOf] }, isNew: false })}
                  className="group border-b border-line/70 transition-colors duration-100 hover:bg-bg-2/60"
                >
                  <td className="px-3 py-2">
                    <div className="flex items-center gap-2.5">
                      <span className={clsx("flex size-7 shrink-0 items-center justify-center rounded-full", r.canLogin ? "bg-brand-soft text-brand-text" : "bg-bg-3 text-fg-muted")}>
                        {r.canLogin ? <User weight="fill" className="size-3.5" /> : <UsersThree weight="fill" className="size-3.5" />}
                      </span>
                      <div className="min-w-0">
                        <div className="truncate font-medium">{r.name}</div>
                        <div className="truncate text-[11.5px] text-fg-subtle">{r.comment || (r.canLogin ? "Login role" : "Group role")}</div>
                      </div>
                    </div>
                  </td>
                  <td className="px-3 py-2">
                    <div className="flex flex-wrap gap-1">
                      {r.superuser && (
                        <Badge tone="warn">
                          <Crown weight="fill" className="mr-1 size-3" /> Superuser
                        </Badge>
                      )}
                      {r.canLogin && <Badge tone="brand">Login</Badge>}
                      {r.createDb && <Badge>Create DB</Badge>}
                      {r.createRole && <Badge>Create role</Badge>}
                      {r.replication && <Badge>Replication</Badge>}
                      {r.bypassRls && <Badge>Bypass RLS</Badge>}
                      {r.builtIn && <Badge>Built-in</Badge>}
                    </div>
                  </td>
                  <td className="px-3 py-2 text-[12px] text-fg-muted">{r.memberOf.length ? r.memberOf.join(", ") : "—"}</td>
                  <td className="px-3 py-2 font-mono text-[12px] text-fg-muted">{r.connLimit < 0 ? "∞" : r.connLimit}</td>
                  <td className="px-3 py-2 font-mono text-[12px] text-fg-muted">{r.validUntil ? r.validUntil.slice(0, 10) : "never"}</td>
                  <td className="px-1">
                    <DropdownMenu>
                      <DropdownMenuTrigger aria-label={`Actions for ${r.name}`} className="flex size-7 cursor-pointer items-center justify-center rounded-md text-fg-muted opacity-0 hover:bg-bg-3 hover:text-fg focus:opacity-100 group-hover:opacity-100 data-[state=open]:opacity-100">
                        <DotsThree weight="bold" className="size-4" />
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end" className="w-44 border-line-strong bg-popover">
                        <DropdownMenuItem className="cursor-pointer gap-2.5 text-[13px]" onSelect={() => setEditing({ role: { ...r, memberOf: [...r.memberOf] }, isNew: false })}>
                          <PencilSimple className="size-4" /> {readOnly || r.builtIn ? "View" : "Edit…"}
                        </DropdownMenuItem>
                        {!r.builtIn && (
                          <>
                            <DropdownMenuSeparator />
                            <DropdownMenuItem disabled={readOnly} className="cursor-pointer gap-2.5 text-[13px] text-danger focus:text-danger" onSelect={() => void drop(r)}>
                              <Trash className="size-4" /> Drop role…
                            </DropdownMenuItem>
                          </>
                        )}
                      </DropdownMenuContent>
                    </DropdownMenu>
                  </td>
                </tr>
              ))}
              {!visible.length && (
                <tr>
                  <td colSpan={6} className="px-3 py-10 text-center text-[13px] text-fg-subtle">
                    No roles match.
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        )}
      </div>

      {editing && roles && (
        <RoleDialog
          connId={tab.connId}
          initial={editing.role}
          isNew={editing.isNew}
          allRoles={roles}
          readOnly={readOnly || editing.role.builtIn}
          onClose={() => setEditing(null)}
          onSaved={() => {
            setEditing(null);
            void load();
            refreshTree();
          }}
        />
      )}
    </div>
  );
}

function RoleDialog({
  connId,
  initial,
  isNew,
  allRoles,
  readOnly,
  onClose,
  onSaved,
}: {
  connId: string;
  initial: Role;
  isNew: boolean;
  allRoles: Role[];
  readOnly: boolean;
  onClose: () => void;
  onSaved: () => void;
}) {
  const [r, setR] = useState<Role>(initial);
  const [password, setPassword] = useState("");
  const [changePw, setChangePw] = useState(isNew);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const groups = allRoles.filter((x) => x.name !== initial.name);

  const save = async () => {
    setSaving(true);
    setError(null);
    try {
      const limit = Number.isFinite(r.connLimit) ? r.connLimit : -1;
      await api.saveRole(connId, {
        oldName: isNew ? "" : initial.name,
        role: { ...r, name: r.name.trim(), connLimit: limit },
        setPassword: changePw && (isNew ? password !== "" : true),
        password,
      });
      toast.success(isNew ? `Created role “${r.name}”` : `Saved role “${r.name}”`);
      onSaved();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="flex max-h-[calc(100vh-48px)] w-full max-w-[640px] flex-col gap-0 overflow-hidden rounded-xl border-line-strong bg-bg-1 p-0 shadow-pop sm:max-w-[640px]">
        <header className="flex items-center gap-3 border-b border-line px-5 py-4">
          <span className="flex size-9 items-center justify-center rounded-full bg-brand-soft text-brand-text">
            {r.canLogin ? <User weight="fill" className="size-4" /> : <UsersThree weight="fill" className="size-4" />}
          </span>
          <div>
            <DialogTitle className="text-[15px] font-semibold">{isNew ? "New role" : readOnly ? initial.name : `Edit ${initial.name}`}</DialogTitle>
            <DialogDescription className="text-[12px] text-fg-subtle">
              {readOnly ? "Built-in roles and read-only connections can only be viewed." : "Changes are applied together in one transaction."}
            </DialogDescription>
          </div>
        </header>

        <form
          className="min-h-0 space-y-5 overflow-auto px-5 py-4"
          onSubmit={(e) => {
            e.preventDefault();
            if (!readOnly) void save();
          }}
        >
          <fieldset disabled={readOnly} className="space-y-5">
            <div className="grid grid-cols-2 gap-3">
              <div>
                <Label htmlFor="r-name">Name</Label>
                <Input id="r-name" autoFocus required value={r.name} onChange={(e) => setR({ ...r, name: e.target.value })} className="font-mono" />
              </div>
              <div>
                <Label htmlFor="r-pw" hint={!isNew && !changePw ? <button type="button" className="cursor-pointer text-brand-text hover:underline" onClick={() => setChangePw(true)}>Change</button> : undefined}>
                  Password
                </Label>
                <Input
                  id="r-pw"
                  type="password"
                  disabled={!changePw}
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  placeholder={changePw ? (isNew ? "Optional" : "Leave empty to remove") : "•••••• unchanged"}
                />
              </div>
            </div>

            <section>
              <h3 className="mb-2 text-[11px] font-semibold uppercase tracking-wider text-fg-subtle">Privileges</h3>
              <div className="grid grid-cols-1 gap-1 sm:grid-cols-2">
                {privileges.map((p) => (
                  <label key={p.key} className="flex cursor-pointer items-start gap-3 rounded-lg border border-line bg-bg-0 p-2.5 has-[:disabled]:cursor-default">
                    <Switch
                      checked={r[p.key] as boolean}
                      onCheckedChange={(v) => setR({ ...r, [p.key]: v })}
                      className="mt-0.5 cursor-pointer data-[state=checked]:bg-brand"
                    />
                    <span className="min-w-0">
                      <span className="flex items-center gap-1.5 text-[13px] font-medium">
                        {p.key === "superuser" && <ShieldCheck aria-hidden className="size-3.5 text-warn" />}
                        {p.label}
                      </span>
                      <span className="block text-[11.5px] text-fg-subtle">{p.hint}</span>
                    </span>
                  </label>
                ))}
              </div>
            </section>

            <div className="grid grid-cols-2 gap-3">
              <div>
                <Label htmlFor="r-limit" hint="-1 = unlimited">
                  Connection limit
                </Label>
                <Input
                  id="r-limit"
                  inputMode="numeric"
                  value={String(r.connLimit)}
                  onChange={(e) => {
                    const n = parseInt(e.target.value, 10);
                    setR({ ...r, connLimit: Number.isNaN(n) ? -1 : Math.max(-1, n) });
                  }}
                  className="font-mono"
                />
              </div>
              <div>
                <Label htmlFor="r-valid" hint="empty = never">
                  Password expires
                </Label>
                <Input id="r-valid" value={r.validUntil} onChange={(e) => setR({ ...r, validUntil: e.target.value })} placeholder="2027-12-31" className="font-mono" />
              </div>
            </div>

            <section>
              <h3 className="mb-2 text-[11px] font-semibold uppercase tracking-wider text-fg-subtle">Member of</h3>
              <div className="max-h-40 overflow-auto rounded-lg border border-line bg-bg-0 p-1">
                {groups.map((g) => {
                  const on = r.memberOf.includes(g.name);
                  return (
                    <label key={g.name} className="flex cursor-pointer items-center gap-2.5 rounded-md px-2 py-1.5 text-[13px] hover:bg-bg-2">
                      <Checkbox
                        checked={on}
                        onCheckedChange={(v) => setR({ ...r, memberOf: v ? [...r.memberOf, g.name] : r.memberOf.filter((x) => x !== g.name) })}
                        className="cursor-pointer data-[state=checked]:border-brand data-[state=checked]:bg-brand"
                      />
                      <span className="font-mono text-[12.5px]">{g.name}</span>
                      {g.builtIn && <span className="text-[11px] text-fg-subtle">built-in</span>}
                    </label>
                  );
                })}
              </div>
            </section>

            <div>
              <Label htmlFor="r-comment" hint="optional">
                Comment
              </Label>
              <Input id="r-comment" value={r.comment} onChange={(e) => setR({ ...r, comment: e.target.value })} />
            </div>
          </fieldset>

          {error && (
            <div role="alert" className="flex gap-2 rounded-lg border border-danger/30 bg-danger-soft p-2.5 text-[12.5px]">
              <WarningOctagon aria-hidden weight="fill" className="mt-0.5 size-4 shrink-0 text-danger" />
              <span className="selectable min-w-0 break-words font-mono">{error}</span>
            </div>
          )}
          <button type="submit" hidden />
        </form>

        <footer className="flex items-center justify-end gap-2 border-t border-line bg-bg-0 px-5 py-3">
          <Button variant="ghost" onClick={onClose}>
            {readOnly ? "Close" : "Cancel"}
          </Button>
          {!readOnly && (
            <Button variant="primary" disabled={saving || !r.name.trim()} onClick={() => void save()}>
              {saving && <Spinner />} {isNew ? "Create role" : "Save changes"}
            </Button>
          )}
        </footer>
      </DialogContent>
    </Dialog>
  );
}
