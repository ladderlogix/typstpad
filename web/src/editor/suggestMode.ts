// Inline suggest mode (tracked changes): while enabled, the editor's own
// typing becomes suggestion records instead of plain edits.
//
// - Insertions DO enter the shared document (so they sync and render for
//   everyone) and are recorded as an "inline insert" suggestion range —
//   accept keeps the text, reject removes it.
// - Deletions are cancelled: the text stays, struck through, as a delete
//   suggestion — accept removes it, reject clears the mark.
//
// Bursts of adjacent keystrokes coalesce into one record via PATCH. Ranges
// are pinned client-side with Yjs relative positions so concurrent edits
// can't drift them. Only editors can use this mode (suggesters have a
// read-only transport and use the Suggest dialog instead).
import { Annotation, EditorState, Transaction, type Extension } from "@codemirror/state";
import { EditorView } from "@codemirror/view";
import * as Y from "yjs";
import { api, type Suggestion } from "../api/client";

// Deleted ranges (start-doc coordinates) the filter cancelled, carried from
// transactionFilter to the update listener.
const suggestIntent = Annotation.define<{ from: number; to: number }[]>();

const intentOf = (tr: Transaction) => {
  const deletions: { from: number; to: number }[] = [];
  tr.changes.iterChanges((fromA, toA) => {
    if (toA > fromA) deletions.push({ from: fromA, to: toA });
  });
  return deletions;
};

interface PendingRecord {
  id: string | null; // null until first flush creates it
  type: "insert" | "delete";
  start: Y.RelativePosition;
  end: Y.RelativePosition;
  creating: boolean;
  dirty: boolean;
}

export interface SuggestModeController {
  extension: Extension;
  setEnabled: (on: boolean) => void;
  isEnabled: () => boolean;
  /** Flush pending records immediately (mode toggle, unmount). */
  flush: () => Promise<void>;
}

export function createSuggestMode(opts: {
  ydoc: Y.Doc;
  ytext: Y.Text;
  fileId: string;
  onRecordsChanged: () => void;
}): SuggestModeController {
  const { ydoc, ytext, fileId, onRecordsChanged } = opts;
  let enabled = false;
  let pendingInsert: PendingRecord | null = null;
  let pendingDelete: PendingRecord | null = null;
  let flushTimer: number | null = null;

  const rel = (index: number, assoc: number) => Y.createRelativePositionFromTypeIndex(ytext, index, assoc);
  const abs = (pos: Y.RelativePosition): number | null => {
    const a = Y.createAbsolutePositionFromRelativePosition(pos, ydoc);
    return a ? a.index : null;
  };

  async function flushRecord(rec: PendingRecord | null) {
    if (!rec || !rec.dirty || rec.creating) return;
    const from = abs(rec.start);
    const to = abs(rec.end);
    if (from === null || to === null || to <= from) return;
    rec.dirty = false;
    try {
      if (rec.id === null) {
        rec.creating = true;
        const created = await api.post<Suggestion>(`/api/files/${fileId}/suggestions`, {
          type: rec.type,
          from,
          to,
          inline: rec.type === "insert",
        });
        rec.id = created.id;
        rec.creating = false;
        if (rec.dirty) await flushRecord(rec); // grew while creating
      } else {
        await api.patch(`/api/suggestions/${rec.id}`, { from, to });
      }
      onRecordsChanged();
    } catch (err) {
      rec.creating = false;
      console.error("suggest-mode flush failed", err);
    }
  }

  function scheduleFlush() {
    if (flushTimer) window.clearTimeout(flushTimer);
    flushTimer = window.setTimeout(() => {
      flushTimer = null;
      void flushRecord(pendingInsert);
      void flushRecord(pendingDelete);
    }, 500);
  }

  async function flushAll() {
    if (flushTimer) window.clearTimeout(flushTimer);
    flushTimer = null;
    await flushRecord(pendingInsert);
    await flushRecord(pendingDelete);
    pendingInsert = null;
    pendingDelete = null;
  }

  function trackInsert(from: number, to: number) {
    const pFrom = pendingInsert ? abs(pendingInsert.start) : null;
    const pTo = pendingInsert ? abs(pendingInsert.end) : null;
    if (pendingInsert && pFrom !== null && pTo !== null && from >= pFrom && from <= pTo) {
      // typing inside/at the edge of the pending range: extend
      if (to > pTo) pendingInsert.end = rel(to, -1);
      pendingInsert.dirty = true;
    } else {
      if (pendingInsert) void flushRecord(pendingInsert);
      pendingInsert = { id: null, type: "insert", start: rel(from, 0), end: rel(to, -1), creating: false, dirty: true };
    }
    scheduleFlush();
  }

  function trackDelete(from: number, to: number) {
    const pFrom = pendingDelete ? abs(pendingDelete.start) : null;
    const pTo = pendingDelete ? abs(pendingDelete.end) : null;
    if (pendingDelete && pFrom !== null && pTo !== null && to >= pFrom && from <= pTo) {
      // adjacent/overlapping: merge
      if (from < pFrom) pendingDelete.start = rel(from, 0);
      if (to > pTo) pendingDelete.end = rel(to, -1);
      pendingDelete.dirty = true;
    } else {
      if (pendingDelete) void flushRecord(pendingDelete);
      pendingDelete = { id: null, type: "delete", start: rel(from, 0), end: rel(to, -1), creating: false, dirty: true };
    }
    scheduleFlush();
  }

  const filter = EditorState.transactionFilter.of((tr) => {
    if (!enabled || !tr.docChanged) return tr;
    const event = tr.annotation(Transaction.userEvent);
    if (!event || !(event.startsWith("input") || event.startsWith("delete"))) return tr;

    let hasDeletion = false;
    tr.changes.iterChanges((fromA, toA) => {
      if (toA > fromA) hasDeletion = true;
    });
    if (!hasDeletion) return tr; // pure insertions pass through unchanged

    // Rebuild the transaction: keep would-be-deleted text, insert replacement
    // text after it. Change specs use start-doc coordinates; the selection
    // uses new-doc coordinates.
    const changes: { from: number; to: number; insert: string }[] = [];
    let deleteCursor: number | null = null;
    let insertEnd: number | null = null;
    let shift = 0;
    tr.changes.iterChanges((fromA, toA, _fromB, _toB, inserted) => {
      if (inserted.length) {
        changes.push({ from: toA, to: toA, insert: inserted.toString() });
        insertEnd = toA + shift + inserted.length;
        shift += inserted.length;
      }
      if (toA > fromA) {
        deleteCursor = event === "delete.forward" ? toA + shift : fromA + shift;
      }
    });
    const anchor = insertEnd ?? deleteCursor;
    return {
      changes,
      selection: anchor !== null ? { anchor } : undefined,
      annotations: [Transaction.userEvent.of(event), suggestIntent.of(intentOf(tr))],
      scrollIntoView: true,
    };
  });

  const listener = EditorView.updateListener.of((update) => {
    if (!enabled || !update.docChanged) return;
    for (const tr of update.transactions) {
      const event = tr.annotation(Transaction.userEvent);
      if (!event || !(event.startsWith("input") || event.startsWith("delete"))) continue;
      // Insertions in this (possibly rewritten) transaction.
      tr.changes.iterChanges((_fromA, _toA, fromB, toB) => {
        if (toB > fromB) trackInsert(fromB, toB);
      });
      // Deletions the filter cancelled (mapped through the kept insertions).
      const deletions = tr.annotation(suggestIntent);
      if (deletions) {
        for (const d of deletions) {
          trackDelete(tr.changes.mapPos(d.from, -1), tr.changes.mapPos(d.to, 1));
        }
      }
    }
  });

  return {
    extension: [filter, listener],
    setEnabled(on: boolean) {
      if (!on) void flushAll();
      enabled = on;
    },
    isEnabled: () => enabled,
    flush: flushAll,
  };
}
