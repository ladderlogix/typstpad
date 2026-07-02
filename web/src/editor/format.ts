import type { EditorView } from "@codemirror/view";

// Editing primitives for the formatting toolbar. All operate on the main
// selection only (a toolbar acting on multi-cursors would be surprising).

/** Wrap the selection in `before`/`after`; if empty, place the cursor between them. */
export function wrapSelection(view: EditorView, before: string, after: string) {
  const { from, to } = view.state.selection.main;
  const selected = view.state.sliceDoc(from, to);
  view.dispatch({
    changes: { from, to, insert: before + selected + after },
    selection: selected
      ? { anchor: from + before.length, head: from + before.length + selected.length }
      : { anchor: from + before.length },
    scrollIntoView: true,
  });
  view.focus();
}

/** Prepend `prefix` to every line touched by the selection (toggles off if all already have it). */
export function prefixLines(view: EditorView, prefix: string) {
  const { doc } = view.state;
  const { from, to } = view.state.selection.main;
  const first = doc.lineAt(from).number;
  const last = doc.lineAt(to).number;
  const lines = [];
  for (let n = first; n <= last; n++) lines.push(doc.line(n));
  const allPrefixed = lines.every((l) => l.text.startsWith(prefix));
  const changes = lines.map((l) =>
    allPrefixed
      ? { from: l.from, to: l.from + prefix.length, insert: "" }
      : { from: l.from, insert: prefix }
  );
  view.dispatch({ changes, scrollIntoView: true });
  view.focus();
}

/**
 * Replace the selection with `text`. If `selectFrom`/`selectTo` are given
 * (offsets within `text`), that slice becomes the new selection — handy for
 * dropping the cursor onto a placeholder like the url in a link snippet.
 */
export function insertSnippet(view: EditorView, text: string, selectFrom?: number, selectTo?: number) {
  const { from, to } = view.state.selection.main;
  view.dispatch({
    changes: { from, to, insert: text },
    selection:
      selectFrom != null
        ? { anchor: from + selectFrom, head: from + (selectTo ?? selectFrom) }
        : { anchor: from + text.length },
    scrollIntoView: true,
  });
  view.focus();
}

export interface OutlineItem {
  level: number;
  title: string;
  line: number;
  from: number;
}

/** Extract the Typst heading outline (`= `, `== `, …) with absolute offsets. */
export function parseOutline(text: string): OutlineItem[] {
  const out: OutlineItem[] = [];
  let pos = 0;
  let lineNo = 0;
  for (const line of text.split("\n")) {
    lineNo++;
    const m = /^(=+)\s+(\S.*)$/.exec(line);
    if (m) out.push({ level: m[1].length, title: m[2].trim(), line: lineNo, from: pos });
    pos += line.length + 1;
  }
  return out;
}

/** Words / characters for the status bar, ignoring leading/trailing whitespace. */
export function wordStats(text: string): { words: number; chars: number } {
  const trimmed = text.trim();
  return {
    words: trimmed ? trimmed.split(/\s+/).length : 0,
    chars: text.length,
  };
}
