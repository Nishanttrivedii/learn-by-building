# Lesson format

This builds on the concept-lesson format in the `design-and-build-workflow` skill (`references/teaching-formats.md`, Format 1). Load that too if it's available. What's here is the concrete file format that the checker parses and the board renders. `assets/examples/lesson.example.md` is a real lesson that passed the checker.

## Why it's shaped like this

These rules came from the learner, often as corrections:
- **Examples are the teaching.** "example is not one way to learn things, its the only way."
- **Teach before you test.** A quiz before the idea was rejected.
- **One idea per concept**, so progress is trackable: "for one thing we know we have learned this."
- **The learner must produce something** before seeing the reference.
- **The comparison with the language the learner knows goes where their instinct would be wrong.** A JavaScript developer expects `len("Zoë")` to be 3; Go says 4.

## The file

`lessons/<milestone>/NN-name.md`:

````markdown
---
id: m00-l1
title: Running Go
milestone: m00
---

Intro: two or three sentences on what this lesson gives the learner and why now.

## concept go.toolchain: Go checks your whole program before it runs any of it
flags: core, new

### Example
```go
package main
...
```
```output
```

### Explain
What *that example* did, naming its lines and names. At most 80 words.

### In JavaScript
```js
...
```
```output
```
One or two sentences: where the instinct from JavaScript would mislead.

### More
A contrasting case, such as the error version or the trap, with its own code and output.

### Takeaway
One line.

## exercise m00-l1-p1 predict: What do these verbs print?
practises: go.fmt

### Prompt
Type the exact line this program prints.
```go
...
```

### Answer
```output
```

### Why
Short explanation shown after the learner commits.
````

**Rules for the format:**
- Each heading `## concept <topic-id>: <statement>` is one concept. Its id is a topic in `curriculum.json` for that milestone. The title is a statement the learner could say back, not a topic label.
- The `flags:` line holds depth (`core|interview|advanced`) and relation (`transfers|differs|new`).
- The comparison section is called `In <Name>`, where `<Name>` comes from `track.json` → `contrast.name`. It's required for every concept in the language track.
- **The board shows the comparison collapsed**, below the Takeaway, as "Compare with <Name>". The learner found it useful but distracting: "i want it to be collapsed which if i want i can open it so that the flow goes with go lang". So Example → Explain → More → Takeaway must teach the whole idea in the language being learned. Never put code, or a fact about the new language, only in the comparison; put it in More. The checker rejects code in the language being learned inside the comparison section.
- Code fences:
  - The tag is the language key, e.g. `go` or `js`.
  - `file=<path>` adds a second file to the same example, for multi-file examples such as packages.
  - `run=check` runs the language's check command (e.g. `go vet`) instead of running the code.
  - `snippet` shows code without running it. Use it only for something that genuinely can't run, like an ES-module `export` in a single-file runner.
- Every non-snippet code block is followed by an **empty** ```` ```output ```` block. `lessoncheck -update` fills it from a real run. **Never type an output by hand.** After `-update`, re-read every Explain next to a changed block; when the real output surprises you, the explanation is what's wrong.
- A lesson has 3–8 concepts. Order them by dependency.

## Exercises

Headings take the form `## exercise <id> <type>: <title>`, followed by a `practises: <topic ids>` line (and `dir: <milestone>/<folder>` for coding exercises).

| type | sections | how it's checked | notes |
|---|---|---|---|
| `predict` | Prompt (code), Answer (empty output block), Why | the board compares the typed answer with the real output, line by line with whitespace trimmed | test-out uses these |
| `choice` | Prompt (code), Options (`- [x]` / `- [ ]`, exactly one right), Answer (output), Why | the board checks the option, then shows the real output | good for "does it even compile?" |
| `write` | Task, Hints (exactly 3: a nudge, the approach, nearly the answer), Why | the learner runs the tests on their machine; the reference appears only after "my tests pass" | starter must fail its tests |
| `fix` | Task, Hints (1+), Why | same | a realistic bug from the lesson's trap |
| `explain` | Task, Checklist (2+ points), Why | the learner writes an answer, commits it, then ticks the checklist; ⅔ ticked counts as done | used for reading real product files |

**Rules:**
- Every concept is practised by at least one exercise.
- Every lesson has at least two predict or choice exercises. Those are what "test out" uses.
- Use the product's own domain in examples and exercises (rates, depots, bookings); it makes the lesson feel like the job.
- A **reading build step** (M0: read a real, small product file that uses only what's been taught; tell the learner to skip the parts that come later) is an `explain` exercise that the curriculum's build step points to.

## Coding exercise files

One bundle per exercise, `lessons/<milestone>/exercises/<exercise-id>.txtar`, in Go's txtar format. It's one file instead of three, which keeps the file count down (the learner objected to 128 files in git):

```
A comment line describing the exercise.
-- starter/splitcost.go --
...a stub that compiles but fails the tests...
-- solution/splitcost.go --
...the reference...
-- splitcost_test.go --
...tests shared by both...
```

- **Starters should compile and fail on assertions.** A starter that doesn't compile fails for the wrong reason, unless the lesson is about the compile error itself, like an unexported name used from a `_test` package.
- The learner's workspace is `practice/<dir>/`. The checker creates it from the starter the first time and **never overwrites it**. It checks that the test files there still match the bundle.
- The command shown on the board comes from `track.json` → `exercise.test`, with `{dir}` filled in, e.g. `go test ./practice/m00/splitcost/`.
