# Build briefs — starting at the build step

The default path through a milestone is lessons, then the build. Some learners
want the other direction:

> "we should be directly able to jump to build step, so that i can skip lessons
> and directly go to build but the instructions should be as such, i should be
> able to break it down and then learn (like reverse engineering)"

A **build brief** is what makes that possible. It turns a build step from a
one-line label into something you can open cold and start taking apart, with the
lessons pulled in when you get stuck rather than pushed at you first.

This does not weaken the rule it amends. "Nothing is asked before it has been
taught" existed because the learner said *"if im writing something, i should
actually know what to write and how"*. The guarantee is that the explanation is
**there when it is needed**. A brief keeps that and changes only *when* it
arrives: pushed up front before, pulled on demand now.

When you add this to a track, amend the original requirement with a new number
rather than rewriting it, and record the old rule as superseded. The learner
stated it; only they can change it.

## Which order did they mean?

"Break it down and learn" has two readings and they produce different artefacts.
Ask, with these options:

1. **Spec plus failing tests, built cold.** Never see working code until yours
   passes.
2. **Start from working code and take it apart.** Read first, rebuild after.
3. **Both, in that order.** Attempt cold, then open the reference and compare
   decision by decision.

Three is usually right and is what the format below assumes: it is the only one
where the learner finds out *which of their instincts were wrong*.

## Where a brief lives

```
<learning folder>/_builds/<build id>/
    README.md                 the brief
    <something>_test.go       the contract tests
```

The directory name matches the `id` of a build step in `curriculum.json`.

The leading underscore matters: the test files inside belong to the *product's*
module, not the learning one, so the Go tool must not try to compile them. Go
skips any directory whose name starts with `_` or `.`. Use the same trick for a
worked reference (`_reference/`).

## What a brief contains

Seven sections. The checker enforces two of them by name.

### 1. Goal

What the step must produce, concretely enough to start from nothing — and then,
immediately, what actually makes it hard. That second part is the one people
leave out, and it is what distinguishes a brief from a ticket:

> The hard part is not Go. It is deciding **whose words win**. Swiftpost says
> `consignmentId`, `rateId`, `sessionId`… Cargolink says `CreateLabel`, `ServiceType`,
> `RequestId`… Neither vocabulary may reach anything above the adapter.

### 2. What is pinned, and what is yours

A table. This is what keeps build-first honest: without it the learner cannot
tell a real constraint from your taste, so they either guess or ask.

Pin only what something **outside them** depends on — a published API shape, a
type name a later milestone's interface references, a method that encodes a
business rule. Everything else is theirs: field names, pointer or value or
wrapper, how an enum is represented, how the files are split.

Say why each pin exists. "The frontend will depend on it" is a reason; "for
consistency" is not.

### 3. Constraints, stated as problems

The core of the format. **A constraint names the problem, never the answer.**

| Write this | Not this |
|---|---|
| "A parcel who gave no middle name and one who gave a blank one are different facts. An carrier rejects a blank name differently from an absent one." | "Use a `*string` for the middle name." |
| "The total is whatever the supplier said, never recomputed — their total legitimately differs from base + tax once a convenience fee applies." | "Store `Total` rather than computing it." |

Give each one the real-world consequence. A constraint the learner understands
the cost of is one they will remember; a rule they were handed is one they will
follow and forget.

### 4. `needs:` tags

Every constraint and every sub-problem ends with the topics that explain it:

```
→ `needs: go.zero-values`, `pat.anti-corruption`
```

These are the whole mechanism. They turn "I'm stuck" into one click instead of a
search, which is the difference between reverse engineering and being abandoned.
The checker rejects a tag naming a topic that does not exist, and rejects a brief
with no tags at all.

### 5. Break it down

A table of sub-problems in a suggested order, each small enough to finish in one
sitting, each carrying its constraint numbers and its `needs:` tags. End it with
the instruction that makes the whole thing work:

> Open a lesson when a row stops being obvious, not before.

### 6. Done when

The exact commands. The contract tests are the **only** definition of done —
there is no prose description of "finished" anywhere in a brief.

Also include the project's own gates (build, vet, formatter) so the learner does
not discover them separately.

### 7. Not tested — the judgement calls

List what the tests **cannot** check, so a green run is not mistaken for a
finished design. Every real design step has some: a constraint that needs a
method nothing calls yet, whether a field belongs at all, scope decisions that
are defensible either way.

This section is short and it is the most honest thing in the brief.

### 8. Compare — open only after the tests pass

Where the worked reference lives, and a plain warning that reading it early costs
the practice and teaches nothing.

Then **two or three decisions in it worth arguing with**, named specifically:

> It uses `*string` for the optional middle name. A `HasMiddleName bool`, or a
> small `Optional[T]` generic, would also pass. The pointer is conventional Go
> and it is also the one that panics if you forget to check it.

A reference presented as correct teaches deference. A reference presented as one
defensible set of trade-offs teaches judgement.

## Contract tests: the part that is easy to get wrong

### Implementation steps vs design steps

For an **implementation** step — a client, a registry, a cache — ordinary tests
work. "The registry skips a failing supplier", "fifty concurrent callers cause
one login": these demand behaviour without dictating how it is written.

A **design** step is different, and this is the trap:

> A test that asserts `p.MiddleName` has already made the decision the step
> exists to make.

For a design step the tests must assert **properties**, not names. The reliable
technique is to go through a serialisation boundary — JSON round-trips, a
formatter, whatever the product already treats as a contract:

```go
// Absent and blank must not collapse into the same value. Passes with a
// pointer, a wrapper, a flag — anything that actually works.
var absent, blank Recipient
json.Unmarshal([]byte(`{"firstName":"Asha"}`), &absent)
json.Unmarshal([]byte(`{"firstName":"Asha","middleName":""}`), &blank)

a, _ := json.Marshal(absent)
b, _ := json.Marshal(blank)
if string(a) == string(b) {
    t.Errorf("a recipient with NO middle name and one with a BLANK one "+
        "produced identical JSON (%s) — the two must stay distinguishable", a)
}
```

That works because the wire shape is a genuine external contract while the Go
internals are not. Pin the JSON names in the brief; leave everything behind them
free.

You can also test for **absence** — that no supplier's vocabulary leaked into a
shape — by marshalling and searching the output for words that must never appear.

### Verify a suite in both directions

A suite that only passes is worthless as a specification. Before shipping one:

1. Run it against the worked reference. It must pass.
2. Write a **deliberately naive** implementation — the mistakes a reasonable
   person makes on a first attempt — and run it. Count how many distinct tests
   fail, and say the number in the brief.

For the M1 models step that naive version used plain strings, `omitempty`
everywhere, documents counted as chargeable parcels, a leaked supplier id and a
leaked trace id. Six distinct tests caught it. That number is what makes
"passing them is real" a claim rather than a hope.

### Failure messages are teaching

A contract test's message is read at the moment the learner is stuck, which is
the moment they are most receptive. Say what is wrong *and why it matters*:

```
Chargeable() = 4, want 3 (parcels + oversized, documents excluded)

the total was recomputed as base+tax (3932) instead of kept as sent (4732)

a ShippingOption marshalled a supplier's vocabulary ("rateId")
```

## What the checker enforces

`loadBriefs` in `scripts/lessoncheck` reads `_builds/*/` and refuses:

| Problem | Why it is fatal |
|---|---|
| directory name is not a build id in the curriculum | a brief for nothing |
| no `README.md` | — |
| a `needs:` tag naming a topic that does not exist | being stuck must lead somewhere |
| no `needs:` tags at all | same |
| no `*_test.go` | no tests, no definition of done |
| no `## Done when` section | same |
| no `## Compare` section | the reference must be findable afterwards |

Briefs go into `content.json` under `briefs`, keyed by build id, each carrying
its markdown, its resolved `needs` list and its test files.

Prove the validation the way you prove everything else: break a brief three ways
at once — a bogus topic id, a renamed `## Compare`, the test file moved aside —
confirm the checker names all three, then restore and confirm the content hash
returns to what it was.

## On the board

`assets/board/index.html` renders briefs when `content.json` has them:

- a build step with a brief is **clickable from the Journey**, labelled
  "brief — you can start here";
- **"Up next" opens the brief** rather than stepping past it to the next unread
  concept — this is what makes build-first a real path rather than a thing the
  learner has to know about;
- the brief view opens with the concepts it draws on, each showing the learner's
  current state on it;
- `linkTopics()` turns **every topic id printed as code in the brief** into a
  link to its concept card. This is the feature. Without it the `needs:` tags are
  decoration;
- the contract tests render at the bottom, labelled as the specification.

Nothing in the board's data model needed changing: a topic reached through a
build step before its lesson is read is already `practised` rather than
`understood`, and both already exist as distinct states.

## What this does not change

- A brief is an **additional** entry point, never a replacement. The lessons
  still get written, still get verified, and a learner who prefers reading first
  loses nothing.
- Practice exercises are unaffected. They remain the small, fast loop; a brief is
  the large one.
- **Claude still does not write the learner's build step.** If the learner asks
  for that in the moment, treat it as a one-off for that conversation, say so
  plainly, and keep it out of the track's rules.
