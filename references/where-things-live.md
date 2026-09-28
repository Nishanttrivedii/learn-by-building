# Where things live: learning vs product

The learner's rule, arrived at partway through the first build:

> "i dont want any of my learning and everything to be on the remote, because my learning is different and the product itself is different."

Put the split in place **from the start** in future tracks. It's much cheaper than moving 30 files and splitting documents afterwards.

## The layout

```
<learning folder>\            e.g. ~/go-learning: its own Go module, never inside the product repo
  track.json                  what the checker and the board need to know
  go.mod                      module <track.module>
  requirements.md             learning requirements + decisions
  plan.md                     the teaching plan, with phase status lines
  log.md                      dated log, newest first
  board/                      index.html, curriculum.json, content.json (generated)
  lessons/<m>/                NN-name.md, exercises/<id>.txtar
  practice/<m>/<exercise>/    the learner's workspaces
  progress/                   notes.md, progress.json, snapshots/
  tools/lessoncheck/, tools/progress/

<product repo>\docs\<feature>\
  requirements.md             product requirements, build order, capabilities; says nothing about learning
  <api>-reference/            local API reference (specdump output), if the product integrates one
```

**The dividing line:**
- Practice exercises are learning, so they go in the learning folder.
- Build steps are product code the learner writes, so they go in the product repo, where they're reviewed and committed like any other change.
- The external API reference is product knowledge, so it goes in the product repo.

## Requirements across two documents

Use **one shared numbering** across the two documents (R1–R3 product, R4–R8 learning, and so on). Then log entries and conversations can say "R10" without saying which document. The product document carries one neutral line, "ids are shared with a companion document kept outside this repository, so some numbers don't appear here". It must not describe the learning track.

The checker reads both documents: open and decided items appear on the board labelled product or learning.

## Questions to ask the learner up front

Ask these once, in plain words, with a recommendation each:

1. **Where should your learning live?** Recommend a folder beside the product repo, e.g. `D:\<topic>-learning`.
2. **Backup:** local only, or a personal copy somewhere they own (external drive, personal cloud, a private repo under their own account, never the company's)? Local only is a legitimate answer. Write the accepted risk into the learning requirements in one sentence: a failed disk loses the lessons, while notes and progress still exist on the board.
3. **Git for the learning folder?** It gives undo and history. The learner in this track said no, and that's fine. The checker never overwrites their work, and progress has dated snapshots.

**Never** commit or push anything in the product repo without explicit approval, each time. Never suggest putting learning material on any remote the learner didn't choose.

## Practicalities

- The Artifact tool only publishes files under the working folder or scratchpad, so copy the two board files to the scratchpad before each publish.
- Commands shown to the learner start with `cd <learning folder>`; the checker writes the folder into `content.json` as `workdir`.
- Tools and file edits outside the working folder may prompt for permission. That's expected.
