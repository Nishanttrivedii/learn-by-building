---
name: learn-by-building
description: Teach someone a new programming language, system design and design patterns while building a real feature in their own codebase. It covers a senior-developer walkthrough of the problem, a topic map tied to milestones and build steps, example-first lessons whose outputs are really run, exercises checked by tests, and one board that tracks lessons, topics covered, feature progress and notes, with the learner's progress copied to disk so nothing is lost. Use this whenever the user wants to learn while building something real ("I don't know Go, teach me as we build this", "help me understand how an experienced developer would approach this, step by step, I have no context", "I want to learn system design and design patterns with this task", "turn this into a learning track", "lessons + board for this feature"), and whenever resuming an existing learning track (next milestone, board, notes), even if they never say "lesson". Not for a one-off explanation or a tutorial with nothing being built.
---

# Learn by building

A way to learn a language, system design and design patterns **through one real feature**, so the feature ships and the learning sticks. The learner called it "arguably the best way to learn things". It works because of five rules:
- **Every lesson is used by a build step, and every build step is teachable on demand.** Learning never drifts into toy material, and the learner never has to write code they weren't shown how to write — whether they read the lesson first or reach for it at the moment the build step stops being obvious (`references/build-briefs.md`).
- **Examples are the teaching.** Each idea is a small runnable example from the product's own domain, with its *real* output, explained, then compared with the language the learner already knows, precisely where their instinct would be wrong.
- **Everything is measured.** The topic map says what's covered and what isn't (with reasons). A checker proves every output and every exercise. The board shows learned, understood, practised and built at a glance.
- **Nothing is lost.** Lessons are files. Clicks, answers and notes live on the board and are copied to disk every session.
- **Learning and product stay apart.** The product repo holds only product material; the learning folder stays private.

The language being learned is configuration (`track.json` → `language`, `contrast`, `exercise.test`), not something baked into the format: the examples here are Go with JavaScript as the known language, but nothing about the lessons, briefs or board is Go-specific. The checker ships as both a Node script and a Go program, so only the language being learned needs its own toolchain.

This skill specialises `design-and-build-workflow`. That skill's stages (understand → requirements → plan → build → prove), its templates and its teaching formats all apply. Load it too if it's available.

## The stages, in order

The order matters: each stage ends with something the learner can react to cheaply.

### 1. Measure, then walk through it as a senior developer
Read the real code and the real external spec before explaining anything.
- If the spec sits behind a login or a browser tab, make a local reference first. `scripts/specdump` turns an OpenAPI YAML into operations, schemas and examples Markdown, in three generated files.
- Then write the numbered walkthrough in `references/thinking-walkthrough.md`: what exists, precedent in their own codebase, the new API read as a flow, the differences that force decisions, the contract, shared vs per-implementation, failure modes, migration.
- Quote lines and counts, not impressions.
- End with three or four numbered questions, each with a recommendation.

### 2. Say it back, then pivot to the track
When the learner explains their plan and says they want to learn from it:
1. Restate it in your own words.
2. Name the **one risk** their plan creates. Usually it's an abstraction designed from one implementation, fixed by checking every shared type against the second on paper.
3. Propose milestones, where each teaches exactly what its build step needs.
4. Ask only what's genuinely theirs to decide:
   - **Who writes what.** Mixed is best: Claude writes the plumbing, the learner writes the core, always taught first. The learner's words: "if im writing something, i should actually know what to write and how".
   - **Where the learning lives**, and whether they want a backup or git for it. Recommend a folder outside the product repo from day one (`references/where-things-live.md`).
   - **Naming** of anything user-visible.

### 3. Requirements: two documents, one numbering
- **Product requirements** go in the product repo: feature rules, build order, capabilities. Nothing about learning.
- **Learning requirements** go in the learning folder: how they learn, where things live, the accepted risks.
- Quote every rule the learner states **verbatim** and number it (R1, R2…). Your own inferences go in a separate "derived" table (P1, P2…) until confirmed.
- Keep a decisions table (decided / still open) in each document; the board shows both.

### 4. The teaching plan
Build the topic map (`references/curriculum.md`):
- tracks × milestones;
- every topic core (tied to the build step that uses it), bonus (at most two per milestone) or not planned (with a reason);
- capabilities for the feature;
- a balance table.

Write it as tables in `plan.md`, generate `board/curriculum.json` from them, and **count everything with a script** before stating a number. Say the honest size up front, e.g. "84 core lessons, written one milestone at a time".

### 5. Set up the learning folder, the checker and the board
1. Create the learning folder. Copy `scripts/lessoncheck` and `scripts/progress` into `tools/`, and `assets/board/index.html` into `board/`, replacing `__TRACK_TITLE__` with the track's title. Run the checker with `node tools/lessoncheck/lessoncheck.mjs`, or `go run ./tools/lessoncheck` if the machine has Go — a Go track also wants a `go.mod` here so the exercises build.
2. Write `track.json` (start from `assets/track.example.json`; `references/verification.md` explains every field).
3. Run `go run ./tools/lessoncheck`, then **break one output on purpose** to see it fail, and restore it.
4. Publish the board (`references/board-and-progress.md`: copy both board files to the scratchpad, publish with the `db` and `user` capabilities, keep the URL).

### 6. Each milestone: the lessons and the build, in either order
1. **Write the lessons** in the format in `references/lesson-format.md`, modelled on `assets/examples/lesson.example.md`:
   - 3–8 concepts per lesson;
   - each concept: an example with an empty output block, Explain (≤ 80 words), the comparison with the known language, More (the traps), a one-line Takeaway;
   - then exercises (at least two predict or choice; coding exercises as `.txtar` bundles whose starter fails and solution passes).
2. **Prove them.** `lessoncheck -update` fills the outputs from real runs. Re-read every explanation next to a changed output, run `lessoncheck` until it prints `ok`, then republish.
3. **Write the build brief** (`references/build-briefs.md`), in `_builds/<build id>/`. It is what lets the learner open the build step cold and pull the lessons as they hit them, rather than reading them all first:
   - the goal, and what actually makes it hard;
   - what is **pinned** because something outside them depends on it, and what is genuinely theirs to decide;
   - the constraints stated as **problems, never answers** ("absent and blank must stay distinguishable", not "use a pointer");
   - a decomposition where every sub-problem carries `needs: <topic-id>`, so being stuck is one click rather than a search;
   - **contract tests, which are the only definition of done** — for a *design* step they must assert properties through a serialisation boundary, never field names, or the test makes the decision the step existed to make;
   - a **Compare** section, opened only once the tests pass, naming two or three decisions in the reference worth arguing with.

   Verify every suite in both directions: the reference passes it, and a deliberately naive implementation fails it. Say how many distinct mistakes it caught.
4. **Then the build step**, in the product repo:
   - The learner writes the core pieces, guided like an exercise (what goes where, hints on request).
   - Claude writes the plumbing.
   - Check shared types against the second implementation (the P1 rule).
   - Run their tests yourself before marking the step or capability done in `curriculum.json`.

If the learner has said they don't want a pilot review loop, build each milestone complete. The checker replaces the review.

### 7. End of every session
1. Copy progress from the board into `progress/` (`references/board-and-progress.md`) and read the learner's notes. Answer any note that shows a misunderstanding.
2. Add a dated line to `log.md`, update the plan's phase status with the checker's summary line, re-run the checker and republish.
3. Update memory with where the track lives and what's next.

### 8. Resuming a track
Read the learning `requirements.md`, the plan's phase status lines, `log.md` and `progress/notes.md`, then re-run the checker. The board's "Up next" says where the learner is. `practice-passing` in the checker's summary shows which coding exercises they've already solved.

## Rules that came from real corrections

Each of these cost a round trip once. The reason is attached so you can apply the spirit, not just the letter.

- **Plain words before planning prose.** After a dense plan, the learner said: "i dont get the last three questions… where are p1 p2… your plan is not clear to me."
  - Lead with a table of "you learn / we build" and one real, run example.
  - Say where things are by file and section.
  - Drop questions you can decide yourself.
- **Terse replies mean the plan was clear.** Don't ask them to re-explain. Ask only what's theirs to decide, numbered, with a recommendation each.
- **No pilot loop once the format is known:** "i dont want to review and loop in". Build complete and let verification carry the trust.
- **Few files.** 128 generated files in source control drew a complaint. Bundle generated examples into one file, and use one `.txtar` per exercise.
- **No browser dependency for reference material:** "i want to stop sharing swagger doc/browser". Make it local and greppable, and say when it was downloaded.
- **Learning stays off the product remote:** "my learning is different and the product itself is different". Set up the separate folder at the start.
- **Nothing is committed or pushed without explicit approval, each time.**
- **Count with a script.** A hand-counted plan was wrong five times.
- **Say what wasn't verified.** For example, "the board was syntax-checked, not clicked through". Never present unrun output as real.
- **Write long Markdown with the file tool,** plus a small Node splice script with anchors that fail loudly. A bash heredoc broke on quoting.
- **The comparison helps but shouldn't interrupt:** "comparison with javascript is distracting although its useful". The board collapses it below the Takeaway. So each concept must teach fully in the new language on its own, with Go-only facts in More, never only in the comparison.
- **Some learners want to start at the build and work backwards:** "i want to skip lessons and directly go to build but the instructions should be as such, i should be able to break it down and then learn (like reverse engineering)". Give the build step a brief, keep the guarantee that the explanation is one click away, and amend the taught-first requirement with a **new number** rather than rewriting it — the learner stated it, so only they can change it.
- **A contract test for a design step must assert properties, not names.** A test that says `p.MiddleName` has already made the decision the step existed to make. Go through a serialisation boundary instead, and pin only what something outside the learner depends on.
- **The learner's notes are signal.** Read them each session and reply to them.

## Files in this skill

| Path | Use it for |
|---|---|
| `references/thinking-walkthrough.md` | Stage 1: the senior-developer walkthrough and the pivot into a track |
| `references/curriculum.md` | Stage 4: tracks, milestones, topics, bonus cap, capabilities, the plan's structure |
| `references/lesson-format.md` | Stage 6: the lesson file format, exercise types, txtar bundles, practice workspaces |
| `references/build-briefs.md` | Stage 6: build briefs — starting at the build step, constraints as problems, contract tests for design steps |
| `references/board-and-progress.md` | Stages 5 and 7: board tabs, data, publishing, progress copy and restore |
| `references/verification.md` | `track.json` fields, what the checker enforces, other languages |
| `references/where-things-live.md` | Learning vs product layout, split requirements, the up-front questions |
| `scripts/lessoncheck/main.go` | The checker: copy to `tools/lessoncheck/` |
| `scripts/progress/main.go` | The progress copier: copy to `tools/progress/` |
| `scripts/specdump/` | OpenAPI YAML → local Markdown reference (own module; `go run . <spec.yaml> <outdir>`) |
| `assets/board/index.html` | The board page template (`__TRACK_TITLE__` to replace) |
| `assets/track.example.json` | A complete `track.json` (Go learned, JavaScript known, product repo with protected paths) |
| `assets/examples/lesson.example.md` | A real lesson that passed the checker |
| `assets/examples/exercise.example.txtar` | A real write exercise: starter, solution, tests |
| `assets/examples/build-brief.example.md` | A real build brief that passed the checker, with its contract tests beside it |
| `assets/examples/curriculum.example.json` | The curriculum shape, trimmed |

For scale, the track this came from: the first milestone was 2 lessons, 7 concepts and 10 exercises (26 verified outputs). The full map was 105 topics across three tracks, 84 of them core, feeding 11 capabilities over 11 milestones.
