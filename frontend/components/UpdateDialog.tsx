"use client";

import { ArrowCircleUp, ArrowSquareOut, CheckCircle, WarningOctagon } from "@phosphor-icons/react";
import { Dialog, DialogContent, DialogDescription, DialogTitle } from "@/components/ui/dialog";
import { win } from "@/lib/platform";
import { skipVersion, useUpdater } from "@/lib/updater";
import { Button, Spinner } from "./common";

const mb = (n: number) => `${(n / (1 << 20)).toFixed(1)} MB`;

/** Inline markdown: `code`, **bold**, [links](url) and bare URLs. */
function Inline({ text }: { text: string }) {
  const out: React.ReactNode[] = [];
  const re = /(`[^`]+`)|(\*\*[^*]+\*\*)|(\[[^\]]+\]\([^)]+\))|(https?:\/\/[^\s)]+)/g;
  let last = 0;
  for (const m of text.matchAll(re)) {
    if (m.index! > last) out.push(text.slice(last, m.index));
    const [tok] = m;
    const key = m.index;
    if (m[1]) {
      out.push(
        <code key={key} className="rounded bg-bg-3 px-1 font-mono text-[12px] text-fg">
          {tok.slice(1, -1)}
        </code>,
      );
    } else if (m[2]) {
      out.push(
        <strong key={key} className="font-semibold text-fg">
          {tok.slice(2, -2)}
        </strong>,
      );
    } else {
      const [, label, url] = m[3] ? /\[([^\]]+)\]\(([^)]+)\)/.exec(tok)! : [tok, tok.replace(/^https:\/\/github\.com\/[^/]+\/[^/]+\//, ""), tok];
      out.push(
        <button key={key} onClick={() => win.openURL(url)} className="cursor-pointer text-brand-text underline-offset-2 hover:underline">
          {label}
        </button>,
      );
    }
    last = m.index! + tok.length;
  }
  out.push(text.slice(last));
  return <>{out}</>;
}

/**
 * Renders GitHub release notes: headings, bullets, tables and inline markup.
 * The "Download" section is dropped — you're already updating in the app.
 */
function Notes({ text }: { text: string }) {
  const lines = text.replace(/\r/g, "").split("\n");
  const blocks: React.ReactNode[] = [];
  let skipping = false;
  for (let i = 0; i < lines.length; i++) {
    const t = lines[i].trim();
    const heading = /^(#{1,6})\s+(.*)$/.exec(t);
    if (heading) {
      skipping = /download|install/i.test(heading[2]);
      if (!skipping)
        blocks.push(
          <h4 key={i} className="pt-2 text-[11.5px] font-semibold uppercase tracking-wider text-fg">
            {heading[2]}
          </h4>,
        );
      continue;
    }
    // The changelog link is useful even when it sits inside a skipped section.
    if (skipping && !t.startsWith("**Full Changelog**")) continue;
    if (!t) continue;
    if (t.startsWith("|")) {
      const rows: string[][] = [];
      for (; i < lines.length && lines[i].trim().startsWith("|"); i++) {
        const cells = lines[i].trim().replace(/^\||\|$/g, "").split("|").map((c) => c.trim());
        if (!cells.every((c) => /^:?-+:?$/.test(c))) rows.push(cells);
      }
      i--;
      blocks.push(
        <table key={i} className="w-full border-collapse text-[12.5px]">
          <tbody>
            {rows.map((r, ri) => (
              <tr key={ri} className="border-b border-line last:border-0">
                {r.map((c, ci) => (
                  <td key={ci} className={ci === 0 ? "py-1 pr-3 text-fg" : "py-1"}>
                    <Inline text={c} />
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>,
      );
      continue;
    }
    if (/^[-*]\s+/.test(t)) {
      blocks.push(
        <div key={i} className="flex gap-2">
          <span aria-hidden className="mt-[9px] size-1 shrink-0 rounded-full bg-fg-subtle" />
          <span className="min-w-0">
            <Inline text={t.replace(/^[-*]\s+/, "")} />
          </span>
        </div>,
      );
      continue;
    }
    blocks.push(
      <p key={i}>
        <Inline text={t} />
      </p>,
    );
  }
  if (!blocks.length) return <p className="text-[13px] text-fg-subtle">No notes were published for this version — see the release page for details.</p>;
  return <div className="selectable space-y-1.5 text-[13px] leading-relaxed text-fg-muted">{blocks}</div>;
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
              {rel?.size ? ` · ${mb(rel.size)} download` : ""}
            </DialogDescription>
          </div>
        </div>

        {rel?.method === "package" && status !== "error" && status !== "handoff" && (
          <p className="mx-5 mb-3 rounded-md border border-line bg-bg-0 px-3 py-2 text-[12px] text-fg-muted">
            Quarry was installed as a system package, so the update opens in your software installer, which may ask for your password.
          </p>
        )}
        {rel?.method === "installer" && status !== "error" && (
          <p className="mx-5 mb-3 rounded-md border border-line bg-bg-0 px-3 py-2 text-[12px] text-fg-muted">
            Quarry is installed in a protected folder, so the update runs the installer: Quarry closes, updates and reopens by itself.
            Windows may ask for permission.
          </p>
        )}
        <div className="max-h-[300px] overflow-auto border-y border-line bg-bg-0 px-5 py-3">
          {rel?.notes ? <Notes text={rel.notes} /> : <p className="text-[13px] text-fg-subtle">No release notes.</p>}
        </div>

        {(busy || status === "ready" || status === "handoff" || status === "error") && (
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
            {status === "handoff" && (
              <p className="flex items-start gap-2 text-[13px]">
                <CheckCircle aria-hidden weight="fill" className="mt-0.5 size-4 shrink-0 text-run" />
                <span>Your software installer opened with the new version. Finish installing there, then restart Quarry.</span>
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
            {status === "ready" || status === "handoff" ? (
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
