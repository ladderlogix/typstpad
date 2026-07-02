import { useEffect, useRef } from "react";
import { EditorState } from "@codemirror/state";
import {
  EditorView,
  keymap,
  lineNumbers,
  highlightActiveLine,
  highlightActiveLineGutter,
  highlightSpecialChars,
  drawSelection,
  rectangularSelection,
  crosshairCursor,
} from "@codemirror/view";
import { defaultKeymap, indentWithTab } from "@codemirror/commands";
import { indentOnInput, bracketMatching, syntaxHighlighting, defaultHighlightStyle } from "@codemirror/language";
import { closeBrackets, closeBracketsKeymap, completionKeymap } from "@codemirror/autocomplete";
import { searchKeymap, highlightSelectionMatches } from "@codemirror/search";
import { yCollab, yUndoManagerKeymap } from "y-codemirror.next";
import * as Y from "yjs";
import type { Awareness } from "y-protocols/awareness";
import { typstLanguage } from "./language";
import { annotationsExtension, setAnnotations, type AnnotationData, type AnnotationCallbacks } from "./annotations";

interface Props {
  ydoc: Y.Doc;
  ytext: Y.Text;
  awareness: Awareness;
  readOnly: boolean;
  annotations: AnnotationData;
  callbacks: AnnotationCallbacks;
  onViewReady?: (view: EditorView) => void;
}

export default function CodeEditor({ ydoc, ytext, awareness, readOnly, annotations, callbacks, onViewReady }: Props) {
  const containerRef = useRef<HTMLDivElement>(null);
  const viewRef = useRef<EditorView | null>(null);

  useEffect(() => {
    if (!containerRef.current) return;
    const undoManager = new Y.UndoManager(ytext);
    const state = EditorState.create({
      doc: ytext.toString(),
      extensions: [
        lineNumbers(),
        highlightActiveLineGutter(),
        highlightSpecialChars(),
        drawSelection(),
        indentOnInput(),
        bracketMatching(),
        closeBrackets(),
        highlightActiveLine(),
        highlightSelectionMatches(),
        rectangularSelection(),
        crosshairCursor(),
        syntaxHighlighting(defaultHighlightStyle, { fallback: true }),
        EditorView.lineWrapping,
        keymap.of([
          ...closeBracketsKeymap,
          ...defaultKeymap,
          ...searchKeymap,
          ...completionKeymap,
          ...yUndoManagerKeymap,
          indentWithTab,
        ]),
        ...typstLanguage(),
        yCollab(ytext, awareness, { undoManager }),
        annotationsExtension(ydoc, callbacks),
        EditorState.readOnly.of(readOnly),
        EditorView.editable.of(!readOnly),
      ],
    });
    const view = new EditorView({ state, parent: containerRef.current });
    viewRef.current = view;
    onViewReady?.(view);
    return () => {
      view.destroy();
      viewRef.current = null;
      undoManager.destroy();
    };
  }, [ytext, readOnly]);

  useEffect(() => {
    viewRef.current?.dispatch({ effects: setAnnotations.of(annotations) });
  }, [annotations]);

  return <div ref={containerRef} className="h-full overflow-hidden" />;
}
