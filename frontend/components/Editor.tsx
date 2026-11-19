"use client";

import CodeMirror, { keymap, Prec, type ReactCodeMirrorRef } from "@uiw/react-codemirror";
import { sql, PostgreSQL, MySQL, MSSQL, SQLite, StandardSQL } from "@codemirror/lang-sql";
import { javascript } from "@codemirror/lang-javascript";
import { HighlightStyle, syntaxHighlighting } from "@codemirror/language";
import { tags as t } from "@lezer/highlight";
import { forwardRef, useMemo } from "react";

// Token colours tuned for both themes (they sit on --bg-1).
const highlight = HighlightStyle.define([
  { tag: [t.keyword, t.operatorKeyword, t.modifier], color: "#7aa2f7", fontWeight: "600" },
  { tag: [t.string, t.special(t.string)], color: "#9ece6a" },
  { tag: [t.number, t.bool, t.null], color: "#ff9e64" },
  { tag: [t.comment, t.lineComment, t.blockComment], color: "#7c8aa0", fontStyle: "italic" },
  { tag: [t.function(t.variableName), t.function(t.propertyName)], color: "#7dcfff" },
  { tag: [t.typeName, t.className], color: "#bb9af7" },
  { tag: [t.propertyName], color: "#73daca" },
  { tag: [t.operator, t.punctuation], color: "var(--fg-muted)" },
]);

const dialects = { postgres: PostgreSQL, cockroachdb: PostgreSQL, mysql: MySQL, mariadb: MySQL, tidb: MySQL, mssql: MSSQL, sqlite: SQLite };

export const Editor = forwardRef<
  ReactCodeMirrorRef,
  {
    value: string;
    onChange: (v: string) => void;
    language: "sql" | "mongodb" | "redis";
    driver: string;
    onRun: () => void;
    schema?: Record<string, string[]>;
  }
>(function Editor({ value, onChange, language, driver, onRun, schema }, ref) {
  const extensions = useMemo(() => {
    const run = Prec.highest(
      keymap.of([
        { key: "Mod-Enter", run: () => (onRun(), true) },
        { key: "Shift-Enter", run: () => (onRun(), true) },
      ]),
    );
    const lang =
      language === "sql"
        ? [sql({ dialect: dialects[driver as keyof typeof dialects] ?? StandardSQL, schema, upperCaseKeywords: true })]
        : language === "mongodb"
          ? [javascript()]
          : [];
    return [run, syntaxHighlighting(highlight), ...lang];
  }, [language, driver, onRun, schema]);

  return (
    <CodeMirror
      ref={ref}
      value={value}
      onChange={onChange}
      theme="none"
      height="100%"
      className="h-full"
      extensions={extensions}
      basicSetup={{ foldGutter: false, highlightActiveLineGutter: true, autocompletion: true, bracketMatching: true }}
      aria-label="Query editor"
    />
  );
});
