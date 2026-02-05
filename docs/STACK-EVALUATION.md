# Tech Stack Evaluation: Agent-Forward Assessment

*Prepared by Claw for Phaedrus — 2026-02-05*

## Current Stack

| Component | Tool | Monthly Cost |
|-----------|------|-------------|
| Framework | Next.js + TypeScript | Free |
| Database | Convex | Free tier (then $25/mo) |
| Hosting | Vercel | $20/mo Pro + $20/additional seat |
| Auth | Clerk | Free tier (then $25/mo) |
| VCS + CI | GitHub + Actions | Free |

## Evaluation Criteria

For an **agent-operated** stack, what matters most:

1. **CLI/API completeness** — Can I do everything without a browser?
2. **LLM training data** — Do agents write good code for this tool?
3. **Simplicity** — Fewer abstractions = fewer agent mistakes
4. **No per-seat pricing** — Agents shouldn't cost seats
5. **Fast feedback loops** — Quick deploys, easy rollbacks
6. **Self-contained** — Less vendor dependency = more control

---

## Component-by-Component

### TypeScript — ✅ KEEP

**Verdict: No change needed.**

TypeScript is the ideal agent language right now. Massive training data, strong type system catches agent mistakes at compile time, and every modern tool supports it. Both Kimi K2.5 and Claude Code handle TS extremely well.

No language switch would improve our situation.

### Next.js — ⚠️ KEEP WITH RESERVATIONS

**Verdict: Keep for existing projects. Consider simpler alternatives for new ones.**

Next.js has enormous training data and agents write good Next.js code. But it's gotten complex — App Router, Server Components, server actions, middleware, route handlers, layouts, loading states. Lots of ways to do the same thing, which means agents sometimes pick the wrong pattern.

For new projects, consider:
- **Hono** (lightweight, API-first, deploys anywhere) for backend-heavy apps
- **Remix/React Router v7** (simpler mental model than Next.js App Router)
- **Plain React + Vite** for SPAs that don't need SSR

But migrating existing Next.js apps isn't worth it. The training data advantage is real.

### Convex — 🔴 RECONSIDER

**Verdict: Switch to Postgres (via Supabase or direct) for new projects.**

Convex is the weakest link in the stack from an agent perspective:

**Problems:**
- **Niche training data.** Convex has unique conventions (reactive queries, mutations, actions, validators) that agents frequently get wrong. Compare to Postgres/SQL which every model handles flawlessly.
- **Vendor lock-in.** Convex is a proprietary runtime. Your data model, query patterns, and business logic are deeply coupled to their system. No migration path to standard SQL.
- **CLI gaps.** `npx convex` covers dev/deploy/data/env, which is decent. But schema changes require code, and there's no way to programmatically manage projects or teams via CLI. Dashboard required for project creation, team management, and some configuration.
- **No local development option.** (Update: Convex added local dev in 2025, but it's not the same as having a standard Postgres you can inspect with psql.)
- **Testing is harder.** Convex's unique runtime means tests need their test backend. Standard DB tests are simpler.

**Better alternatives:**
- **Supabase** — Postgres + Auth + Storage + Realtime. Excellent CLI (`supabase`), full local dev stack via Docker, SQL migrations as files, massive agent training data for Postgres/SQL. Free tier is generous. $25/mo Pro.
- **Drizzle ORM + any Postgres** — If you don't need Supabase's extras. Agents write excellent Drizzle code.
- **Turso** (libSQL/SQLite) — For lightweight apps. Embedded-first, dirt cheap, great CLI.

**Migration cost:** High for existing Convex apps (rewrite queries). Worth it for new projects.

### Vercel — 🟡 RETHINK

**Verdict: Keep for now, but the pricing model is agent-hostile.**

**Good:**
- Excellent CLI (`vercel`). Deploy, env vars, domains, logs, rollbacks — all CLI.
- Preview deploys on PRs are genuinely useful.
- Git integration is smooth.
- Agents know Vercel well (huge training data).

**Bad:**
- **$20/seat for deploying members.** Adding me (Kaylee) as a team member to make PR preview deploys work costs $20/mo. Each sprite that needs deploy access would be another $20. This doesn't scale.
- **Viewer seats are free** but viewers can't deploy.
- **Build minutes are limited.** With 5 sprites opening PRs, preview deploys add up.

**Alternatives:**
- **Fly.io** — Already using for sprites. No per-seat pricing. CLI-first (`fly`). Can host Next.js apps. Pay for compute only. But: no native preview deploys (would need to build this ourselves).
- **Cloudflare Pages** — Free unlimited seats. Great CLI (`wrangler`). Preview deploys on PRs. But: Next.js support is limited (needs @cloudflare/next-on-pages adapter, some features don't work).
- **Railway** — Good CLI, no per-seat pricing, auto-deploys from GitHub, preview environments. $5/mo hobby plan, then usage-based. Simpler than Vercel.
- **Self-hosted on a VPS** (Peter Levels approach) — Full control, no per-seat costs, but more ops. A single Hetzner box ($5-20/mo) can host multiple apps. We'd need to set up CI/CD ourselves.

**My recommendation:** For existing Vercel projects, add me as a team member ($20/mo) and keep using Vercel. For new projects, evaluate **Railway** (simplest) or **Fly.io** (already in our ecosystem).

### Clerk — 🟡 RETHINK FOR NEW PROJECTS

**Verdict: Works but has agent friction. Consider Auth.js or Supabase Auth for new projects.**

**Problems:**
- **No CLI at all.** Zero. Everything is dashboard-managed — creating applications, configuring social logins, managing users, setting up webhooks. This is the most agent-unfriendly tool in the stack.
- **API exists but is limited.** REST API for user management (CRUD users, sessions). But initial setup, OAuth provider configuration, and UI customization all require the dashboard.
- **Per-app pricing.** Free tier (10K MAU), then $25/mo. Each new app needs separate dashboard setup.

**Alternatives:**
- **Auth.js (NextAuth)** — Code-based configuration, no dashboard needed. Agents can set up auth entirely through code. Supports all major providers. Free, self-hosted. The clear agent-first winner.
- **Supabase Auth** — If you go Supabase for DB, auth comes free. CLI manages most config. Good agent support.
- **Lucia Auth patterns** — (Library deprecated but the pattern of rolling your own session management on top of your DB is very agent-friendly.)

**My recommendation:** Auth.js for new projects. It's code, it's version controlled, agents handle it perfectly. No dashboard, no vendor, no per-app cost.

### GitHub + Actions — ✅ KEEP

**Verdict: Best in class. No change.**

- `gh` CLI is phenomenal. Issues, PRs, Actions, API — all CLI.
- GitHub Actions are YAML in the repo — fully version controlled, agents write them well.
- No per-seat costs for our usage level.
- Massive training data.

This is the strongest part of the stack.

---

## The Peter Levels Question

> Should we just run a VPS and do things more manually?

**It depends on what we're building.**

For **SaaS products** with users, auth, databases, and web UIs — managed services (Vercel/Supabase/etc.) save real ops time. Setting up TLS, auth, database backups, monitoring on a VPS is doable but it's work that doesn't ship product.

For **internal tools, APIs, and experiments** — a VPS is simpler and cheaper. Sprites can SSH in, deploy directly, no vendor friction.

**Hybrid approach:** Use managed services for customer-facing products (where reliability matters) and VPS/Fly.io for internal tools and experiments. Don't go pure Levels-mode unless you want to spend time on ops.

---

## Recommended Agent-Forward Stack

### For New Projects

| Component | Recommendation | Why |
|-----------|---------------|-----|
| Language | TypeScript | Training data, types catch mistakes |
| Framework | Next.js (simple apps) or Hono (APIs) | Familiarity vs simplicity tradeoff |
| Database | **Supabase** (Postgres) | CLI-first, standard SQL, massive training data |
| Auth | **Auth.js** | Pure code, no dashboard, free |
| Hosting | **Railway** or Fly.io | No per-seat, CLI-first, affordable |
| VCS + CI | GitHub + Actions | Best in class |

### For Existing Projects

Don't migrate unless something is actively broken. The cost of rewriting exceeds the benefit. Instead:

1. **Add Kaylee to Vercel** ($20/mo) — unblock PR preview deploys
2. **Keep Convex** for existing apps — migration cost too high
3. **Use Auth.js** for any new auth needs
4. **Evaluate Railway** for the next new project as a Vercel alternative

### Cost Comparison

**Current (per project):** Vercel $20 + Convex free + Clerk free = $20/mo base
**Recommended (per project):** Railway ~$5 + Supabase free + Auth.js free = ~$5/mo base
**Peter Levels (all projects):** Hetzner $10-20/mo total (but ops overhead)

---

## Summary

The biggest agent-friction points in the current stack are **Clerk** (no CLI, dashboard-only setup) and **Convex** (niche patterns, agents struggle with conventions). GitHub is perfect. Vercel is good but expensive for multi-agent teams. TypeScript and Next.js are solid.

For the next new project: **TypeScript + Next.js + Supabase + Auth.js + Railway + GitHub Actions.** Everything CLI-manageable, everything standard, no per-seat surprises.
