// Lightweight Typst syntax highlighting as a CM6 StreamLanguage, plus a
// static autocomplete source for common stdlib functions. A full Lezer/WASM
// grammar (codemirror-lang-typst) can be swapped in behind this facade later.
import { StreamLanguage, LanguageSupport } from "@codemirror/language";
import { autocompletion, completeFromList, type Completion } from "@codemirror/autocomplete";

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

const stdlibCompletions: Completion[] = [
  // text & layout
  "text", "par", "page", "heading", "strong", "emph", "raw", "link", "label", "ref",
  "figure", "caption", "image", "table", "grid", "stack", "align", "block", "box",
  "pad", "place", "line", "rect", "circle", "ellipse", "polygon", "path",
  "columns", "colbreak", "pagebreak", "linebreak", "parbreak", "v", "h", "hide",
  "list", "enum", "terms", "outline", "bibliography", "cite", "footnote", "quote",
  // math & code
  "numbering", "counter", "locate", "query", "measure", "context", "state",
  "calc.abs", "calc.ceil", "calc.floor", "calc.max", "calc.min", "calc.pow", "calc.round", "calc.sqrt",
  "lorem", "datetime.today", "sym.arrow", "emoji",
  // set-rule targets
  "set text", "set page", "set par", "set heading", "set list", "set enum", "set table",
  "show heading", "show link", "show raw",
  // control
  "let", "if", "else", "for", "while", "import", "include",
].map((label) => ({ label, type: label.includes(" ") ? "keyword" : "function" }));

export function typstLanguage(): (LanguageSupport | ReturnType<typeof autocompletion>)[] {
  return [
    new LanguageSupport(typstStream),
    autocompletion({
      override: [
        (ctx) => {
          const word = ctx.matchBefore(/[#]?[\w.]*$/);
          if (!word || (word.from === word.to && !ctx.explicit)) return null;
          return completeFromList(stdlibCompletions)(ctx);
        },
      ],
    }),
  ];
}
