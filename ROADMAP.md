# TypstPad Roadmap

Planned improvements, roughly in priority order. Check an item when it's built,
deployed through the pipeline, and verified.

## P0 — Safety (do first)
- [x] Password reset for local accounts (email link via SES)
- [x] Self-service account settings (change display name, color, password)
- [x] Automated backups — nightly `pg_dump` + assets, 7-day local rotation (systemd timer)
      + off-box to Backblaze B2 (rclone, ~30-day remote retention)
- [x] Rate limiting (login / register / resend-verification / forgot-password / compile)
- [x] Abuse quotas (per-user project count, per-user asset storage)

## P1 — Production hardening
- [x] Compile memory limits — typst subprocess wrapped with `ulimit -v`
      (COMPILE_MAX_MEMORY_MB=2048); verified normal compiles unaffected
- [x] Automated tests + CI gate — Go unit tests for role checks, @mention parsing, zip
      import prefix/path handling, password + token hashing, and the diagnostics parser.
      The Docker build runs `go vet ./... && go test ./...` before producing the binary,
      so a red test fails the image build and the deploy hook rolls back.
      (Deeper DB-integration tests for collab merge / restore are still a future add.)
- [x] Deeper deploy health check — deploy hook checks `/api/auth/config` (routing + settings
      + JSON), not just the DB ping, before considering the deploy healthy (else rolls back)
- [x] Observability (metrics) — Prometheus + Grafana stack; app instrumented (HTTP
      traffic/latency/errors, compile rate/duration, business gauges, Go runtime); provisioned
      TypstPad dashboard. Grafana on localhost:3001 (SSH tunnel or a CF hostname).
      TODO: error tracking (Sentry) + uptime alerting are still open.
- [x] Admin server stats — users / projects / documents / templates / teams / active sessions
      / disk usage in the admin panel

## P2 — Collaboration UX
- [x] Notifications: in-app feed with a bell + unread badge (comment / @mention / project
      shared-with-you); email via SES on @mention and share. @handles match by email,
      email local-part, or name.
- [x] Typst language support in the editor — snippet completions (figure/image/table/
      grid/link/cite/for/if… insert templates with tab stops), stdlib + math-symbol
      completions, and hover tooltips documenting common functions. Diagnostics already
      come from the in-browser WASM compiler (error panel + squiggles). A full tinymist
      LSP (go-to-def, workspace-wide analysis) remains a possible future upgrade.
- [x] Editor niceties: formatting/symbol toolbar (bold/italic/headings/lists/link/math/
      figure/table/pagebreak), document outline panel (click-to-jump), live word/char count.
      TODO: find-and-replace across files (CM6 has single-file search built in).
- [x] Export/import: download the whole project (source + assets) as a zip; import a
      Typst project zip (nested-dir stripping, binary-asset detection, main-file autodetect)
- [x] Public read-only compiled share link — owner toggles a per-project public link in
      the Share dialog; anyone with it views the server-compiled PDF at /share/{token}
      with no account (rate-limited per IP; token retrievable so the URL stays copyable)

## P3 — Nice-to-haves
- [x] Team-scoped collections — a collection can belong to a team and is visible to all
      its members (create with a team in the New-collection dialog); rename/delete limited
      to the owner or a team admin
- [ ] Per-file permissions
- [x] Project favorites/starring (star on cards + Favorites filter); trash with restore
      (deletes go to a Trash view with restore / delete-forever; owner-only)
- [x] Admin audit log — records role changes, user/project deletions, sharing, and
      settings updates; viewer in the Admin panel (most recent first)
- [ ] Exact scroll-sync / jump-to-source (SyncTeX-style)
- [x] Mobile/responsive editor layout — dashboard grid + header collapse to one column on
      narrow screens; the editor's file-tree / editor / preview / side-panel stack vertically
      below the `lg` breakpoint and sit side-by-side above it; toolbars wrap.
- [x] Accessibility pass — aria-labels on icon-only controls (theme, bell, formatting),
      `role="toolbar"` on the format bar, focus-visible rings; page already sets lang + viewport.

---
Progress notes: see git history. Each batch is built on `main`, pushed to
`git.ics.red` (auto-deploys to typst.ics.red) and mirrored to GitHub.
