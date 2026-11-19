"use client";

import { ArrowCircleUp, ArrowSquareOut, CheckCircle, WarningOctagon } from "@phosphor-icons/react";
import { Dialog, DialogContent, DialogDescription, DialogTitle } from "@/components/ui/dialog";
import { win } from "@/lib/platform";
import { skipVersion, useUpdater } from "@/lib/updater";
import { Button, Spinner } from "./common";

const mb = (n: number) => `${(n / (1 << 20)).toFixed(1)} MB`;

/** Renders GitHub release notes (light markdown: headings, bullets, code). */
function Notes({ text }: { text: string }) {
  const lines = text.replace(/\r/g, "").split("\n");
  return (
    <div className="selectable space-y-1.5 text-[13px] leading-relaxed text-fg-muted">
      {lines.map((l, i) => {
        const t = l.trim();
        if (!t) return null;
        if (/^#{1,6}\s/.test(t))
          return (
            <h4 key={i} className="pt-1.5 text-[12px] font-semibold uppercase tracking-wider text-fg">
              {t.replace(/^#+\s*/, "")}
            </h4>
          );
        const bullet = /^[-*]\s+/.test(t);
        const body = t.replace(/^[-*]\s+/, "").replace(/\*\*(.+?)\*\*/g, "$1");
        const parts = body.split(/(`[^`]+`)/g).map((p, j) =>
          p.startsWith("`") ? (
            <code key={j} className="rounded bg-bg-3 px-1 font-mono text-[12px] text-fg">
              {p.slice(1, -1)}
            </code>
          ) : (
            p
          ),
        );
        return bullet ? (
          <div key={i} className="flex gap-2">
            <span aria-hidden className="mt-[9px] size-1 shrink-0 rounded-full bg-fg-subtle" />
            <span>{parts}</span>
          </div>
        ) : (
          <p key={i}>{parts}</p>
        );
      })}
    </div>
  );
}

export function UpdateDialog() {
  const { dialog, available: rel, status, progress, error } = useUpdater();
  const { closeDialog, install, restart } = useUpdater.getState();
  const pct = progress.total ? Math.min(100, (progress.done / progress.total) * 100) : 0;
  const busy = status === "downloading";

  return (
    <Dialog open={dialog && !!rel} onOpenChange={(o) => !o && closeDialog()}>
      <DialogContent
        showCloseButton={!busy}
        onInteractOutside={(e) => busy && e.preventDefault()}
        className="max-w-[480px] gap-0 overflow-hidden rounded-xl border-line-strong bg-bg-1 p-0 shadow-pop sm:max-w-[480px]"
      >
        <div className="flex items-start gap-3 p-5 pb-3">
          <div className="flex size-10 shrink-0 items-center justify-center rounded-xl bg-brand-soft">
            <ArrowCircleUp aria-hidden weight="fill" className="size-6 text-brand-text" />
          </div>
          <div className="min-w-0">
            <DialogTitle className="text-[15px] font-semibold">Quarry {rel?.version} is available</DialogTitle>
            <DialogDescription className="mt-0.5 text-[12.5px] text-fg-subtle">
              {rel?.publishedAt ? `Released ${new Date(rel.publishedAt).toLocaleDateString()}` : ""}
              {rel?.size ? ` · ${mb(rel.size)}` : ""}
            </DialogDescription>
          </div>
        </div>

        <div className="max-h-[300px] overflow-auto border-y border-line bg-bg-0 px-5 py-3">
          {rel?.notes ? <Notes text={rel.notes} /> : <p className="text-[13px] text-fg-subtle">No release notes.</p>}
        </div>

        {(busy || status === "ready" || status === "error") && (
          <div className="px-5 pt-4" aria-live="polite">
            {busy && (
              <div className="space-y-2">
                <div className="flex justify-between text-[12px] text-fg-muted">
                  <span className="flex items-center gap-2">
                    <Spinner className="size-3 text-brand-text" /> Downloading & verifying…
                  </span>
                  <span className="font-mono tabular-nums">
                    {mb(progress.done)}
                    {progress.total ? ` / ${mb(progress.total)}` : ""}
                  </span>
                </div>
                <div className="h-1.5 overflow-hidden rounded-full bg-bg-3" role="progressbar" aria-valuenow={Math.round(pct)} aria-valuemin={0} aria-valuemax={100}>
                  <div className="h-full rounded-full bg-brand transition-[width] duration-200" style={{ width: `${pct}%` }} />
                </div>
              </div>
            )}
            {status === "ready" && (
              <p className="flex items-center gap-2 text-[13px]">
                <CheckCircle aria-hidden weight="fill" className="size-4 text-run" /> Update installed. Restart to start using it.
              </p>
            )}
            {status === "error" && (
              <div role="alert" className="flex gap-2 rounded-lg border border-danger/30 bg-danger-soft p-2.5 text-[12.5px]">
                <WarningOctagon aria-hidden weight="fill" className="mt-0.5 size-4 shrink-0 text-danger" />
                <span className="min-w-0 break-words">
                  {error} You can also download it from the release page.
                </span>
              </div>
            )}
          </div>
        )}

        <footer className="flex items-center gap-2 p-4">
          <Button variant="ghost" size="sm" onClick={() => rel && win.openURL(rel.url)}>
            <ArrowSquareOut aria-hidden className="size-3.5" /> Release page
          </Button>
          <div className="ml-auto flex gap-2">
            {status === "ready" ? (
              <Button variant="primary" onClick={restart}>
                Restart now
              </Button>
            ) : (
              <>
                <Button variant="ghost" disabled={busy} onClick={() => rel && skipVersion(rel.version)}>
                  Skip this version
                </Button>
                <Button variant="primary" disabled={busy} onClick={install}>
                  {busy ? <Spinner /> : null} {status === "error" ? "Try again" : "Update & restart"}
                </Button>
              </>
            )}
          </div>
        </footer>
      </DialogContent>
    </Dialog>
  );
}
