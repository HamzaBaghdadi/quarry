"use client";

import { useEffect, useState } from "react";
import { Database, Trash, WarningOctagon } from "@phosphor-icons/react";
import { toast } from "sonner";
import { create } from "zustand";
import { Dialog, DialogContent, DialogDescription, DialogTitle } from "@/components/ui/dialog";
import { api, type Role } from "@/lib/api";
import { useStore } from "@/lib/store";
import { Button, Input, Label, Select, Spinner } from "./common";

type Req = { mode: "create" | "drop"; connId: string; name?: string } | null;
const useDbDialog = create<{ req: Req }>(() => ({ req: null }));

export const openCreateDatabase = (connId: string) => useDbDialog.setState({ req: { mode: "create", connId } });
export const openDropDatabase = (connId: string, name: string) => useDbDialog.setState({ req: { mode: "drop", connId, name } });
const close = () => useDbDialog.setState({ req: null });

/** Reload the explorer root and drop tabs for a removed database. */
function afterChange(connId: string, dropped?: string) {
  const s = useStore.getState();
  if (dropped) {
    const tabs = s.tabs.filter((t) => !(t.connId === connId && t.scope === dropped && t.kind === "table"));
    useStore.setState({ tabs, activeTab: tabs.some((t) => t.id === s.activeTab) ? s.activeTab : tabs.at(-1)?.id ?? null });
  }
  void s.loadChildren(connId, [], true);
}

export function DatabaseDialogs() {
  const req = useDbDialog((s) => s.req);
  if (!req) return null;
  return req.mode === "create" ? <CreateDatabase connId={req.connId} key={req.connId} /> : <DropDatabase connId={req.connId} name={req.name!} key={req.name} />;
}

function CreateDatabase({ connId }: { connId: string }) {
  const conn = useStore((s) => s.connections.find((c) => c.id === connId));
  const driver = useStore((s) => s.drivers.find((d) => d.id === conn?.driver));
  const supportsOwner = !!driver?.capabilities?.includes("roles");
  const [name, setName] = useState("");
  const [owner, setOwner] = useState("");
  const [roles, setRoles] = useState<Role[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (supportsOwner) void api.listRoles(connId).then((r) => setRoles(r.filter((x) => !x.builtIn)), () => {});
  }, [connId, supportsOwner]);

  const submit = async () => {
    setBusy(true);
    setError(null);
    try {
      await api.createDatabase(connId, name.trim(), owner);
      toast.success(`Created database “${name.trim()}”`);
      close();
      afterChange(connId);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <Dialog open onOpenChange={(o) => !o && close()}>
      <DialogContent className="max-w-[440px] gap-0 overflow-hidden rounded-xl border-line-strong bg-bg-1 p-0 shadow-pop sm:max-w-[440px]">
        <header className="flex items-center gap-3 px-5 pb-3 pt-5">
          <span className="flex size-9 items-center justify-center rounded-lg bg-brand-soft text-brand-text">
            <Database weight="fill" className="size-4" />
          </span>
          <div>
            <DialogTitle className="text-[15px] font-semibold">Create database</DialogTitle>
            <DialogDescription className="text-[12px] text-fg-subtle">on {conn?.name}</DialogDescription>
          </div>
        </header>
        <form
          className="space-y-3 px-5 pb-4"
          onSubmit={(e) => {
            e.preventDefault();
            if (name.trim()) void submit();
          }}
        >
          <div>
            <Label htmlFor="db-name">Name</Label>
            <Input id="db-name" autoFocus value={name} onChange={(e) => setName(e.target.value)} placeholder="my_database" className="font-mono" />
          </div>
          {supportsOwner && (
            <div>
              <Label htmlFor="db-owner" hint="optional">
                Owner
              </Label>
              <Select
                id="db-owner"
                value={owner}
                onChange={setOwner}
                options={[{ value: "", label: "Current user" }, ...roles.map((r) => ({ value: r.name, label: r.name }))]}
              />
            </div>
          )}
          {error && (
            <div role="alert" className="flex gap-2 rounded-lg border border-danger/30 bg-danger-soft p-2.5 text-[12.5px]">
              <WarningOctagon aria-hidden weight="fill" className="mt-0.5 size-4 shrink-0 text-danger" />
              <span className="selectable min-w-0 break-words font-mono">{error}</span>
            </div>
          )}
          <button type="submit" hidden />
        </form>
        <footer className="flex justify-end gap-2 border-t border-line bg-bg-0 px-5 py-3">
          <Button variant="ghost" onClick={close}>
            Cancel
          </Button>
          <Button variant="primary" disabled={busy || !name.trim()} onClick={() => void submit()}>
            {busy && <Spinner />} Create
          </Button>
        </footer>
      </DialogContent>
    </Dialog>
  );
}

function DropDatabase({ connId, name }: { connId: string; name: string }) {
  const [typed, setTyped] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const match = typed === name;

  const submit = async () => {
    setBusy(true);
    setError(null);
    try {
      await api.dropDatabase(connId, name);
      toast.success(`Dropped database “${name}”`);
      close();
      afterChange(connId, name);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <Dialog open onOpenChange={(o) => !o && !busy && close()}>
      <DialogContent className="max-w-[440px] gap-0 overflow-hidden rounded-xl border-line-strong bg-bg-1 p-0 shadow-pop sm:max-w-[440px]">
        <header className="flex items-center gap-3 px-5 pb-3 pt-5">
          <span className="flex size-9 items-center justify-center rounded-lg bg-danger-soft text-danger">
            <Trash weight="fill" className="size-4" />
          </span>
          <div>
            <DialogTitle className="text-[15px] font-semibold">Drop database “{name}”?</DialogTitle>
            <DialogDescription className="text-[12px] text-fg-subtle">Every table and row in it is deleted permanently.</DialogDescription>
          </div>
        </header>
        <form
          className="space-y-3 px-5 pb-4"
          onSubmit={(e) => {
            e.preventDefault();
            if (match) void submit();
          }}
        >
          <p className="rounded-lg border border-danger/30 bg-danger-soft p-2.5 text-[12.5px]">
            This can’t be undone. Consider a <strong>backup</strong> first.
          </p>
          <div>
            <Label htmlFor="db-confirm">
              Type <span className="font-mono text-fg">{name}</span> to confirm
            </Label>
            <Input id="db-confirm" autoFocus value={typed} onChange={(e) => setTyped(e.target.value)} className="font-mono" />
          </div>
          {error && (
            <div role="alert" className="flex gap-2 rounded-lg border border-danger/30 bg-danger-soft p-2.5 text-[12.5px]">
              <WarningOctagon aria-hidden weight="fill" className="mt-0.5 size-4 shrink-0 text-danger" />
              <span className="selectable min-w-0 break-words font-mono">{error}</span>
            </div>
          )}
          <button type="submit" hidden />
        </form>
        <footer className="flex justify-end gap-2 border-t border-line bg-bg-0 px-5 py-3">
          <Button variant="ghost" onClick={close} disabled={busy}>
            Cancel
          </Button>
          <Button variant="primary" className="!bg-destructive !text-white hover:!bg-destructive/90" disabled={busy || !match} onClick={() => void submit()}>
            {busy && <Spinner />} Drop database
          </Button>
        </footer>
      </DialogContent>
    </Dialog>
  );
}
