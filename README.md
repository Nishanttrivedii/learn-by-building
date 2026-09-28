# learn-by-building

A [Claude Code](https://claude.com/claude-code) skill for learning a new language — plus system
design and design patterns — **by shipping something real**, instead of doing a course on the side
and hoping it transfers.

You point Claude at the thing you actually have to build. It produces a curriculum where every lesson
exists because a build step needs it, writes those lessons with examples whose output is really run,
and gives you a board that tracks what you have learned, what you have practised, and how much of the
feature exists.

**It works either way round.** Joining an existing codebase, it reads the code first and builds the
curriculum around the feature you have been handed. Starting from nothing, it works from the plan
instead — I have used it both ways: to learn Go inside a service that already existed, and to design
and build a new module from scratch. An existing repo is not a requirement.

---

## Why it exists

I come from a MERN background, and I recently got the chance to work on a Go service at work. I am
still near the beginning of that.

The problem wasn't the syntax. It was having to learn the language and ship something at the same
time. A course doesn't know anything about the codebase you're in, and reading the existing code
teaches you what it does rather than why it was written that way.

So I tried building the learning around the feature instead, with one rule:

> Every lesson is used by a build step, and every build step is teachable on demand.

Whatever you're reading is something the next step actually needs, and you're never asked to write
code nobody has explained.

**Where this is:** early. It works, I use it, and there is plenty I haven't built yet. Parts of it
will change as I learn more. If you try it and something is wrong or unclear, please say so.

## What you get

| | |
|---|---|
| **A topic map** | every topic marked core (a build step needs it), bonus, or deliberately not planned **with a reason** — so "coverage" is a number you can check, not a claim |
| **Milestones** | a few lessons plus the build steps that use them, each with a gate written so it can be checked |
| **Lessons** | 3–8 concepts each: a runnable example, its **real** output, an explanation of that example, the traps, and a comparison with the language you already know — collapsed, so it never interrupts |
| **Exercises** | predict · choose · write · fix · read-and-explain, checked by real tests |
| **Build briefs** | open a build step cold and take it apart: goal, what makes it hard, what is pinned vs yours, constraints stated as **problems never answers**, and every constraint linking to the concept that explains it |
| **A board** | what you have learned, what the map covers, how much of the feature works, and your own notes — all in one page |
| **A checker** | runs every example, fills in every output from a real run, and refuses a brief whose links go nowhere |

## Install

Claude Code reads skills from `~/.claude/skills/`. Clone this repo into it:

```bash
git clone https://github.com/<your-username>/learn-by-building ~/.claude/skills/learn-by-building
```

On Windows, that path is `C:\Users\<you>\.claude\skills\learn-by-building`.

That is the whole install. Claude picks the skill up automatically the next time it starts, and uses
it when you ask for something it covers.

The checker and the progress tool ship in two versions, so nothing else is required:

```bash
node tools/lessoncheck/lessoncheck.mjs   # Node 18+, no dependencies
go run ./tools/lessoncheck               # the original, if you have Go
```

## Which languages does it work for?

The method has nothing to do with Go, and neither does the content format. Which language you are
learning is configuration, in `track.json`:

```jsonc
"language": { "key": "py", "name": "Python", "file": "main.py",
              "run": ["python", "main.py"] },
"contrast": { "key": "js", "name": "JavaScript", "run": ["node", "main.cjs"] },
"exercise": { "test": ["python", "-m", "pytest", "{dir}"] }
```

Rust would be a `Cargo.toml` in `setup` and `["cargo", "run", "-q"]`. The lessons, the briefs, the
board and the topic map do not change at all.

**One thing to know:** every example in this repo is Go, with JavaScript as the
language-you-already-know. That is what I happened to be learning; it is not a limitation of the
format.

The checker comes in both flavours — `lessoncheck.mjs` (Node 18+, no dependencies) and `main.go` —
so a Python or Rust track needs nothing but Node. The two are kept in step: on the same track they
produce the same counts, the same problem reports and the same `content.json`. The only thing the
Node one cannot do in-process is check gofmt formatting, so if `language.gofmt` is set it shells out
to `gofmt` and says so plainly when it is missing.

I have run this with Go and JavaScript tracks. If you use it for another language and something in
the tooling assumes Go, tell me — that is exactly the feedback worth having.

## Use it

Open Claude Code where the work is going to happen, and say what you are doing.

In an existing codebase:

```
I have to build <the feature> in this repo. I don't know <language>.
Teach me as we build it.
```

Or starting something new:

```
I want to build <the thing> in <language>, and I'm learning the language as I go.
Plan it as a track and teach me through it.
```

Either way Claude looks at what exists — the code, or just the idea — walks you through the problem
the way a senior developer would, and comes back with the questions only you can answer: what goes in
the first cut, who writes which parts, where the learning material should live. Then it writes the
plan, and the first milestone.

Some useful things to say later:

- *"next milestone"* — carry on where you stopped
- *"I want to start at the build step"* — get the brief and pull lessons in as you need them
- *"I already know this one"* — skip a lesson, or test out of it by answering its questions first

## How it helps

- **The learning is the work.** Nothing is a toy exercise; the thing you build is the thing you were
  going to have to build anyway.
- **Being stuck leads somewhere.** Every constraint in a build brief links to the concept that
  explains it, so "I don't know how to do this" becomes one click.
- **You can trust the material.** Every output shown is produced by really running the code — the
  checker fails the build if a single one is wrong.
- **Progress is honest.** A topic counts as learned only when you have both said you understand it
  *and* passed an exercise that practises it, and the map says out loud what it does not cover.
- **You keep it.** Lessons are plain Markdown, progress is yours, and the learning folder stays out
  of the product repository.

## Layout

```
SKILL.md                    the workflow Claude follows, stage by stage
references/                 the formats: curriculum, lessons, build briefs, verification, the board
assets/board/index.html     the board, published as an artifact
assets/examples/            a real lesson, a real build brief and its contract tests, a curriculum
scripts/lessoncheck/        the checker, in Node and in Go: proves every output and every link
scripts/progress/           copies your board progress to disk
scripts/specdump/           turns an OpenAPI spec into local Markdown, so docs stop living in a tab
```

The examples use a fictional shipping domain — two carriers with incompatible vocabularies behind one
interface — because that is the shape of most real integration work.

## Licence

MIT.
