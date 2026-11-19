"use client";

import type { ReactNode } from "react";
import { Copy, Lightning, PencilSimple, Plug, Trash } from "@phosphor-icons/react";
import { toast } from "sonner";
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuLabel,
  ContextMenuSeparator,
  ContextMenuTrigger,
} from "@/components/ui/context-menu";
import { api, type Connection } from "@/lib/api";
import { useStore } from "@/lib/store";
import { confirmDialog } from "./common";

/** Ask, then delete a saved connection (works whether or not it's connected). */
export async function deleteConnection(c: Pick<Connection, "id" | "name">) {
  const ok = await confirmDialog({
    title: `Delete “${c.name}”?`,
    description: "The saved connection and its stored password are removed. Your database itself is not touched.",
    confirmLabel: "Delete",
    destructive: true,
  });
  if (!ok) return false;
  try {
    await useStore.getState().removeConnection(c.id);
    toast.success(`Deleted “${c.name}”`);
    return true;
  } catch (e) {
    toast.error("Couldn't delete the connection", { description: (e as Error).message });
    return false;
  }
}

export async function duplicateConnection(c: Pick<Connection, "id" | "name">) {
  try {
    await api.duplicateConnection(c.id);
    await useStore.getState().refreshConnections();
    toast.success(`Duplicated “${c.name}”`);
  } catch (e) {
    toast.error("Couldn't duplicate the connection", { description: (e as Error).message });
  }
}

const item = "cursor-pointer gap-2.5 text-[13px] [&_svg]:size-4 [&_svg]:text-fg-muted";

/** Right-click menu for a saved connection. */
export function ConnectionMenu({ conn, children }: { conn: Connection; children: ReactNode }) {
  const { connect, disconnect, openDialog } = useStore.getState();
  return (
    <ContextMenu>
      <ContextMenuTrigger asChild>{children}</ContextMenuTrigger>
      <ContextMenuContent className="w-52 border-line-strong bg-popover">
        <ContextMenuLabel className="truncate text-[11px] font-normal text-fg-subtle">{conn.name}</ContextMenuLabel>
        {conn.connected ? (
          <ContextMenuItem className={item} onSelect={() => void disconnect(conn.id)}>
            <Plug /> Disconnect
          </ContextMenuItem>
        ) : (
          <ContextMenuItem className={item} onSelect={() => void connect(conn.id)}>
            <Lightning /> Connect
          </ContextMenuItem>
        )}
        <ContextMenuItem className={item} onSelect={() => openDialog(conn)}>
          <PencilSimple /> Edit…
        </ContextMenuItem>
        <ContextMenuItem className={item} onSelect={() => void duplicateConnection(conn)}>
          <Copy /> Duplicate
        </ContextMenuItem>
        <ContextMenuSeparator />
        <ContextMenuItem className={`${item} text-danger focus:text-danger [&_svg]:!text-danger`} onSelect={() => void deleteConnection(conn)}>
          <Trash /> Delete…
        </ContextMenuItem>
      </ContextMenuContent>
    </ContextMenu>
  );
}
