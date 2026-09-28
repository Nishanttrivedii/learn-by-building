# The senior-developer walkthrough

This is what opens a learn-by-building track. The learner asked to understand *how an experienced developer approaches the problem*, not to receive an architecture. It worked because every step was grounded in something real: a line of their own code, a field in the external API, a precedent already in the repo. Nothing was abstract.

Write it as numbered steps. Each step has a **thinking habit** (one line, in a quote block) that names the habit being shown, so the learner can reuse it on the next problem.

## Before writing: measure

Read the actual code and the actual external spec first. Collect evidence you can quote:
- file paths with line numbers;
- counts ("32 files", "17 endpoints", "182 schemas");
- a type signature that reveals a design decision, such as `CreateLabel []map[string]interface{}`, which shows the browser echoes raw supplier data back.

If the external docs sit behind a login or a browser tab, make them local first (`scripts/specdump` for OpenAPI). Every later step depends on being able to grep them.

## The steps

**0 — Say what "done" means in one sentence.** Written from the user's side, with one clause that will rule options in or out later (e.g. "…without knowing or caring which supplier served them").

**1 — Understand what you already have before touching the new thing.** The beginner's mistake is opening the new API docs first. Walk the existing flow end to end (routes, request shapes, storage). Name what's coupled to the current implementation, with the exact line that proves it. Look for someone having already anticipated the change (a `provider` column defaulting to one value).
> Thinking habit: point at the line, don't describe the feeling.

**2 — Look for a precedent in your own codebase.** Has the team already solved a problem of this shape? Name its pieces (interface, orchestrator, opaque IDs, pending-first writes) and show its contract. Conclude: the big decisions are already made, so the job is to apply them and find where this problem differs.
> Thinking habit: consistency beats cleverness.

**3 — Read the new API as a flow, not a list of endpoints.** Trace one complete transaction through it, then put it side by side with the current implementation in a table. The rows that don't line up are the design work.

**4 — The design work is in the differences.** One sub-heading per difference: data shape, ID chain and lifetimes, extra steps, sync vs async, auth, cancellation, enums and formats, transport. Each one ends with **→ Decision:** what that difference forces.

**5 — Decide what the client sees.** Derive the contract from step 0's sentence. Give independent reasons that point the same way. Name the cost honestly (e.g. "the frontend changes substantially") and say that the cost is the user's call.

**6 — Split shared from per-implementation, then look downstream.** Draw the line between the orchestrator and the adapters. Then list everyone who *reads* what you're changing: vouchers, invoices, emails, listings, other services. Say what you haven't counted yet; that count is the first measurement for the requirements.

**7 — Problems that only exist with more than one implementation.** Fan-out, timeouts per supplier, partial results, duplicates, on/off switches, what to cache.

**8 — List failure modes before the happy path.** For each step: "what if it fails halfway?" Unknown outcomes, expiry mid-flow, retries that double-charge.

**9 — Migrate without breaking anything.** A new versioned surface next to the old one. A pilot slice that exercises every new idea. Wrap old code instead of copying it. Each phase shippable on its own.

End with **three or four numbered questions**, each with a recommendation, then stop. The learner replies by number.

## When the learner then says "I want to learn from this"

This is the pivot into the track. Restate their plan in your own words, including anything they changed (e.g. "keep Cargolink separate, integrate Swiftpost first"). Then add the **one risk their plan creates**. Designing a shared abstraction from a single implementation makes it quietly take that implementation's shape. Give the no-cost fix: check each shared type against a second implementation on paper. This became the confirmed rule "P1" in the project this skill came from.

Then propose milestones, where each one **teaches exactly what its build step needs**, as a table:

| # | What we build | System design | Pattern | Language |

Name the pilot milestone and ask the few genuinely open questions: who writes what, where lessons live, and the prefix or naming for anything user-visible.
