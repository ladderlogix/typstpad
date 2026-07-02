# TypstPad

A self-hosted collaborative [Typst](https://typst.app) document editor — "Overleaf for Typst".

## Features

- **Real-time collaboration** — multiple editors on the same document with live cursors and
  presence (Yjs CRDT, merges never conflict)
- **Live preview** — Typst compiled to WASM renders in your browser as you type; errors and
  warnings shown with click-to-jump; approximate scroll sync (preview follows your cursor,
  double-click the preview to jump the editor); fonts and Typst Universe packages are served
  and cached by your own server, so browsers need no internet access
- **SSO** — generic OIDC (Keycloak, Authentik, Google, Azure AD, …) plus built-in
  email/password accounts; the first registered user becomes admin
- **Version history** — automatic snapshots while you work, named versions, visual diffs and
  one-click restore (restores merge safely into live editing sessions)
- **Suggestions mode** — editors toggle "Suggesting" and type directly: insertions appear as
  underlined proposed text, deletions as strikethrough, coalesced into reviewable tracked
  changes; a selection-based Suggest dialog covers the read-only suggester role (enforced
  server-side)
- **Comments** — threaded comments anchored to text ranges that follow the text as it moves
- **Sharing** — invite by email with roles (owner / editor / suggester / viewer) or create
  share links
- **Templates** — built-in report/letter/résumé/slides templates; publish any project as a
  template
- **AI integration** — REST API, **MCP server** (streamable HTTP + stdio) and CLI so agents
  like Claude can read, edit (through the CRDT — safe with live editors), suggest, comment
  and compile
- **PDF export** — in-browser or server-side via the native typst compiler
- **Spell check** — optional per-user toggle using the browser's built-in dictionary
  (red squiggles + right-click corrections; no dictionary to bundle)
- Project search, duplication, image/asset upload via drag & drop, admin panel

## Quick start

```bash
cp .env.example .env   # fill in secrets: openssl rand -hex 32
docker compose up -d --build
# open http://<host>:8080 and register — the first account becomes admin
```

## AI / CLI access

Create an API token under **Settings → API tokens**, then either:

```bash
# MCP over HTTP (e.g. Claude Code):
claude mcp add typstpad http://<host>:8080/api/mcp \
  --transport http --header "Authorization: Bearer tfp_..."

# or the CLI (also provides an MCP stdio transport):
typstpad login --url http://<host>:8080 --token tfp_...
typstpad projects ls
typstpad pull <project-id> ./mydoc && typstpad push <project-id> ./mydoc
typstpad compile <project-id> -o out.pdf
typstpad watch <project-id>          # recompile on every remote change
typstpad mcp                         # MCP server over stdio
```

MCP tools: `list_projects`, `list_files`, `read_file`, `apply_edit`, `propose_suggestion`,
`add_comment`, `get_compile_diagnostics`, `get_version_history`, `create_version`.

## SSO (OIDC)

Set `OIDC_ISSUER`, `OIDC_CLIENT_ID`, `OIDC_CLIENT_SECRET` in `.env` and register
`${PUBLIC_URL}/api/auth/oidc/callback` as the redirect URI with your identity provider.
Accounts are linked by verified email when one already exists.

## Architecture

```
browser (React + CodeMirror 6 + Yjs + typst.ts WASM preview)
   │ REST + SSE          │ websocket /collab
   ▼                     ▼ (reverse-proxied by app)
app (Go: API, auth, versions, typst compile, MCP)   collab (Node: Hocuspocus sync + doc-ops)
   └── postgres 16 ──────┘        (internal network, only :8080 published)
```

Dev: `make collab` (sidecar), `go run ./cmd/typstpad serve`, `cd web && npm run dev` (Vite
proxies /api and /collab to :8080). Deploy: `make deploy`.
