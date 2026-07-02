// Lightweight Typst syntax highlighting as a CM6 StreamLanguage, plus a
// static autocomplete source for common stdlib functions. A full Lezer/WASM
// grammar (codemirror-lang-typst) can be swapped in behind this facade later.
import { StreamLanguage, LanguageSupport } from "@codemirror/language";
import { autocompletion, completeFromList, snippetCompletion, type Completion } from "@codemirror/autocomplete";
import { hoverTooltip } from "@codemirror/view";

interface TypstState {
  inCode: number; // depth of #{...} / code brackets
  inMathDepth: number;
}

const keywords = new Set([
  "let", "set", "show", "if", "else", "for", "in", "while", "break", "continue",
  "return", "import", "include", "as", "none", "auto", "true", "false", "not", "and", "or", "context",
]);

const typstStream = StreamLanguage.define<TypstState>({
  name: "typst",
  startState: () => ({ inCode: 0, inMathDepth: 0 }),
  token(stream, state) {
    // comments
    if (stream.match("//")) {
      stream.skipToEnd();
      return "comment";
    }
    if (stream.match("/*")) {
      while (!stream.eol()) {
        if (stream.match("*/")) return "comment";
        stream.next();
      }
      return "comment";
    }
    // math mode
    if (stream.peek() === "$") {
      stream.next();
      state.inMathDepth = state.inMathDepth > 0 ? state.inMathDepth - 1 : state.inMathDepth + 1;
      return "keyword";
    }
    if (state.inMathDepth > 0) {
      stream.next();
      return "string";
    }
    // strings
    if (stream.peek() === '"') {
      stream.next();
      while (!stream.eol()) {
        const c = stream.next();
        if (c === "\\") stream.next();
        else if (c === '"') break;
      }
      return "string";
    }
    // headings
    if (stream.sol() && stream.match(/^=+\s/)) {
      stream.skipToEnd();
      return "heading";
    }
    // labels & references
    if (stream.match(/^<[\w:.-]+>/)) return "labelName";
    if (stream.match(/^@[\w:.-]+/)) return "link";
    // hash expressions: #func, #{ ... }, #import etc.
    if (stream.peek() === "#") {
      stream.next();
      if (stream.match(/^[a-zA-Z_][\w-]*/)) {
        const word = stream.current().slice(1);
        return keywords.has(word) ? "keyword" : "function";
      }
      return "keyword";
    }
    // numbers with optional units
    if (stream.match(/^\d+(\.\d+)?(pt|mm|cm|in|em|fr|%|deg|rad)?/)) return "number";
    // emphasis markers
    if (stream.match(/^\*[^*]+\*/)) return "strong";
    if (stream.match(/^_[^_]+_/)) return "emphasis";
    // raw/code spans
    if (stream.match(/^`[^`]*`/)) return "monospace";
    // words / identifiers
    if (stream.match(/^[a-zA-Z_][\w-]*/)) {
      const word = stream.current();
      if (keywords.has(word)) return "keyword";
      // function call in code context
      if (stream.peek() === "(") return "function";
      return null;
    }
    stream.next();
    return null;
  },
  languageData: {
    commentTokens: { line: "//", block: { open: "/*", close: "*/" } },
  },
});

const plainCompletions: Completion[] = [
  // text & layout
  "text", "par", "page", "heading", "strong", "emph", "raw", "label", "ref",
  "caption", "stack", "align", "block", "box",
  "pad", "place", "line", "rect", "circle", "ellipse", "polygon", "path",
  "columns", "colbreak", "pagebreak", "linebreak", "parbreak", "v", "h", "hide",
  "list", "enum", "terms", "outline", "bibliography", "quote",
  // math & code
  "numbering", "counter", "locate", "query", "measure", "context", "state",
  "calc.abs", "calc.ceil", "calc.floor", "calc.max", "calc.min", "calc.pow", "calc.round", "calc.sqrt",
  "lorem", "datetime.today", "emoji",
  // set-rule targets
  "set text", "set page", "set par", "set heading", "set list", "set enum", "set table",
  "show heading", "show link", "show raw",
  // control
  "let", "if", "else", "for", "while", "import", "include",
].map((label) => ({ label, type: label.includes(" ") ? "keyword" : "function" }));

// Templated inserts. ${} marks tab stops the user fills in after accepting.
const snippetCompletions: Completion[] = [
  snippetCompletion('figure(\n  image("${path}"),\n  caption: [${caption}],\n)', {
    label: "figure", detail: "figure with image", type: "function",
  }),
  snippetCompletion('image("${path}", width: ${80%})', { label: "image", detail: "image()", type: "function" }),
  snippetCompletion('table(\n  columns: ${2},\n  [${a}], [${b}],\n)', { label: "table", detail: "table", type: "function" }),
  snippetCompletion('grid(\n  columns: ${2},\n  [${a}], [${b}],\n)', { label: "grid", detail: "grid", type: "function" }),
  snippetCompletion('link("${url}")[${text}]', { label: "link", detail: "hyperlink", type: "function" }),
  snippetCompletion('cite(<${key}>)', { label: "cite", detail: "citation", type: "function" }),
  snippetCompletion('footnote[${text}]', { label: "footnote", detail: "footnote", type: "function" }),
  snippetCompletion('let ${name} = ${value}', { label: "let", detail: "binding", type: "keyword" }),
  snippetCompletion('for ${x} in ${range(10)} [\n  ${body}\n]', { label: "for", detail: "loop", type: "keyword" }),
  snippetCompletion('if ${cond} [\n  ${body}\n]', { label: "if", detail: "conditional", type: "keyword" }),
  snippetCompletion('import "${file}": ${names}', { label: "import", detail: "import module", type: "keyword" }),
];

// Common math-mode symbols (typed as words inside $ … $).
const mathSymbols: Completion[] = [
  "alpha", "beta", "gamma", "delta", "epsilon", "theta", "lambda", "mu", "pi", "rho",
  "sigma", "tau", "phi", "psi", "omega", "Gamma", "Delta", "Theta", "Lambda", "Pi", "Sigma", "Omega",
  "arrow.r", "arrow.l", "arrow.t", "arrow.b", "arrow.l.r", "arrow.r.double",
  "times", "div", "plus.minus", "dot", "dots.h", "dots.v", "star", "infinity",
  "sum", "product", "integral", "diff", "partial", "nabla", "in", "subset", "union", "sect",
  "lt.eq", "gt.eq", "eq.not", "approx", "prop", "and", "or", "not", "forall", "exists",
].map((label) => ({ label, type: "constant" }));

const docs: Record<string, string> = {
  figure: "A figure with an optional caption; auto-numbered and referenceable.",
  image: 'image("path", width: …) — embed a raster or vector image.',
  table: "A grid of cells with column tracks, strokes and alignment.",
  grid: "Low-level layout grid (like table without default strokes).",
  link: 'link(dest)[body] — a hyperlink to a URL or a label.',
  ref: "ref(<label>) — a cross-reference to a labelled element.",
  cite: "cite(<key>) — a bibliography citation.",
  heading: "A section heading (also written with leading = signs).",
  set: "A set rule: change default properties, e.g. #set text(size: 12pt).",
  show: "A show rule: transform elements, e.g. #show heading: it => …",
  let: "Bind a variable or define a function.",
  outline: "Generate a table of contents from headings.",
  pagebreak: "Start a new page.",
  bibliography: 'bibliography("refs.bib") — the reference list.',
};

const typstHover = hoverTooltip((view, pos) => {
  const { text, from } = view.state.doc.lineAt(pos);
  const rel = pos - from;
  const re = /[#]?[a-zA-Z][\w.]*/g;
  let m: RegExpExecArray | null;
  while ((m = re.exec(text))) {
    const start = m.index;
    const end = start + m[0].length;
    if (rel < start || rel > end) continue;
    const name = m[0].replace(/^#/, "").split(".")[0];
    const doc = docs[name];
    if (!doc) return null;
    return {
      pos: from + start,
      end: from + end,
      create() {
        const dom = document.createElement("div");
        dom.className = "tp-hover";
        dom.textContent = doc;
        return { dom };
      },
    };
  }
  return null;
});

export function typstLanguage(): (LanguageSupport | ReturnType<typeof autocompletion> | typeof typstHover)[] {
  return [
    new LanguageSupport(typstStream),
    typstHover,
    autocompletion({
      override: [
        (ctx) => {
          const word = ctx.matchBefore(/[#]?[\w.]*$/);
          if (!word || (word.from === word.to && !ctx.explicit)) return null;
          return completeFromList([...snippetCompletions, ...plainCompletions, ...mathSymbols])(ctx);
        },
      ],
    }),
  ];
}
