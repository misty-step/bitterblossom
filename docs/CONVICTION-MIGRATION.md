# Conviction Stack Migration Plan

## Scope
Migrate `misty-step/conviction` from old stack to agent-forward stack:
- **Clerk → Auth.js** (auth)
- **Convex → Postgres via Supabase** (database)
- **Vercel → Fly.io** (hosting)

## Current State
- ~2,400 lines TypeScript (Next.js 16 + React 19)
- 4 Convex tables: users, theses, positions, thesisPositions
- Clerk auth with middleware, JWT identity
- Sentry error tracking, PostHog analytics (keep these)
- Tailwind CSS, Recharts (keep these)
- No existing users/data to migrate (prototype stage)

## Phase 1: Infrastructure Setup (Kaylee — agentic work)
1. Create Supabase project for Conviction
2. Create Fly.io app for Conviction
3. Set up Auth.js with GitHub provider (simplest to start)
4. Create Postgres schema matching Convex tables

## Phase 2: Database Migration (Sprite: Bramble)
- Replace all `convex/` with Drizzle ORM + Postgres
- Create schema: users, theses, positions, thesis_positions
- Create API routes (Next.js route handlers) replacing Convex queries/mutations
- Update all frontend components to use fetch/SWR instead of Convex hooks

## Phase 3: Auth Migration (Sprite: Bramble or second sprite)
- Remove @clerk/nextjs
- Add next-auth (Auth.js)
- Replace Clerk middleware with Auth.js middleware
- Replace Clerk sign-in/sign-up pages with Auth.js
- Update auth helpers (getCurrentUser, verifyOwnership)

## Phase 4: Deploy Migration (Sprite: Fern or Kaylee)
- Create Dockerfile for Next.js
- Create fly.toml
- Set up GitHub Actions for deploy to Fly.io
- Configure env vars on Fly.io
- Verify deployment works

## Sprite Assignments
- **Bramble** — Phase 2 + 3 (data layer + auth — systems work)
- **Fern** — Phase 4 (deployment — platform work)
- OR: Single sprite (Bramble) does all engineering, Kaylee handles infra

## Open Questions
- Drizzle vs Prisma? (Drizzle is lighter, more SQL-native, better for agents)
- Auth provider: GitHub only? Or Google too?
- Fly.io region: Which one?
