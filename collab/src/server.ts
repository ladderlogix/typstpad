// TypstPad collab sidecar: Hocuspocus websocket sync + an internal "doc-ops"
// HTTP API that lets the Go backend read live documents, apply CRDT edits
// (AI/CLI/restore paths) and encode/decode relative-position anchors for
// suggestions and comments.
import type { IncomingMessage, ServerResponse } from "node:http";
import { URL } from "node:url";
import { Server } from "@hocuspocus/server";
import jwt from "jsonwebtoken";
import * as Y from "yjs";
import DiffMatchPatch from "diff-match-patch";

const PORT = parseInt(process.env.COLLAB_PORT ?? "8090", 10);
const SECRET = process.env.COLLAB_SECRET ?? "";
const APP_URL = (process.env.APP_INTERNAL_URL ?? "http://localhost:8080").replace(/\/$/, "");

if (!SECRET) {
  console.error("COLLAB_SECRET is required");
  process.exit(1);
}

const TEXT_KEY = "content";
const dmp = new DiffMatchPatch();

interface TokenClaims {
  doc: string;
  name: string;
  color: string;
  mode: "rw" | "ro";
  sub: string;
}

async function appFetch(path: string, init: RequestInit = {}): Promise<Response> {
  const headers = new Headers(init.headers);
  headers.set("X-Internal-Secret", SECRET);
  const resp = await fetch(APP_URL + path, { ...init, headers });
  if (!resp.ok) {
    throw new Error(`app ${init.method ?? "GET"} ${path}: ${resp.status} ${await resp.text()}`);
  }
  return resp;
}

const server = new Server({
  port: PORT,
  debounce: 2000,
  maxDebounce: 10000,
  quiet: true,
  stopOnSignals: true,

  async onAuthenticate({ token, documentName, connectionConfig }) {
    let claims: TokenClaims;
    try {
      claims = jwt.verify(token, SECRET, { algorithms: ["HS256"] }) as TokenClaims;
    } catch {
      throw new Error("invalid token");
    }
    if (claims.doc !== documentName) {
      throw new Error("token is for a different document");
    }
    if (claims.mode !== "rw") {
      connectionConfig.readOnly = true;
    }
    return { user: { id: claims.sub, name: claims.name, color: claims.color } };
  },

  async onLoadDocument({ document, documentName }) {
    const resp = await appFetch(`/internal/ydoc/${documentName}`);
    const body = (await resp.json()) as { state: string | null; content?: string };
    if (body.state) {
      Y.applyUpdate(document, Buffer.from(body.state, "base64"));
    } else {
      // First load: seed the CRDT from the stored plain text.
      document.getText(TEXT_KEY).insert(0, body.content ?? "");
    }
    return document;
  },

  async onStoreDocument({ document, documentName }) {
    const state = Buffer.from(Y.encodeStateAsUpdate(document)).toString("base64");
    const text = document.getText(TEXT_KEY).toString();
    await appFetch(`/internal/ydoc/${documentName}`, {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ state, text }),
    });
  },

  // Plain-HTTP requests hit the doc-ops API. Rejecting the hook promise tells
  // Hocuspocus we already answered.
  async onRequest({ request, response }) {
    const url = new URL(request.url ?? "/", "http://internal");
    if (url.pathname === "/healthz") {
      json(response, 200, { status: "ok" });
      return Promise.reject();
    }
    if (url.pathname.startsWith("/docs/")) {
      await handleDocOps(request, response, url);
      return Promise.reject();
    }
  },
});

// ---- doc-ops API ----

function json(res: ServerResponse, status: number, body: unknown) {
  const data = JSON.stringify(body);
  res.writeHead(status, { "Content-Type": "application/json" });
  res.end(data);
}

async function readBody(req: IncomingMessage): Promise<any> {
  const chunks: Buffer[] = [];
  for await (const chunk of req) chunks.push(chunk as Buffer);
  const raw = Buffer.concat(chunks).toString("utf8");
  return raw ? JSON.parse(raw) : {};
}

/**
 * Runs fn against a live document. Uses a direct connection so loads/stores
 * go through the normal Hocuspocus persistence hooks and connected editors
 * receive changes immediately.
 */
async function withDoc<T>(docId: string, fn: (doc: Y.Doc) => T): Promise<T> {
  const conn = await server.hocuspocus.openDirectConnection(docId, {});
  try {
    let result!: T;
    await conn.transact((doc) => {
      result = fn(doc);
    });
    return result;
  } finally {
    await conn.disconnect();
  }
}

function applyMinimalDiff(ytext: Y.Text, next: string) {
  const current = ytext.toString();
  if (current === next) return;
  const diffs = dmp.diff_main(current, next);
  dmp.diff_cleanupSemantic(diffs);
  let index = 0;
  for (const [op, text] of diffs) {
    if (op === DiffMatchPatch.DIFF_EQUAL) {
      index += text.length;
    } else if (op === DiffMatchPatch.DIFF_DELETE) {
      ytext.delete(index, text.length);
    } else {
      ytext.insert(index, text);
      index += text.length;
    }
  }
}

async function handleDocOps(req: IncomingMessage, res: ServerResponse, url: URL): Promise<void> {
  if (req.headers["x-internal-secret"] !== SECRET) {
    return json(res, 401, { error: "unauthorized" });
  }
  const match = url.pathname.match(/^\/docs\/([0-9a-f-]+)\/(text|edit|content|relpos|abspos)$/);
  if (!match) {
    return json(res, 404, { error: "not found" });
  }
  const [, docId, op] = match;

  try {
    switch (op) {
      case "text": {
        if (url.searchParams.get("ifLoaded") && !server.hocuspocus.documents.has(docId)) {
          res.writeHead(204);
          res.end();
          return;
        }
        const text = await withDoc(docId, (doc) => doc.getText(TEXT_KEY).toString());
        return json(res, 200, { text });
      }
      case "edit": {
        const { from, to, insert = "", origin = "api" } = await readBody(req);
        if (typeof from !== "number" || typeof to !== "number" || from < 0 || to < from) {
          return json(res, 400, { error: "invalid range" });
        }
        await withDoc(docId, (doc) => {
          const ytext = doc.getText(TEXT_KEY);
          const len = ytext.length;
          const f = Math.min(from, len);
          const t = Math.min(to, len);
          doc.transact(() => {
            if (t > f) ytext.delete(f, t - f);
            if (insert) ytext.insert(f, insert);
          }, origin);
        });
        return json(res, 200, { ok: true });
      }
      case "content": {
        const body = await readBody(req);
        if (typeof body.text !== "string") {
          return json(res, 400, { error: "text required" });
        }
        await withDoc(docId, (doc) => {
          doc.transact(() => applyMinimalDiff(doc.getText(TEXT_KEY), body.text), "api");
        });
        return json(res, 200, { ok: true });
      }
      case "relpos": {
        const { from, to } = await readBody(req);
        if (typeof from !== "number" || typeof to !== "number" || from < 0 || to < from) {
          return json(res, 400, { error: "invalid range" });
        }
        const result = await withDoc(docId, (doc) => {
          const ytext = doc.getText(TEXT_KEY);
          if (to > ytext.length) {
            return null;
          }
          const start = Y.createRelativePositionFromTypeIndex(ytext, from, 0);
          const end = Y.createRelativePositionFromTypeIndex(ytext, to, -1);
          return {
            anchorStart: Buffer.from(Y.encodeRelativePosition(start)).toString("base64"),
            anchorEnd: Buffer.from(Y.encodeRelativePosition(end)).toString("base64"),
            slice: ytext.toString().slice(from, to),
          };
        });
        if (!result) return json(res, 400, { error: "range beyond end of document" });
        return json(res, 200, result);
      }
      case "abspos": {
        const body = await readBody(req);
        const anchors: string[] = body.anchors ?? [];
        const positions = await withDoc(docId, (doc) =>
          anchors.map((b64) => {
            try {
              const rel = Y.decodeRelativePosition(Buffer.from(b64, "base64"));
              const abs = Y.createAbsolutePositionFromRelativePosition(rel, doc);
              return abs ? abs.index : -1;
            } catch {
              return -1;
            }
          })
        );
        return json(res, 200, { positions });
      }
    }
  } catch (err) {
    console.error("doc-ops error", op, docId, err);
    if (!res.headersSent) {
      json(res, 500, { error: String(err) });
    }
  }
}

server.listen().then(() => {
  console.log(`typstpad-collab listening on :${PORT} (app: ${APP_URL})`);
});
