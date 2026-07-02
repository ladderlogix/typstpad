import type { EditorView } from "@codemirror/view";
import { insertSnippet, prefixLines, wrapSelection } from "./format";

// A compact row of Typst-markup shortcuts. `getView` returns the live
// EditorView (via the page's viewRef) so the buttons always act on the
// currently-open file.
export default function FormatToolbar({ getView }: { getView: () => EditorView | null }) {
  const act = (fn: (v: EditorView) => void) => () => {
    const v = getView();
    if (v) fn(v);
  };

  const table = `#table(
  columns: 2,
  [*Header 1*], [*Header 2*],
  [Cell], [Cell],
)`;

  return (
    <div
      role="toolbar"
      aria-label="Text formatting"
      className="flex flex-wrap items-center gap-0.5 border-b border-gray-200 dark:border-gray-800 bg-gray-50 dark:bg-gray-900 px-2 py-1"
    >
      <Btn title="Bold  *text*" onClick={act((v) => wrapSelection(v, "*", "*"))}>
        <span className="font-bold">B</span>
      </Btn>
      <Btn title="Italic  _text_" onClick={act((v) => wrapSelection(v, "_", "_"))}>
        <span className="italic">I</span>
      </Btn>
      <Btn title="Inline code  `text`" onClick={act((v) => wrapSelection(v, "`", "`"))}>
        <span className="font-mono">{"</>"}</span>
      </Btn>
      <Sep />
      <Btn title="Heading 1" onClick={act((v) => prefixLines(v, "= "))}>
        H1
      </Btn>
      <Btn title="Heading 2" onClick={act((v) => prefixLines(v, "== "))}>
        H2
      </Btn>
      <Btn title="Heading 3" onClick={act((v) => prefixLines(v, "=== "))}>
        H3
      </Btn>
      <Sep />
      <Btn title="Bullet list" onClick={act((v) => prefixLines(v, "- "))}>
        •
      </Btn>
      <Btn title="Numbered list" onClick={act((v) => prefixLines(v, "+ "))}>
        1.
      </Btn>
      <Btn title="Block quote" onClick={act((v) => prefixLines(v, "> "))}>
        ❝
      </Btn>
      <Sep />
      <Btn title="Link  #link(&quot;url&quot;)[text]" onClick={act((v) => insertSnippet(v, `#link("https://")[text]`, 7, 15))}>
        🔗
      </Btn>
      <Btn title="Inline math  $ … $" onClick={act((v) => wrapSelection(v, "$", "$"))}>
        <span className="italic">∑</span>
      </Btn>
      <Btn title="Figure with image" onClick={act((v) => insertSnippet(v, `#figure(\n  image("file.png", width: 80%),\n  caption: [Caption],\n)\n`, 15, 23))}>
        🖼
      </Btn>
      <Btn title="Table" onClick={act((v) => insertSnippet(v, table))}>
        ▦
      </Btn>
      <Btn title="Page break" onClick={act((v) => insertSnippet(v, "#pagebreak()\n"))}>
        ⤓
      </Btn>
    </div>
  );
}

function Btn({ children, title, onClick }: { children: React.ReactNode; title: string; onClick: () => void }) {
  return (
    <button
      type="button"
      title={title}
      aria-label={title}
      onClick={onClick}
      className="flex h-7 min-w-7 items-center justify-center rounded px-1.5 text-xs text-gray-600 focus-visible:ring-2 focus-visible:ring-indigo-500 dark:text-gray-300 hover:bg-gray-200 dark:hover:bg-gray-700"
    >
      {children}
    </button>
  );
}

function Sep() {
  return <span className="mx-1 h-4 w-px bg-gray-300 dark:bg-gray-700" />;
}
