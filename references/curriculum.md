# The teaching plan: topic map, milestones, balance

The learner asked for two things that pull against each other: *broad coverage* ("I expect that we go well with coverage so that not a lot of topics remain") and *a real feature shipped* ("the final feature we are building is also important"). The topic map reconciles them. Every topic is tied to the build step that uses it, so learning never drifts away from the product, and coverage becomes something you can count.

## Milestones

A milestone = a few lessons + one or more **build steps** that use them. Each has an id (`m00`…), a short code (`M0`), a title, a goal (one line), and a **gate**: "done when …", written so it can be checked.

- **M0 is setup and basics**, with a *reading* build step (read a small, real file from the product repo) rather than writing product code.
- The first milestones lean towards learning; from about the middle on, every milestone ships part of the feature. Say this openly in the plan; it's a deliberate choice, not an imbalance.
- The last milestone is often "the second implementation joins". It's the test that the abstraction was right (open/closed principle).

## Tracks

Usually three: the **language** being learned, **system design**, and **design patterns**. They're declared in `curriculum.json` → `tracks` (key + label). The key of the language track must equal `track.json` → `language.key`, because that's how the checker knows which concepts need the comparison with the language the learner already knows.

## Topics

Each topic gets:

| field | meaning |
|---|---|
| `id` | `<prefix>.<slug>`, e.g. `go.slices`, `sd.idempotency`, `pat.adapter` |
| `track` | one of the track keys |
| `title` | what it covers, plainly |
| `milestone` | where it's taught (null only for not-planned) |
| `kind` | **core** (a build step needs it) · **bonus** (worth knowing, the build doesn't need it) · **not-planned** (deliberately left out) |
| `usedBy` | core only: the build step that uses it, in the same or a later milestone |
| `reason` | not-planned only, e.g. "relies on inheritance, which Go doesn't have" |
| `depth` | core · interview · advanced |
| `relation` | transfers · differs · new, compared with the language the learner already knows |

How to build the list:
1. Walk the milestones' build steps and ask "what must the learner know to write this?". Those are the core topics.
2. Then write out what a competent practitioner of the language knows, and mark whatever the build doesn't need as bonus or not planned. **Not planned always gets a reason.** An honest "we won't cover cgo, there's no C here" is what makes the coverage count trustworthy.
3. Place each topic at the **earliest milestone whose build step needs it**. Lesson order inside a milestone follows dependencies: strings before the functions example that uses strings.

**The bonus cap** is at most two per milestone, and bonus topics never block a build. When one milestone collects too many, move the extras to the nearest *quieter* milestone that's close in subject (reflection → the JSON milestone; fuzzing → the testing milestone). Don't raise the cap. Record the moves in the plan.

## Capabilities (the feature)

Split the final feature into about 8–12 capabilities (`F1`…), each with a milestone and a first-slice flag. The board's Feature tab shows them next to the build steps, which keeps the product visible while the lessons pile up. If there's a product requirements document, the same table lives there too, and the checker compares the two.

## The plan document

`plan.md` in the learning folder holds, in this order:
1. The short version, including the honest sizes (for example "84 core lessons").
2. What exists.
3. Principles.
4. Constraints.
5. Where things live.
6. Lesson anatomy.
7. Exercise types.
8. Topic states.
9. **The full outline:** one table per track, with columns `| id | Topic | Milestone | Used by | Kind |`, the capability table, and a balance table (core lessons, build steps and feature moved, per milestone).
10. The data model.
11. Verification.
12. Phases, with **status lines** that quote the checker's output.
13. Risks.
14. Decisions.

The checker compares the plan's topic tables with `curriculum.json`, row by row. That way the plan the learner reads can't drift from the map the board shows.

**Count with a script, never by hand.** The first version of this plan got five counts wrong: the lesson total, four per-milestone counts and two bonus counts. A ten-line Node or Go script over the tables catches that in seconds.

## Generating curriculum.json

Write the tables in `plan.md` first (humans review tables), then generate `board/curriculum.json` from them with a small script that maps "Used by" text to build ids. See `assets/examples/curriculum.example.json` for the shape.
