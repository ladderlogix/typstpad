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
- [ ] Automated tests (collab merge, suggestions accept/reject, version restore) + CI gate
      that runs them before the deploy hook proceeds
- [x] Deeper deploy health check — deploy hook checks `/api/auth/config` (routing + settings
      + JSON), not just the DB ping, before considering the deploy healthy (else rolls back)
- [ ] Observability: metrics, error tracking, uptime alerting
- [x] Admin server stats — users / projects / documents / templates / teams / active sessions
      / disk usage in the admin panel

## P2 — Collaboration UX
- [ ] Notifications: email on comment / @mention / project shared-with-you; in-app feed
- [ ] Real Typst language support via tinymist LSP (hover, completion, diagnostics)
- [ ] Editor niceties: formatting/symbol toolbar, document outline panel,
      find-and-replace across files, word count
- [ ] Export/import: download source or whole project as a zip; import a Typst project
- [ ] Public read-only compiled share link (view the PDF without an account)

## P3 — Nice-to-haves
- [ ] Team-scoped collections (shared groups)
- [ ] Per-file permissions
- [ ] Project favorites/starring; trash with restore
- [ ] Admin audit log
- [ ] Exact scroll-sync / jump-to-source (SyncTeX-style)
- [ ] Mobile/responsive editor layout
- [ ] Accessibility pass (keyboard nav, ARIA, screen readers)

---
Progress notes: see git history. Each batch is built on `main`, pushed to
`git.ics.red` (auto-deploys to typst.ics.red) and mirrored to GitHub.
