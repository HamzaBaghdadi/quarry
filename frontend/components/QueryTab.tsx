"use client";

import { useCallback, useMemo, useRef, useState } from "react";
import type { ReactCodeMirrorRef } from "@uiw/react-codemirror";
import { Play, Stop, Database, LockSimple } from "@phosphor-icons/react";
import { api, newRunId, type Result } from "@/lib/api";
import { treeKey, useDriver, useStore, type Tab } from "@/lib/store";
import { Editor } from "./Editor";
import { ResultPanel } from "./ResultPanel";
import { Split } from "./Split";
import { Button, Kbd, mod } from "./ui";

export function QueryTab({ tab }: { tab: Tab }) {
  const conn = useStore((s) => s.connections.find((c) => c.id === tab.connId));
  const driver = useDriver(conn?.driver);
  const rootNodes = useStore((s) => s.tree[treeKey(tab.connId, [])]?.nodes);
  const tree = useStore((s) => s.tree);
  const updateTab = useStore((s) => s.updateTab);

  const [query, setQuery] = useState(tab.query ?? "");
  const [result, setResult] = useState<Result | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [running, setRunning] = useState(false);
  const runId = useRef("");
  const editor = useRef<ReactCodeMirrorRef>(null);
  const queryRef = useRef(query);
  queryRef.current = query;

  const scopes = useMemo(() => (rootNodes ?? []).filter((n) => n.kind === "database").map((n) => n.name), [rootNodes]);

  // Feed loaded table names to SQL autocomplete.
  const schema = useMemo(() => {
    const out: Record<string, string[]> = {};
    const prefix = tab.connId + "\u0000";
    for (const [k, e] of Object.entries(tree)) {
      if (!k.startsWith(prefix)) continue;
      for (const n of e.nodes ?? []) if (n.browsable) out[n.name] = [];
    }
    return out;
  }, [tree, tab.connId]);

  const run = useCallback(async () => {
    if (running) return;
    // Run the selection if there is one, otherwise the whole buffer.
    const view = editor.current?.view;
    const sel = view?.state.selection.main;
    const text = sel && !sel.empty ? view!.state.sliceDoc(sel.from, sel.to) : queryRef.current;
    if (!text.trim()) return;
    runId.current = newRunId();
    setRunning(true);
    setError(null);
    try {
      setResult(await api.execute(tab.connId, tab.scope, text, runId.current));
    } catch (e) {
      setResult(null);
      setError((e as Error).message);
    } finally {
      setRunning(false);
    }
  }, [running, tab.connId, tab.scope]);

  const cancel = () => api.cancel(runId.current);

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="flex h-10 shrink-0 items-center gap-2 border-b border-line bg-bg-1 px-2">
        {running ? (
          <Button variant="danger" size="sm" onClick={cancel} className="border border-danger/40">
            <Stop aria-hidden weight="fill" className="size-3.5" /> Stop
          </Button>
        ) : (
          <Button variant="run" size="sm" onClick={run} title={`Run (${mod()}+Enter). Runs the selection if any.`}>
            <Play aria-hidden weight="fill" className="size-3.5" /> Run
          </Button>
        )}
        <span className="hidden text-[11px] text-fg-subtle sm:inline">
          <Kbd>{mod()}</Kbd> <Kbd>Enter</Kbd>
        </span>

        {scopes.length > 0 && (
          <label className="ml-2 flex items-center gap-1.5 text-[12px] text-fg-muted">
            <Database aria-hidden className="size-3.5" />
            <span className="sr-only">Database</span>
            <select
              value={tab.scope}
              onChange={(e) => updateTab(tab.id, { scope: e.target.value })}
              className="h-7 max-w-48 cursor-pointer rounded-md border border-line bg-bg-0 px-1.5 text-[12px] text-fg hover:border-line-strong focus:border-accent focus:outline-none"
            >
              <option value="">Default</option>
              {scopes.map((s) => (
                <option key={s} value={s}>
                  {s}
                </option>
              ))}
            </select>
          </label>
        )}
        {conn?.readOnly && (
          <span className="ml-auto flex items-center gap-1 rounded-full border border-warn/40 px-2 py-0.5 text-[11px] text-warn" title="Writes are blocked on this connection">
            <LockSimple aria-hidden weight="fill" className="size-3" /> Read-only
          </span>
        )}
      </div>

      <Split
        direction="vertical"
        initial={0.42}
        min={0.12}
        max={0.85}
        storageKey="editor"
        className="min-h-0 flex-1"
        first={
          <div className="min-h-0 flex-1 overflow-hidden">
            <Editor
              ref={editor}
              value={query}
              onChange={setQuery}
              language={driver?.language ?? "sql"}
              driver={conn?.driver ?? ""}
              onRun={run}
              schema={schema}
            />
          </div>
        }
        second={
          <ResultPanel
            result={result}
            error={error}
            loading={running}
            onCancel={cancel}
            name={tab.title}
            emptyHint={
              <span>
                Press <Kbd>{mod()}</Kbd> <Kbd>Enter</Kbd> to run. Select text to run just that part.
              </span>
            }
          />
        }
      />
    </div>
  );
}
