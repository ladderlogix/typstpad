// Renders suggestions (tracked changes) and comment ranges as CodeMirror
// decorations. Anchors are Yjs relative positions (base64) decoded against
// the live Y.Doc on every doc change, so they follow concurrent edits.
import { EditorView, Decoration, DecorationSet, WidgetType, ViewPlugin, ViewUpdate } from "@codemirror/view";
import { StateEffect, StateField } from "@codemirror/state";
import * as Y from "yjs";
import type { Comment, Suggestion } from "../api/client";

export interface AnnotationData {
  suggestions: Suggestion[];
  comments: Comment[];
}

export const setAnnotations = StateEffect.define<AnnotationData>();

const annotationState = StateField.define<AnnotationData>({
  create: () => ({ suggestions: [], comments: [] }),
  update(value, tr) {
    for (const e of tr.effects) {
      if (e.is(setAnnotations)) return e.value;
    }
    return value;
  },
});

function b64ToBytes(b64: string): Uint8Array {
  const bin = atob(b64);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
}

export function resolveAnchor(ydoc: Y.Doc, b64?: string): number | null {
  if (!b64) return null;
  try {
    const rel = Y.decodeRelativePosition(b64ToBytes(b64));
    const abs = Y.createAbsolutePositionFromRelativePosition(rel, ydoc);
    return abs ? abs.index : null;
  } catch {
    return null;
  }
}

class GhostTextWidget extends WidgetType {
  constructor(
    private text: string,
    private color: string,
    private suggestionId: string
  ) {
    super();
  }
  eq(other: GhostTextWidget) {
    return other.text === this.text && other.suggestionId === this.suggestionId;
  }
  toDOM() {
    const span = document.createElement("span");
    span.className = "tp-suggest-insert";
    span.style.setProperty("--tp-author-color", this.color);
    span.textContent = this.text;
    span.dataset.suggestion = this.suggestionId;
    span.title = "Proposed insertion — click to review";
    return span;
  }
  ignoreEvent() {
    return false;
  }
}

export interface AnnotationCallbacks {
  onSelectSuggestion?: (id: string) => void;
  onSelectComment?: (id: string) => void;
}

export function annotationsExtension(ydoc: Y.Doc, callbacks: AnnotationCallbacks = {}) {
  const plugin = ViewPlugin.fromClass(
    class {
      decorations: DecorationSet;
      constructor(view: EditorView) {
        this.decorations = this.build(view);
      }
      update(update: ViewUpdate) {
        const hasEffect = update.transactions.some((tr) => tr.effects.some((e) => e.is(setAnnotations)));
        if (update.docChanged || hasEffect) {
          this.decorations = this.build(update.view);
        }
      }
      build(view: EditorView): DecorationSet {
        const { suggestions, comments } = view.state.field(annotationState);
        const docLen = view.state.doc.length;
        const clamp = (n: number) => Math.max(0, Math.min(n, docLen));
        const ranges: { from: number; to: number; deco: Decoration }[] = [];

        for (const sg of suggestions) {
          if (sg.status !== "open") continue;
          const start = resolveAnchor(ydoc, sg.anchorStart);
          if (start === null) continue;
          const from = clamp(start);
          if (sg.type === "insert" && !sg.anchorEnd) {
            // pending text (dialog/API suggestion): ghost widget at the point
            ranges.push({
              from,
              to: from,
              deco: Decoration.widget({
                widget: new GhostTextWidget(sg.insertedText ?? "", sg.authorColor, sg.id),
                side: 1,
              }),
            });
            continue;
          }
          if (sg.type === "insert") {
            // inline suggestion: the text is real, mark it as proposed
            const end = resolveAnchor(ydoc, sg.anchorEnd);
            if (end === null) continue;
            const to = clamp(end);
            if (to > from) {
              ranges.push({
                from,
                to,
                deco: Decoration.mark({
                  class: "tp-suggest-insert",
                  attributes: {
                    "data-suggestion": sg.id,
                    style: `--tp-author-color: ${sg.authorColor}`,
                    title: "Proposed insertion — click to review",
                  },
                }),
              });
            }
            continue;
          }
          const end = resolveAnchor(ydoc, sg.anchorEnd);
          if (end === null) continue;
          const to = clamp(end);
          if (to > from) {
            ranges.push({
              from,
              to,
              deco: Decoration.mark({
                class: "tp-suggest-delete",
                attributes: { "data-suggestion": sg.id, title: "Proposed deletion — click to review" },
              }),
            });
          }
          if (sg.type === "replace") {
            ranges.push({
              from: to,
              to,
              deco: Decoration.widget({
                widget: new GhostTextWidget(sg.insertedText ?? "", sg.authorColor, sg.id),
                side: 1,
              }),
            });
          }
        }

        for (const c of comments) {
          if (c.status !== "open" || !c.anchorStart || !c.anchorEnd) continue;
          const start = resolveAnchor(ydoc, c.anchorStart);
          const end = resolveAnchor(ydoc, c.anchorEnd);
          if (start === null || end === null) continue;
          const from = clamp(start);
          const to = clamp(end);
          if (to > from) {
            ranges.push({
              from,
              to,
              deco: Decoration.mark({
                class: "tp-comment-range",
                attributes: { "data-comment": c.id },
              }),
            });
          }
        }

        ranges.sort((a, b) => a.from - b.from || a.to - b.to);
        return Decoration.set(
          ranges.map((r) => r.deco.range(r.from, r.to)),
          true
        );
      }
    },
    { decorations: (v) => v.decorations }
  );

  const clickHandler = EditorView.domEventHandlers({
    mousedown(event) {
      const target = (event.target as HTMLElement).closest<HTMLElement>("[data-suggestion],[data-comment]");
      if (!target) return false;
      if (target.dataset.suggestion) {
        callbacks.onSelectSuggestion?.(target.dataset.suggestion);
      } else if (target.dataset.comment) {
        callbacks.onSelectComment?.(target.dataset.comment);
      }
      return false;
    },
  });

  return [annotationState, plugin, clickHandler];
}
