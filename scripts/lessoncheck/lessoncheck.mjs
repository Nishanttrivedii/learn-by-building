#!/usr/bin/env node
// lessoncheck — verifies a learn-by-building track and builds the file its
// board displays. A port of main.go, so the tooling does not need Go unless the
// language you are learning does.
//
// Copy this folder to <learning folder>/tools/lessoncheck and run it from the
// learning folder:
//
//   node tools/lessoncheck/lessoncheck.mjs            verify, then write board/content.json
//   node tools/lessoncheck/lessoncheck.mjs -update    rewrite every output block from a real run
//
// Needs Node 18 or newer and nothing else. Everything project-specific comes
// from track.json: the language being learned and the one the learner already
// knows (how to run each), how coding exercises are tested, and optionally the
// product repo whose protected paths must stay untouched.
//
// The one thing it cannot do that the Go version can is check gofmt formatting
// in-process. If track.json sets `language.gofmt`, it shells out to `gofmt`;
// with no gofmt on PATH it says so rather than pretending the check ran.

import { spawn } from "node:child_process";
import { createHash } from "node:crypto";
import { existsSync, mkdirSync, mkdtempSync, readdirSync, readFileSync, rmSync, statSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { basename, dirname, isAbsolute, join, resolve, sep } from "node:path";

// ---------------------------------------------------------------------------
// problems
// ---------------------------------------------------------------------------

const problems = [];
const add = (...parts) => problems.push(parts.join(""));
const fatal = (message) => {
  console.error("lessoncheck:", message);
  process.exit(2);
};

// ---------------------------------------------------------------------------
// strict JSON: an unknown key is a typo, and a typo silently does nothing
// ---------------------------------------------------------------------------

/** Shapes of the two files we read, so a misspelled key is an error not a shrug. */
const LANG_KEYS = ["key", "name", "file", "setup", "run", "check", "gofmt", "hljs", "runLabel"];
const SCHEMA = {
  track: {
    title: 1, eyebrow: 1, lede: 1, route: 1, module: 1, plan: 1, bonusCap: 1,
    language: LANG_KEYS, contrast: LANG_KEYS,
    exercise: ["setup", "test"],
    product: ["repo", "baseCommit", "protectedPaths", "requirements", "capabilitiesHeading", "pilotColumn"],
  },
  curriculum: {
    tracks: ["key", "label"],
    milestones: ["id", "code", "order", "title", "goal", "gate"],
    builds: ["id", "milestone", "title", "owner", "exercise", "state"],
    capabilities: ["id", "title", "milestone", "pilot", "state"],
    topics: ["id", "track", "title", "milestone", "kind", "usedBy", "reason", "depth", "relation", "lesson"],
  },
};

function readStrict(path, shape, what) {
  let parsed;
  try {
    parsed = JSON.parse(readFileSync(path, "utf8"));
  } catch (error) {
    throw new Error(`${basename(path)}: ${error.message}`);
  }
  const unknown = [];
  const walkObject = (value, allowed, where) => {
    for (const key of Object.keys(value ?? {})) {
      if (!(key in allowed) && !(Array.isArray(allowed) && allowed.includes(key))) {
        unknown.push(where ? `${where}.${key}` : key);
      }
    }
  };
  if (what === "track") {
    walkObject(parsed, SCHEMA.track, "");
    for (const key of ["language", "contrast"]) {
      if (parsed[key]) for (const k of Object.keys(parsed[key])) if (!LANG_KEYS.includes(k)) unknown.push(`${key}.${k}`);
    }
    if (parsed.exercise) for (const k of Object.keys(parsed.exercise)) if (!["setup", "test"].includes(k)) unknown.push(`exercise.${k}`);
    if (parsed.product) for (const k of Object.keys(parsed.product)) if (!SCHEMA.track.product.includes(k)) unknown.push(`product.${k}`);
  } else {
    walkObject(parsed, SCHEMA.curriculum, "");
    for (const [group, keys] of Object.entries(SCHEMA.curriculum)) {
      for (const row of parsed[group] ?? []) {
        for (const k of Object.keys(row)) if (!keys.includes(k)) unknown.push(`${group}[].${k}`);
      }
    }
  }
  if (unknown.length) throw new Error(`${basename(path)}: unknown field(s): ${[...new Set(unknown)].join(", ")}`);
  return parsed;
}

// ---------------------------------------------------------------------------
// lessons
// ---------------------------------------------------------------------------

const reConcept = /^## concept ([a-z0-9.-]+): (.+)$/;
const reExercise = /^## exercise ([a-z0-9.-]+) (predict|choice|write|fix|explain): (.+)$/;
const reHeader = /^(flags|practises|dir): (.+)$/;
const reOption = /^- \[( |x)\] (.+)$/;
const reHint = /^\d+\. (.+)$/;
const reCheck = /^- (.+)$/;
const reExit = /^exit status \d+$/;
const reTopicRow = /^\| ([a-z0-9]+\.[a-z0-9-]+) \|/;

const sectionOf = (unit, name) => unit.sections.find((s) => s.name === name);

function parseLesson(path, rel) {
  const errs = [];
  let raw;
  try {
    raw = readFileSync(path, "utf8");
  } catch (error) {
    return [null, [error.message]];
  }
  const lines = raw.replace(/\r\n/g, "\n").split("\n");
  const lesson = { id: "", title: "", milestone: "", file: rel, intro: [], concepts: [], exercises: [], _lines: lines };
  const fail = (n, message) => errs.push(`${rel}:${n + 1}: ${message}`);

  let i = 0;
  if (lines[0] === "---") {
    for (i = 1; i < lines.length && lines[i] !== "---"; i++) {
      const at = lines[i].indexOf(": ");
      if (at === -1) {
        fail(i, `bad front matter line "${lines[i]}"`);
        continue;
      }
      const key = lines[i].slice(0, at);
      const value = lines[i].slice(at + 2);
      if (key === "id") lesson.id = value;
      else if (key === "title") lesson.title = value;
      else if (key === "milestone") lesson.milestone = value;
      else fail(i, `unknown front matter key "${key}"`);
    }
    i++;
  } else {
    fail(0, "missing front matter");
  }

  let unit = null;
  let section = null;
  let text = [];
  const target = () => (section ? section.blocks : lesson.intro);
  const flushText = () => {
    const joined = text.join("\n").trim();
    if (joined) target().push({ k: "md", t: joined });
    text = [];
  };

  for (; i < lines.length; i++) {
    const line = lines[i];
    if (line.startsWith("## ")) {
      flushText();
      section = null;
      const concept = reConcept.exec(line);
      const exercise = reExercise.exec(line);
      if (concept) {
        unit = { kind: "concept", id: concept[1], title: concept[2], sections: [], _line: i };
        lesson.concepts.push(unit);
      } else if (exercise) {
        unit = { kind: "exercise", id: exercise[1], type: exercise[2], title: exercise[3], sections: [], _line: i };
        lesson.exercises.push(unit);
      } else {
        fail(i, "heading must be '## concept <id>: <title>' or '## exercise <id> <type>: <title>'");
        unit = null;
        continue;
      }
      while (i + 1 < lines.length) {
        const header = reHeader.exec(lines[i + 1]);
        if (!header) break;
        i++;
        const parts = header[2].split(",").map((p) => p.trim());
        if (header[1] === "flags") unit.flags = parts;
        else if (header[1] === "practises") unit.practises = parts;
        else if (header[1] === "dir") unit.dir = header[2];
      }
    } else if (line.startsWith("### ")) {
      flushText();
      if (!unit) {
        fail(i, "section outside a concept or exercise");
        continue;
      }
      section = { name: line.slice(4), blocks: [] };
      unit.sections.push(section);
    } else if (line.startsWith("```")) {
      flushText();
      const fields = line.slice(3).split(/\s+/).filter(Boolean);
      const block = { k: "code", run: "run", t: "" };
      if (fields.length) block.lang = fields[0];
      if (block.lang === "output") {
        block.k = "out";
        delete block.lang;
        delete block.run;
      }
      for (const field of fields.slice(1)) {
        if (field === "snippet") block.snippet = true;
        else if (field.startsWith("file=")) block.file = field.slice(5);
        else if (field === "run=check" || field === "run=vet") block.run = "check";
        else fail(i, `unknown code block option "${field}"`);
      }
      const start = i + 1;
      for (i++; i < lines.length && lines[i] !== "```"; i++);
      if (i >= lines.length) {
        fail(start - 1, "code block is never closed");
        break;
      }
      block.t = lines.slice(start, i).join("\n");
      block._from = start;
      block._to = i;
      target().push(block);
    } else {
      text.push(line);
    }
  }
  flushText();

  for (const exercise of lesson.exercises) {
    const options = sectionOf(exercise, "Options");
    if (options) {
      exercise.options = [];
      for (const block of options.blocks) {
        for (const line of block.t.split("\n")) {
          const m = reOption.exec(line);
          if (m) exercise.options.push({ text: m[2], correct: m[1] === "x" });
        }
      }
    }
    const hints = sectionOf(exercise, "Hints");
    if (hints) {
      exercise.hints = [];
      for (const block of hints.blocks) {
        for (const line of block.t.split("\n")) {
          const m = reHint.exec(line);
          if (m) exercise.hints.push(m[1]);
          else if (line.trim() && exercise.hints.length) exercise.hints[exercise.hints.length - 1] += " " + line.trim();
        }
      }
    }
    const checklist = sectionOf(exercise, "Checklist");
    if (checklist) {
      exercise.checklist = [];
      for (const block of checklist.blocks) {
        for (const line of block.t.split("\n")) {
          const m = reCheck.exec(line);
          if (m) exercise.checklist.push(m[1]);
        }
      }
    }
  }
  return [lesson, errs];
}

// ---------------------------------------------------------------------------
// running code
// ---------------------------------------------------------------------------

function normalize(text, tmp) {
  let s = text.replace(/\r\n/g, "\n");
  if (tmp) {
    s = s.split(tmp + sep).join("").split(tmp.replace(/\\/g, "/") + "/").join("").split(tmp).join("");
  }
  const keep = [];
  for (const line of s.split("\n")) {
    if (line.startsWith("# ") || reExit.test(line)) continue;
    let out = line;
    if (out.includes(".go:") || out.includes('.py"') || out.includes(".rs:")) out = out.replace(/\\/g, "/");
    keep.push(out.replace(/[ \t]+$/, ""));
  }
  return keep.join("\n").replace(/^\n+/, "").replace(/\n+$/, "");
}

/** Runs a command, capturing stdout and stderr together, like the Go version. */
function runCmd(dir, args, useShell = false) {
  return new Promise((done) => {
    if (!args?.length) return done({ out: "", ok: false, error: "empty command" });
    const child = spawn(args[0], args.slice(1), { cwd: dir, shell: useShell });
    let out = "";
    const collect = (chunk) => (out += chunk);
    child.stdout.on("data", collect);
    child.stderr.on("data", collect);
    const timer = setTimeout(() => child.kill(), 2 * 60 * 1000);
    child.on("error", async (error) => {
      clearTimeout(timer);
      // On Windows a .cmd or .bat launcher (npm, npx, pytest) cannot be spawned
      // directly. Retry through the shell rather than reporting it as missing.
      if (!useShell && process.platform === "win32" && error.code === "ENOENT") {
        return done(await runCmd(dir, args, true));
      }
      done({ out, ok: false, error: error.message });
    });
    child.on("close", (code) => {
      clearTimeout(timer);
      done({ out, ok: code === 0 });
    });
  });
}

function writeFiles(root, files, replace) {
  for (const [name, body] of Object.entries(files ?? {})) {
    const path = join(root, ...name.split("/"));
    mkdirSync(dirname(path), { recursive: true });
    writeFileSync(path, replace ? replace(body) : body);
  }
}

const withTemp = async (prefix, work) => {
  const dir = mkdtempSync(join(tmpdir(), prefix));
  try {
    return await work(dir);
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
};

/** Runs one example in a fresh temp dir and returns what it printed. */
async function runGroup(group, lang) {
  return withTemp("lessoncheck-", async (tmp) => {
    writeFiles(tmp, lang.setup);
    let mode = "run";
    const files = {};
    for (const block of group.code) {
      const name = block.file || lang.file;
      if (!block.file) mode = block.run;
      files[name] = block.t + "\n";
    }
    writeFiles(tmp, files);
    let args = lang.run;
    if (mode === "check") {
      if (!lang.check?.length) throw new Error(`a run=check block needs a 'check' command for ${lang.key} in track.json`);
      args = lang.check;
    }
    const { out } = await runCmd(tmp, args);
    return normalize(out, tmp);
  });
}

/** At most four examples at once, as in the Go version. */
async function inParallel(items, limit, work) {
  const results = new Array(items.length);
  let next = 0;
  const workers = Array.from({ length: Math.min(limit, items.length) }, async () => {
    while (true) {
      const i = next++;
      if (i >= items.length) return;
      results[i] = await work(items[i], i);
    }
  });
  await Promise.all(workers);
  return results;
}

// ---------------------------------------------------------------------------
// exercises
// ---------------------------------------------------------------------------

function parseTxtar(path) {
  const files = {};
  let current = "";
  let buf = [];
  const flush = () => {
    if (current) files[current] = buf.join("\n").replace(/\n+$/, "") + "\n";
    buf = [];
  };
  for (const line of readFileSync(path, "utf8").replace(/\r\n/g, "\n").split("\n")) {
    if (line.startsWith("-- ") && line.endsWith(" --") && line.length > 6) {
      flush();
      current = line.slice(3, -3);
      continue;
    }
    buf.push(line);
  }
  flush();
  return files;
}

const testArgs = (track, pkgDir) => (track.exercise?.test ?? []).map((a) => a.split("{dir}").join(pkgDir));

/** Builds the module layout in a temp dir and runs the track's test command. */
async function testIn(track, pkgDir, files) {
  return withTemp("lessoncheck-ex-", async (tmp) => {
    writeFiles(tmp, track.exercise?.setup, (body) => body.split("{module}").join(track.module ?? ""));
    const put = {};
    for (const [name, body] of Object.entries(files)) put[`${pkgDir}/${name}`] = body;
    writeFiles(tmp, put);
    const { out, ok } = await runCmd(tmp, testArgs(track, pkgDir));
    return { ok, out: normalize(out, tmp) };
  });
}

// ---------------------------------------------------------------------------
// Markdown tables
// ---------------------------------------------------------------------------

function tableAfter(md, heading) {
  const rows = [];
  let head = null;
  let inside = false;
  for (const line of (md ?? "").replace(/\r\n/g, "\n").split("\n")) {
    if (line.trim() === heading) {
      inside = true;
      continue;
    }
    if (!inside) continue;
    if (line.startsWith("#")) break;
    if (!line.startsWith("|")) {
      if (head) break;
      continue;
    }
    const cells = line.replace(/^\|/, "").replace(/\|$/, "").split("|").map((c) => c.trim());
    if (!head) {
      head = cells;
      continue;
    }
    if (cells[0].startsWith("---")) continue;
    const row = {};
    head.forEach((name, i) => {
      if (i < cells.length) row[name] = cells[i];
    });
    rows.push(row);
  }
  return rows;
}

// ---------------------------------------------------------------------------
// build briefs
// ---------------------------------------------------------------------------

const needsLineRe = /needs:\s*(.+)$/gm;
const topicIDRe = /[a-z]+\.[a-z0-9-]+/g;

function loadBriefs(root, curriculum) {
  const briefs = {};
  const dir = join(root, "_builds");
  if (!existsSync(dir)) return briefs;

  const buildIDs = new Set((curriculum.builds ?? []).map((b) => b.id));
  const topicIDs = new Set((curriculum.topics ?? []).map((t) => t.id));

  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    if (!entry.isDirectory()) continue;
    const id = entry.name;
    const at = `_builds/${id}`;
    if (!buildIDs.has(id)) {
      add(`${at}: no build step with id "${id}" in the curriculum`);
      continue;
    }
    const readme = join(dir, id, "README.md");
    if (!existsSync(readme)) {
      add(`${at}/README.md: no such file`);
      continue;
    }
    const body = readFileSync(readme, "utf8");
    if (!body.includes("## Compare")) {
      add(`${at}: a brief needs a '## Compare' section — the reference to read once the tests pass`);
    }
    if (!body.includes("## Done when")) {
      add(`${at}: a brief needs a '## Done when' section — the tests are the only definition of done`);
    }

    const needs = [];
    const seen = new Set();
    for (const match of body.matchAll(needsLineRe)) {
      for (const topic of match[1].match(topicIDRe) ?? []) {
        if (!topicIDs.has(topic)) {
          add(`${at}: needs: ${topic} — no such topic in the curriculum`);
          continue;
        }
        if (!seen.has(topic)) {
          seen.add(topic);
          needs.push(topic);
        }
      }
    }
    if (!needs.length) {
      add(`${at}: no 'needs: <topic-id>' tags — without them a stuck reader has nowhere to go (R17)`);
    }
    needs.sort();

    const tests = {};
    for (const file of readdirSync(join(dir, id), { withFileTypes: true })) {
      if (file.isDirectory() || !/_test\.[a-z]+$/.test(file.name)) continue;
      tests[file.name] = readFileSync(join(dir, id, file.name), "utf8");
    }
    if (!Object.keys(tests).length) {
      add(`${at}: no test files — a brief with no contract tests has no definition of done (R17)`);
    }
    briefs[id] = { build: id, dir: at, markdown: body, needs, tests };
  }
  return briefs;
}

// ---------------------------------------------------------------------------
// shaping content.json the way the board expects
// ---------------------------------------------------------------------------

/** Go omits an empty slice with `omitempty`, and writes null for a nil one. */
const some = (a) => (a && a.length ? a : undefined);
const orNull = (a) => (a && a.length ? a : null);

const blockOut = (b) => ({
  k: b.k,
  ...(b.lang ? { lang: b.lang } : {}),
  ...(b.file ? { file: b.file } : {}),
  ...(b.run ? { run: b.run } : {}),
  ...(b.snippet ? { snippet: true } : {}),
  t: b.t,
});

const unitOut = (u) => ({
  kind: u.kind,
  id: u.id,
  ...(u.type ? { type: u.type } : {}),
  title: u.title,
  ...(some(u.flags) ? { flags: u.flags } : {}),
  ...(some(u.practises) ? { practises: u.practises } : {}),
  ...(u.dir ? { dir: u.dir } : {}),
  sections: orNull((u.sections ?? []).map((s) => ({ name: s.name, blocks: orNull(s.blocks.map(blockOut)) }))),
  ...(some(u.options) ? { options: u.options } : {}),
  ...(some(u.hints) ? { hints: u.hints } : {}),
  ...(some(u.checklist) ? { checklist: u.checklist } : {}),
  ...(u.files ? { files: u.files } : {}),
  ...(u.command ? { command: u.command } : {}),
  ...(u.practice ? { practice: u.practice } : {}),
  ...(some(u.practisedBy) ? { practisedBy: u.practisedBy } : {}),
});

const lessonOut = (l) => ({
  id: l.id,
  title: l.title,
  milestone: l.milestone,
  file: l.file,
  intro: orNull(l.intro.map(blockOut)),
  concepts: orNull(l.concepts.map(unitOut)),
  exercises: orNull(l.exercises.map(unitOut)),
});

// ---------------------------------------------------------------------------
// main
// ---------------------------------------------------------------------------

async function main() {
  const update = process.argv.slice(2).some((a) => a === "-update" || a === "--update");
  const root = process.cwd();

  let track;
  try {
    track = readStrict(join(root, "track.json"), SCHEMA.track, "track");
  } catch (error) {
    fatal(`run this from the learning folder; ${error.message}`);
  }
  track.bonusCap ||= 2;
  track.plan ||= "plan.md";

  const langs = { [track.language.key]: track.language };
  if (track.contrast) langs[track.contrast.key] = track.contrast;

  let product = "";
  if (track.product) {
    product = resolve(isAbsolute(track.product.repo) ? track.product.repo : join(root, track.product.repo));
    if (!existsSync(product)) fatal(`product repo not found at ${product} (track.json product.repo)`);
  }

  let curriculum;
  try {
    curriculum = readStrict(join(root, "board", "curriculum.json"), SCHEMA.curriculum, "curriculum");
  } catch (error) {
    fatal(error.message);
  }
  for (const group of ["tracks", "milestones", "builds", "capabilities", "topics"]) curriculum[group] ??= [];

  // --- curriculum ----------------------------------------------------------
  const trackKeys = new Set(curriculum.tracks.map((t) => t.key));
  if (!trackKeys.size) add("curriculum: 'tracks' must list at least one track");

  const msOrder = new Map(curriculum.milestones.map((m) => [m.id, m.order]));
  const msCode = new Map(curriculum.milestones.map((m) => [m.id, m.code]));
  const ids = new Map();
  const seen = (id, what) => {
    if (ids.has(id)) add(`curriculum: id "${id}" is used by both a ${ids.get(id)} and a ${what}`);
    ids.set(id, what);
  };
  const oneOf = (value, ...options) => options.includes(value);

  for (const m of curriculum.milestones) seen(m.id, "milestone");

  const builds = new Map();
  for (const b of curriculum.builds) {
    seen(b.id, "build step");
    builds.set(b.id, b);
    if (!msOrder.has(b.milestone)) add(`curriculum: build ${b.id}: unknown milestone "${b.milestone}"`);
    if (!oneOf(b.owner, "you", "claude", "both")) add(`curriculum: build ${b.id}: owner must be you, claude or both`);
    if (!oneOf(b.state, "planned", "in-progress", "done")) add(`curriculum: build ${b.id}: state must be planned, in-progress or done`);
  }
  for (const c of curriculum.capabilities) {
    seen(c.id, "capability");
    if (!msOrder.has(c.milestone)) add(`curriculum: capability ${c.id}: unknown milestone "${c.milestone}"`);
    if (!oneOf(c.state, "planned", "in-progress", "working")) add(`curriculum: capability ${c.id}: state must be planned, in-progress or working`);
  }

  const topics = new Map();
  const bonusPer = new Map();
  for (const t of curriculum.topics) {
    seen(t.id, "topic");
    topics.set(t.id, t);
    if (!trackKeys.has(t.track)) add(`curriculum: topic ${t.id}: track "${t.track}" is not in 'tracks'`);
    if (t.kind === "core" || t.kind === "bonus") {
      if (t.milestone == null) add(`curriculum: topic ${t.id}: a ${t.kind} topic needs a milestone`);
      else if (!msOrder.has(t.milestone)) add(`curriculum: topic ${t.id}: unknown milestone "${t.milestone}"`);
      if (t.kind === "core") {
        const b = t.usedBy == null ? undefined : builds.get(t.usedBy);
        if (t.usedBy == null || !b) add(`curriculum: core topic ${t.id} must name the build step that uses it (usedBy)`);
        else if (t.milestone != null && msOrder.get(b.milestone) < msOrder.get(t.milestone)) {
          add(`curriculum: topic ${t.id} is taught in ${t.milestone} but used earlier, by ${b.id}`);
        }
      } else {
        if (t.usedBy != null) add(`curriculum: bonus topic ${t.id} must not have usedBy`);
        if (t.milestone != null) bonusPer.set(t.milestone, (bonusPer.get(t.milestone) ?? 0) + 1);
      }
    } else if (t.kind === "not-planned") {
      if (!t.reason || t.milestone != null || t.usedBy != null) {
        add(`curriculum: not-planned topic ${t.id} needs a reason and no milestone or usedBy`);
      }
    } else {
      add(`curriculum: topic ${t.id}: kind must be core, bonus or not-planned`);
    }
    if (!oneOf(t.depth, "core", "interview", "advanced")) add(`curriculum: topic ${t.id}: depth must be core, interview or advanced`);
  }
  for (const [milestone, n] of bonusPer) {
    if (n > track.bonusCap) add(`curriculum: milestone ${milestone} has ${n} bonus topics; the cap is ${track.bonusCap}`);
  }

  // --- lessons -------------------------------------------------------------
  const lessonsDir = join(root, "lessons");
  const files = [];
  if (existsSync(lessonsDir)) {
    for (const milestone of readdirSync(lessonsDir, { withFileTypes: true })) {
      if (!milestone.isDirectory()) continue;
      for (const file of readdirSync(join(lessonsDir, milestone.name))) {
        if (file.endsWith(".md")) files.push(join(lessonsDir, milestone.name, file));
      }
    }
  }
  files.sort();

  const lessons = [];
  const lessonIDs = new Set();
  for (const file of files) {
    const rel = file.slice(root.length + 1).split(sep).join("/");
    const [lesson, errs] = parseLesson(file, rel);
    for (const e of errs) add(e);
    if (!lesson) continue;
    if (lessonIDs.has(lesson.id)) add(`${rel}: duplicate lesson id "${lesson.id}"`);
    lessonIDs.add(lesson.id);
    if (!msOrder.has(lesson.milestone)) add(`${rel}: unknown milestone "${lesson.milestone}"`);
    lessons.push(lesson);
  }

  const contrastSection = track.contrast ? `In ${track.contrast.name}` : "";
  const taught = new Map();
  for (const lesson of lessons) {
    if (lesson.concepts.length < 3 || lesson.concepts.length > 8) {
      add(`${lesson.file}: a lesson has 3–8 concepts; this one has ${lesson.concepts.length}`);
    }
    for (const concept of lesson.concepts) {
      const at = `${lesson.file}:${concept._line + 1}: concept ${concept.id}`;
      const topic = topics.get(concept.id);
      if (!topic) {
        add(`${at}: not a topic in curriculum.json`);
        continue;
      }
      if (taught.has(concept.id)) add(`${at}: already taught in ${taught.get(concept.id)}`);
      taught.set(concept.id, lesson.id);
      topic.lesson = lesson.id;
      if (topic.milestone == null || topic.milestone !== lesson.milestone) {
        add(`${at}: the curriculum puts this topic in ${topic.milestone ?? ""}, not ${lesson.milestone}`);
      }
      const flags = concept.flags ?? [];
      if (flags.length !== 2 || !oneOf(flags[0], "core", "interview", "advanced") || !oneOf(flags[1], "transfers", "differs", "new")) {
        add(`${at}: flags must be '<core|interview|advanced>, <transfers|differs|new>'`);
      }
      const allowed = new Set(["Example", "Explain", "More", "Takeaway"]);
      if (contrastSection) allowed.add(contrastSection);
      for (const section of concept.sections) {
        if (!allowed.has(section.name)) add(`${at}: unknown section "${section.name}"`);
        // The comparison is collapsed on the board, so nothing about the
        // language being learned may live only there.
        if (contrastSection && section.name === contrastSection) {
          for (const block of section.blocks) {
            if (block.k === "code" && block.lang === track.language.key) {
              add(`${at}: ${track.language.name} code in the "${contrastSection}" section is hidden when it's collapsed; move it to More`);
            }
          }
        }
      }
      const need = ["Example", "Explain", "Takeaway"];
      if (contrastSection && topic.track === track.language.key) need.push(contrastSection);
      for (const name of need) if (!sectionOf(concept, name)) add(`${at}: missing section "${name}"`);

      const explain = sectionOf(concept, "Explain");
      if (explain) {
        let words = 0;
        for (const block of explain.blocks) {
          if (block.k !== "md") add(`${at}: Explain must be prose only`);
          words += block.t.split(/\s+/).filter(Boolean).length;
        }
        if (words > 80) add(`${at}: Explain is ${words} words; the format allows 80`);
      }
      const takeaway = sectionOf(concept, "Takeaway");
      if (takeaway && (takeaway.blocks.length !== 1 || takeaway.blocks[0].t.includes("\n"))) {
        add(`${at}: Takeaway must be a single line`);
      }
      const example = sectionOf(concept, "Example");
      if (example && !example.blocks.some((b) => b.k === "out")) add(`${at}: Example needs code with its output`);
    }
  }

  // --- exercises -----------------------------------------------------------
  const practisedBy = new Map();
  const allExercises = [];
  for (const lesson of lessons) {
    let auto = 0;
    for (const exercise of lesson.exercises) {
      allExercises.push(exercise);
      const at = `${lesson.file}:${exercise._line + 1}: exercise ${exercise.id}`;
      seen(exercise.id, "exercise");
      if (!exercise.practises?.length) add(`${at}: must name the concepts it practises`);
      for (const topic of exercise.practises ?? []) {
        practisedBy.set(topic, [...(practisedBy.get(topic) ?? []), exercise.id]);
      }
      const has = (...names) => {
        for (const name of names) if (!sectionOf(exercise, name)) add(`${at}: missing section "${name}"`);
      };
      if (exercise.type === "predict" || exercise.type === "choice") {
        auto++;
        has("Prompt", "Answer", "Why");
        if (exercise.type === "choice") {
          const correct = (exercise.options ?? []).filter((o) => o.correct).length;
          if ((exercise.options ?? []).length < 2 || correct !== 1) {
            add(`${at}: a choice needs at least two options and exactly one [x]`);
          }
        }
      } else if (exercise.type === "write" || exercise.type === "fix") {
        has("Task", "Hints", "Why");
        if (exercise.type === "write" && (exercise.hints ?? []).length !== 3) {
          add(`${at}: a write exercise has exactly three hints (nudge, approach, nearly the answer); found ${(exercise.hints ?? []).length}`);
        }
        if (exercise.type === "fix" && !(exercise.hints ?? []).length) add(`${at}: a fix exercise needs hints`);
        if (!exercise.dir) add(`${at}: needs 'dir:' (its folder under practice/)`);
      } else if (exercise.type === "explain") {
        has("Task", "Checklist");
        if ((exercise.checklist ?? []).length < 2) add(`${at}: an explain exercise needs a checklist of at least two points`);
      }
    }
    if (auto < 2) {
      add(`${lesson.file}: needs at least two auto-checked exercises (predict or choice) so the lesson can be tested out of; has ${auto}`);
    }
  }
  for (const exercise of allExercises) {
    for (const topic of exercise.practises ?? []) {
      if (!taught.has(topic)) add(`exercise ${exercise.id} practises ${topic}, which no lesson teaches`);
    }
  }
  for (const lesson of lessons) {
    for (const concept of lesson.concepts) {
      concept.practisedBy = practisedBy.get(concept.id) ?? [];
      if (!concept.practisedBy.length) add(`${lesson.file}: concept ${concept.id} is not practised by any exercise`);
    }
  }
  for (const build of curriculum.builds) {
    if (build.exercise != null && !allExercises.some((e) => e.id === build.exercise)) {
      add(`curriculum: build ${build.id} points to exercise ${build.exercise}, which doesn't exist`);
    }
  }
  const written = new Set(lessons.map((l) => l.milestone));
  for (const topic of curriculum.topics) {
    if (topic.kind === "core" && topic.milestone != null && written.has(topic.milestone) && !topic.lesson) {
      add(`curriculum: ${topic.milestone} has lessons, but core topic ${topic.id} isn't taught in any of them`);
    }
  }

  // --- collect every example to run ----------------------------------------
  const groups = [];
  for (const lesson of lessons) {
    const collect = (where, blocks) => {
      let pending = [];
      for (const block of blocks) {
        if (block.k === "code" && block.snippet) continue;
        if (block.k === "code") {
          if (pending.length && pending[0].lang !== block.lang) {
            add(`${where}: a ${pending[0].lang} block and a ${block.lang} block are grouped together`);
          }
          pending.push(block);
        } else if (block.k === "out") {
          if (!pending.length) {
            add(`${where}: output block with no code before it`);
            continue;
          }
          groups.push({ where, code: pending, out: block, file: lesson.file });
          pending = [];
        } else if (pending.length) {
          add(`${where}: code block is shown but never run; add an output block after it or mark it 'snippet'`);
          pending = [];
        }
      }
      if (pending.length) {
        add(`${where}: code block is shown but never run; add an output block after it or mark it 'snippet'`);
      }
    };
    for (const concept of lesson.concepts) {
      for (const section of concept.sections) collect(`${lesson.file} › ${concept.id} › ${section.name}`, section.blocks);
    }
    for (const exercise of lesson.exercises) {
      if (exercise.type === "predict" || exercise.type === "choice") {
        // The prompt's code and the answer's output are one group: the answer
        // stays hidden on the board until the learner commits.
        const prompt = sectionOf(exercise, "Prompt");
        const answer = sectionOf(exercise, "Answer");
        if (!prompt || !answer) continue;
        const code = prompt.blocks.filter((b) => b.k === "code" && !b.snippet);
        const out = [...answer.blocks].reverse().find((b) => b.k === "out");
        if (!code.length || !out) {
          add(`${lesson.file} › ${exercise.id}: needs code in Prompt and an output block in Answer`);
          continue;
        }
        groups.push({ where: `${lesson.file} › ${exercise.id}`, code, out, file: lesson.file });
        continue;
      }
      for (const section of exercise.sections) collect(`${lesson.file} › ${exercise.id} › ${section.name}`, section.blocks);
    }
  }

  // Every group must be in a language track.json knows how to run.
  let gofmt = null;
  for (const group of groups) {
    const lang = langs[group.code[0].lang];
    if (!lang) {
      add(`${group.where}: no way to run "${group.code[0].lang}" code; add it to track.json (language or contrast) or mark the block 'snippet'`);
      continue;
    }
    if (!lang.gofmt) continue;
    if (gofmt === null) gofmt = (await runCmd(root, ["gofmt", "--help"])).error ? false : true;
    if (gofmt === false) continue;
    for (const block of group.code) {
      const formatted = await withTemp("lessoncheck-fmt-", async (tmp) => {
        writeFiles(tmp, { "x.go": block.t + "\n" });
        return runCmd(tmp, ["gofmt", "-l", "x.go"]);
      });
      if (formatted.out.trim()) add(`${group.where}: example isn't gofmt-formatted`);
    }
  }
  if (gofmt === false) {
    add("track.json asks for gofmt-formatted examples but gofmt is not on PATH; install Go or set language.gofmt to false");
  }

  // --- run them ------------------------------------------------------------
  const runnable = groups.filter((g) => langs[g.code[0].lang]);
  const results = await inParallel(runnable, 4, async (group) => {
    try {
      return { got: await runGroup(group, langs[group.code[0].lang]) };
    } catch (error) {
      return { error: error.message };
    }
  });

  const changed = new Map();
  let primaryRuns = 0;
  let contrastRuns = 0;
  runnable.forEach((group, i) => {
    const result = results[i];
    if (result.error) {
      add(`${group.where}: could not run: ${result.error}`);
      return;
    }
    if (group.code[0].lang === track.language.key) primaryRuns++;
    else contrastRuns++;
    const want = group.out.t.replace(/\r\n/g, "\n").replace(/^\n+/, "").replace(/\n+$/, "");
    if (result.got === want) return;
    if (update) {
      changed.set(group.file, [...(changed.get(group.file) ?? []), { from: group.out._from, to: group.out._to, text: result.got }]);
      return;
    }
    add(`${group.where}: shown output does not match a real run\n    shown:  ${JSON.stringify(want)}\n    actual: ${JSON.stringify(result.got)}`);
  });

  if (update) {
    let n = 0;
    for (const lesson of lessons) {
      const edits = changed.get(lesson.file);
      if (!edits?.length) continue;
      edits.sort((a, b) => b.from - a.from);
      let lines = [...lesson._lines];
      for (const edit of edits) {
        lines = [...lines.slice(0, edit.from), ...(edit.text ? edit.text.split("\n") : []), ...lines.slice(edit.to)];
        n++;
      }
      writeFileSync(join(root, ...lesson.file.split("/")), lines.join("\n"));
    }
    console.log(`updated ${n} output block(s) from real runs; re-read the explanations next to them, then run again without -update`);
    return;
  }

  // --- exercises: starter fails, reference passes ---------------------------
  let startersFail = 0;
  let refsPass = 0;
  let practicePass = 0;
  let practiceTotal = 0;
  for (const lesson of lessons) {
    for (const exercise of lesson.exercises) {
      if (exercise.type !== "write" && exercise.type !== "fix") continue;
      const bundle = join(root, "lessons", lesson.milestone, "exercises", `${exercise.id}.txtar`);
      let bundled;
      try {
        bundled = parseTxtar(bundle);
      } catch (error) {
        add(`exercise ${exercise.id}: ${error.message}`);
        continue;
      }
      const files = { starter: {}, solution: {}, tests: {} };
      for (const [name, body] of Object.entries(bundled)) {
        if (name.startsWith("starter/")) files.starter[name.slice(8)] = body;
        else if (name.startsWith("solution/")) files.solution[name.slice(9)] = body;
        else files.tests[name] = body;
      }
      if (!Object.keys(files.starter).length || !Object.keys(files.solution).length || !Object.keys(files.tests).length) {
        add(`exercise ${exercise.id}: the bundle needs starter/, solution/ and test files`);
        continue;
      }
      exercise.files = files;
      const pkgDir = `practice/${exercise.dir}`;
      exercise.practice = pkgDir;
      exercise.command = testArgs(track, pkgDir).join(" ");
      const withTests = (code) => ({ ...code, ...files.tests });

      const starter = await testIn(track, pkgDir, withTests(files.starter));
      if (starter.ok) add(`exercise ${exercise.id}: the starter already passes its tests, so there's nothing to do`);
      else startersFail++;

      const reference = await testIn(track, pkgDir, withTests(files.solution));
      if (!reference.ok) add(`exercise ${exercise.id}: the reference solution fails its tests:\n${reference.out}`);
      else refsPass++;

      // The learner's workspace: created from the starter if missing, never
      // overwritten. Its tests must stay the originals.
      const workspace = join(root, ...pkgDir.split("/"));
      if (!existsSync(workspace)) {
        writeFiles(workspace, withTests(files.starter));
        console.log(`created ${pkgDir} from the starter`);
      }
      for (const [name, body] of Object.entries(files.tests)) {
        const path = join(workspace, name);
        const got = existsSync(path) ? readFileSync(path, "utf8").replace(/\r\n/g, "\n") : null;
        if (got !== body) {
          const from = bundle.slice(root.length + 1).split(sep).join("/");
          add(`exercise ${exercise.id}: ${pkgDir}/${name} differs from the original test; restore it from ${from}`);
        }
      }
      practiceTotal++;
      if ((await runCmd(root, testArgs(track, pkgDir))).ok) practicePass++;
    }
  }

  // --- the plan's tables must agree with the curriculum ---------------------
  let plan = "";
  try {
    plan = readFileSync(join(root, track.plan), "utf8");
  } catch (error) {
    add(`${track.plan}: ${error.message}`);
  }
  const inPlan = new Set();
  for (const line of plan.replace(/\r\n/g, "\n").split("\n")) {
    const m = reTopicRow.exec(line);
    if (!m || !topics.has(m[1])) {
      if (m && (line.match(/\|/g) ?? []).length >= 6) add(`${track.plan}: topic ${m[1]} is not in curriculum.json`);
      continue;
    }
    const cells = line.split("|");
    if (cells.length < 7) continue;
    const id = m[1];
    const ms = cells[3].trim();
    const kind = cells[5].trim();
    inPlan.add(id);
    const topic = topics.get(id);
    const wantMs = topic.milestone != null ? msCode.get(topic.milestone) : "—";
    const planKind = kind.startsWith("not planned") ? "not-planned" : kind;
    if (ms !== wantMs || planKind !== topic.kind) {
      add(`${track.plan}: topic ${id} says ${ms}/${planKind}, curriculum.json says ${wantMs}/${topic.kind}`);
    }
  }
  for (const id of topics.keys()) if (!inPlan.has(id)) add(`${track.plan}: topic ${id} is missing from the tables`);

  // --- requirements, the log, and the protected product paths --------------
  let requirements = "";
  try {
    requirements = readFileSync(join(root, "requirements.md"), "utf8");
  } catch (error) {
    add(`requirements.md: ${error.message}`);
  }
  let productMD = "";
  const docs = [{ source: "learning", md: requirements }];
  if (track.product?.requirements) {
    try {
      productMD = readFileSync(join(product, ...track.product.requirements.split("/")), "utf8");
    } catch (error) {
      add(`product requirements: ${error.message}`);
    }
    docs.unshift({ source: "product", md: productMD });
  }
  const open = [];
  const decided = [];
  for (const doc of docs) {
    const rows = tableAfter(doc.md, "### Decided");
    if (!rows.length) add(`${doc.source} requirements: couldn't read the Decided table`);
    for (const row of rows) decided.push({ ...row, source: doc.source });
    for (const row of tableAfter(doc.md, "### Still open")) {
      if (row.id === "—" || !row.id) continue;
      open.push({ ...row, source: doc.source });
    }
  }
  if (track.product?.capabilitiesHeading) {
    const caps = tableAfter(productMD, track.product.capabilitiesHeading);
    if (caps.length !== curriculum.capabilities.length) {
      add(`product requirements: ${caps.length} capabilities listed, curriculum.json has ${curriculum.capabilities.length}`);
    }
    for (const capability of curriculum.capabilities) {
      const row = caps.find((r) => r.id === capability.id);
      if (!row) {
        add(`product requirements: capability ${capability.id} is missing from its table`);
        continue;
      }
      if (track.product.pilotColumn) {
        const pilot = (row[track.product.pilotColumn] ?? "").toLowerCase().startsWith("yes");
        if (pilot !== Boolean(capability.pilot)) {
          add(`product requirements: ${capability.id} is pilot=${pilot} there but pilot=${Boolean(capability.pilot)} in curriculum.json`);
        }
      }
    }
  }
  let log = "";
  try {
    log = readFileSync(join(root, "log.md"), "utf8");
  } catch (error) {
    add(`log.md: ${error.message}`);
  }

  let protectedState = "n/a";
  if (track.product?.protectedPaths?.length) {
    const top = await runCmd(product, ["git", "rev-parse", "--show-toplevel"]);
    if (!top.ok) {
      add(`git in ${product}: ${top.out.trim() || top.error}`);
    } else {
      const repo = top.out.trim();
      const diff = await runCmd(repo, ["git", "diff", "--name-only", track.product.baseCommit, "--", ...track.product.protectedPaths]);
      if (!diff.ok) add(`git diff: ${diff.out}`);
      const status = await runCmd(repo, ["git", "status", "--porcelain", "--", ...track.product.protectedPaths]);
      if (!status.ok) add(`git status: ${status.out}`);
      for (const line of `${diff.out}\n${status.out}`.trim().split("\n")) {
        if (line.trim()) add(`protected product file changed: ${line.trim()}`);
      }
      protectedState = "yes";
    }
  }

  const briefs = loadBriefs(root, curriculum);

  if (problems.length) {
    problems.sort();
    console.error(`lessoncheck: ${problems.length} problem(s)`);
    for (const problem of problems) console.error("  " + problem);
    process.exit(1);
  }

  // --- everything holds: write what the board displays ----------------------
  const view = (lang) =>
    lang ? { Key: lang.key, Name: lang.name, File: lang.file, RunLabel: lang.runLabel, CheckLabel: (lang.check ?? []).join(" "), Hljs: lang.hljs } : null;

  const content = {
    track: {
      title: track.title,
      eyebrow: track.eyebrow,
      lede: track.lede,
      route: track.route ?? null,
      language: view(track.language),
      contrast: view(track.contrast),
    },
    curriculum,
    lessons: lessons.map(lessonOut),
    briefs,
    docs: { requirements, productRequirements: productMD, log },
    open: orNull(open),
    decided: orNull(decided),
    workdir: root,
  };
  const version = createHash("sha256").update(JSON.stringify(content)).digest("hex").slice(0, 12);
  content.version = version;
  mkdirSync(join(root, "board"), { recursive: true });
  writeFileSync(join(root, "board", "content.json"), JSON.stringify(content, null, 1) + "\n");

  const concepts = lessons.reduce((n, l) => n + l.concepts.length, 0);
  const exercises = lessons.reduce((n, l) => n + l.exercises.length, 0);
  console.log(
    `lessoncheck ok: lessons=${lessons.length} concepts=${concepts} exercises=${exercises} ` +
      `examples-verified=${primaryRuns} contrast-examples-verified=${contrastRuns} starters-fail=${startersFail} ` +
      `references-pass=${refsPass} practice-passing=${practicePass}/${practiceTotal} topics=${curriculum.topics.length} ` +
      `protected-untouched=${protectedState} version=${version}`,
  );
}

main().catch((error) => fatal(error.stack ?? error.message));
