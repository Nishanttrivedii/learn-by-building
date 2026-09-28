# Verification: lessoncheck and track.json

"Verified" means something ran. The checker is what makes every output in every lesson trustworthy. It also stops the plan, the board and the product documents from drifting apart. It's `scripts/lessoncheck/main.go`, a single Go file that uses only the standard library, copied into `<learning folder>/tools/lessoncheck/`. The learning folder needs its own `go.mod` (`module <track.module>`) so the checker and the exercises build there.

## track.json

Everything project-specific lives here. `assets/track.example.json` is a working example from a Go track for a JavaScript developer.

| field | what it's for |
|---|---|
| `title`, `eyebrow`, `lede`, `route` | the board's header |
| `module` | module name used when testing exercises (`{module}` in `exercise.setup`) |
| `plan` | file with the topic tables (default `plan.md`) |
| `bonusCap` | bonus topics allowed per milestone (default 2) |
| `language` | the language being learned: fence `key`, `name`, example `file`, `setup` files for the temp dir, `run` and `check` commands, `gofmt`, `hljs` id, `runLabel` |
| `contrast` | the language the learner already knows, run the same way; optional |
| `exercise.setup`, `exercise.test` | files written at the temp root (e.g. `go.mod`), and the test command with `{dir}` = `practice/<dir>` |
| `product` | optional: `repo` (relative or absolute), `baseCommit`, `protectedPaths`, `requirements` (path in the repo), `capabilitiesHeading`, `pilotColumn` |

**Other languages.** Only `language`, `contrast` and `exercise` change. Examples:
- Python: `"run": ["python", "main.py"]`, `"exercise.test": ["python", "-m", "pytest", "{dir}"]`.
- Rust: a `Cargo.toml` in `setup` and `["cargo", "run", "-q"]`.

The checker itself is Go, so the machine needs Go to run it. If it doesn't have Go, port `main.go` to Node: the structure (parse, collect groups, run, compare, write `content.json`) carries over one-to-one. Say which languages you actually tested; this was only run with Go examples and JavaScript contrasts.

## What it checks

1. **Curriculum:**
   - unique ids;
   - tracks are declared;
   - core topics have a build step in the same or a later milestone;
   - bonus topics have no `usedBy` and stay within the cap;
   - not-planned topics have a reason;
   - once a milestone has lessons, every core topic in it is taught;
   - the plan's topic tables agree row by row;
   - the product document's capability table matches, if configured.
2. **Lesson format:**
   - 3–8 concepts;
   - valid flags and required sections;
   - Explain ≤ 80 words; a one-line Takeaway;
   - every concept practised;
   - at least two predict or choice exercises per lesson;
   - write exercises have three hints; choice questions have exactly one right answer;
   - every shown code block runs unless marked `snippet`.
3. **Execution:**
   - every example runs in a fresh temp dir, four at a time, and its output must match (paths and `exit status` lines are normalised away);
   - Go examples must be gofmt-formatted when `gofmt` is on;
   - predict and choice answers come from running the prompt's code.
4. **Exercises:**
   - each starter must **fail** the test command and each reference must **pass**;
   - the learner's `practice/` workspace is created once, never overwritten, and its test files must match the bundle;
   - it reports how many workspaces currently pass.
5. **Protected product paths:** `git diff --name-only <baseCommit>` and `git status` in the product repo must show nothing under `protectedPaths`. For example, the code the plan says not to touch yet.

It prints one summary line, or a sorted list of problems in compiler style (`file:line: …`), and exits 1. It writes `board/content.json` only when there are no problems.

## Using it

- **After writing a lesson:** `go run ./tools/lessoncheck -update`. This fills every output block from a real run. Then re-read each explanation next to a changed block, then run it again without `-update`.
- **Prove it fails.** The first time you set it up for a track, break one output on purpose (e.g. `4767` → `4768`), confirm the checker names the file and concept, then restore the file. A checker that has only ever passed proves nothing.
- **Quote the summary line** in the plan's phase status and in the log, e.g. `lessoncheck ok: lessons=2 concepts=7 exercises=10 examples-verified=20 contrast-examples-verified=6 starters-fail=5 references-pass=5 practice-passing=0/5 topics=105 protected-untouched=yes`.
- **Before marking a build step done** that depends on the learner's own code, run the tests yourself. The board's "my tests pass" is self-reported, and `practice-passing` is the independent number.
