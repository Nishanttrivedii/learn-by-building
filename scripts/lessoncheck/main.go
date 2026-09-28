// Command lessoncheck verifies a learn-by-building track and builds the file
// its board displays.
//
// Copy this folder to <learning folder>/tools/lessoncheck and run it from the
// learning folder:
//
//	go run ./tools/lessoncheck            verify everything, then write board/content.json
//	go run ./tools/lessoncheck -update    rewrite every output block from a real run instead
//
// Everything project-specific comes from track.json in the learning folder:
// the language being learned and the one the learner already knows (how to run
// each), how coding exercises are tested, and optionally the product repo whose
// protected paths must stay untouched. See the skill's references/verification.md.
//
// It checks that the curriculum is consistent, that every lesson has the blocks
// its format requires, that every output shown to the learner is what the code
// really prints, that every exercise's starter fails its tests while the
// reference passes, and that no protected product file has changed. It writes
// board/content.json only when all of that holds.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/format"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// ---------- track.json ----------

// Lang says how to run examples written in one language.
type Lang struct {
	Key      string            `json:"key"`      // the code-fence tag, e.g. "go", "js", "python"
	Name     string            `json:"name"`     // shown to the learner, e.g. "Go", "JavaScript"
	File     string            `json:"file"`     // file an example is written to, e.g. "main.go"
	Setup    map[string]string `json:"setup"`    // extra files for the temp dir, e.g. {"go.mod": "module example\n\ngo 1.25\n"}
	Run      []string          `json:"run"`      // runs an example, e.g. ["go","run","."]
	Check    []string          `json:"check"`    // for blocks marked run=check (or run=vet), e.g. ["go","vet","."]
	Gofmt    bool              `json:"gofmt"`    // require gofmt-formatted examples (Go only)
	Hljs     string            `json:"hljs"`     // highlight.js language id for the board
	RunLabel string            `json:"runLabel"` // caption on the board, e.g. "go run ."
}

type Product struct {
	Repo                string   `json:"repo"`                // path to the product repo, relative to the learning folder
	BaseCommit          string   `json:"baseCommit"`          // protected paths must not change since this commit
	ProtectedPaths      []string `json:"protectedPaths"`      // e.g. code the build must not touch yet
	Requirements        string   `json:"requirements"`        // product requirements doc, relative to the repo
	CapabilitiesHeading string   `json:"capabilitiesHeading"` // heading of its capability table, e.g. "## 5. Capabilities"
	PilotColumn         string   `json:"pilotColumn"`         // column saying yes/no for the first slice
}

type Track struct {
	Title    string   `json:"title"`   // e.g. "Unified Shipping Logbook"
	Eyebrow  string   `json:"eyebrow"` // small line above the title
	Lede     string   `json:"lede"`    // one or two sentences under the title
	Route    []string `json:"route"`   // optional: the build's order, shown as a strip
	Module   string   `json:"module"`  // module/package root used when testing exercises
	Plan     string   `json:"plan"`    // file with the topic tables, e.g. "plan.md"
	BonusCap int      `json:"bonusCap"`
	Language Lang     `json:"language"`
	Contrast *Lang    `json:"contrast"` // the language the learner already knows; optional
	Exercise struct {
		Setup map[string]string `json:"setup"` // files at the temp root; "{module}" is replaced
		Test  []string          `json:"test"`  // e.g. ["go","test","./{dir}/"]; "{dir}" is practice/<dir>
	} `json:"exercise"`
	Product *Product `json:"product"`
}

// ---------- curriculum ----------

type TrackDef struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

type Milestone struct {
	ID    string `json:"id"`
	Code  string `json:"code"`
	Order int    `json:"order"`
	Title string `json:"title"`
	Goal  string `json:"goal"`
	Gate  string `json:"gate"`
}

type Build struct {
	ID        string  `json:"id"`
	Milestone string  `json:"milestone"`
	Title     string  `json:"title"`
	Owner     string  `json:"owner"`
	Exercise  *string `json:"exercise"`
	State     string  `json:"state"`
}

type Capability struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Milestone string `json:"milestone"`
	Pilot     bool   `json:"pilot"`
	State     string `json:"state"`
}

type Topic struct {
	ID        string  `json:"id"`
	Track     string  `json:"track"`
	Title     string  `json:"title"`
	Milestone *string `json:"milestone"`
	Kind      string  `json:"kind"`
	UsedBy    *string `json:"usedBy"`
	Reason    string  `json:"reason,omitempty"`
	Depth     string  `json:"depth"`
	Relation  string  `json:"relation,omitempty"`
	Lesson    string  `json:"lesson,omitempty"` // filled in from the lessons
}

type Curriculum struct {
	Tracks       []TrackDef   `json:"tracks"`
	Milestones   []Milestone  `json:"milestones"`
	Builds       []Build      `json:"builds"`
	Capabilities []Capability `json:"capabilities"`
	Topics       []Topic      `json:"topics"`
}

// ---------- lessons ----------

// Block is one piece of a section: Markdown text, a code block, or an output
// block. Output blocks remember where their content sits in the file so that
// -update can rewrite them.
type Block struct {
	K       string `json:"k"` // md | code | out
	Lang    string `json:"lang,omitempty"`
	File    string `json:"file,omitempty"`
	Run     string `json:"run,omitempty"` // run | check
	Snippet bool   `json:"snippet,omitempty"`
	T       string `json:"t"`

	from, to int // output content lines [from, to) in the source file
}

type Section struct {
	Name   string  `json:"name"`
	Blocks []Block `json:"blocks"`
}

type Option struct {
	Text    string `json:"text"`
	Correct bool   `json:"correct"`
}

type ExerciseFiles struct {
	Starter  map[string]string `json:"starter"`
	Solution map[string]string `json:"solution"`
	Tests    map[string]string `json:"tests"`
}

type Unit struct {
	Kind        string         `json:"kind"` // concept | exercise
	ID          string         `json:"id"`
	Type        string         `json:"type,omitempty"` // exercise type
	Title       string         `json:"title"`
	Flags       []string       `json:"flags,omitempty"`
	Practises   []string       `json:"practises,omitempty"`
	Dir         string         `json:"dir,omitempty"`
	Sections    []Section      `json:"sections"`
	Options     []Option       `json:"options,omitempty"`
	Hints       []string       `json:"hints,omitempty"`
	Checklist   []string       `json:"checklist,omitempty"`
	Files       *ExerciseFiles `json:"files,omitempty"`
	Command     string         `json:"command,omitempty"`
	Practice    string         `json:"practice,omitempty"`
	PractisedBy []string       `json:"practisedBy,omitempty"`
	line        int
}

type Lesson struct {
	ID        string  `json:"id"`
	Title     string  `json:"title"`
	Milestone string  `json:"milestone"`
	File      string  `json:"file"`
	Intro     []Block `json:"intro"`
	Concepts  []*Unit `json:"concepts"`
	Exercises []*Unit `json:"exercises"`
	lines     []string
}

func (u *Unit) section(name string) *Section {
	for i := range u.Sections {
		if u.Sections[i].Name == name {
			return &u.Sections[i]
		}
	}
	return nil
}

var (
	reConcept  = regexp.MustCompile(`^## concept ([a-z0-9.-]+): (.+)$`)
	reExercise = regexp.MustCompile(`^## exercise ([a-z0-9.-]+) (predict|choice|write|fix|explain): (.+)$`)
	reHeader   = regexp.MustCompile(`^(flags|practises|dir): (.+)$`)
	reOption   = regexp.MustCompile(`^- \[( |x)\] (.+)$`)
	reHint     = regexp.MustCompile(`^\d+\. (.+)$`)
	reCheck    = regexp.MustCompile(`^- (.+)$`)
	reExit     = regexp.MustCompile(`^exit status \d+$`)
	reTopicRow = regexp.MustCompile(`^\| ([a-z0-9]+\.[a-z0-9-]+) \|`)
)

func parseLesson(path, rel string) (*Lesson, []string) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, []string{err.Error()}
	}
	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	l := &Lesson{File: rel, lines: lines}
	var errs []string
	fail := func(n int, f string, a ...any) {
		errs = append(errs, fmt.Sprintf("%s:%d: %s", rel, n+1, fmt.Sprintf(f, a...)))
	}

	i := 0
	if len(lines) > 0 && lines[0] == "---" {
		for i = 1; i < len(lines) && lines[i] != "---"; i++ {
			k, v, ok := strings.Cut(lines[i], ": ")
			if !ok {
				fail(i, "bad front matter line %q", lines[i])
				continue
			}
			switch k {
			case "id":
				l.ID = v
			case "title":
				l.Title = v
			case "milestone":
				l.Milestone = v
			default:
				fail(i, "unknown front matter key %q", k)
			}
		}
		i++
	} else {
		fail(0, "missing front matter")
	}

	var unit *Unit
	var sec *Section
	target := func() *[]Block {
		if sec != nil {
			return &sec.Blocks
		}
		return &l.Intro
	}
	var text []string
	flushText := func() {
		t := strings.TrimSpace(strings.Join(text, "\n"))
		if t != "" {
			*target() = append(*target(), Block{K: "md", T: t})
		}
		text = nil
	}

	for ; i < len(lines); i++ {
		line := lines[i]
		switch {
		case strings.HasPrefix(line, "## "):
			flushText()
			sec = nil
			if m := reConcept.FindStringSubmatch(line); m != nil {
				unit = &Unit{Kind: "concept", ID: m[1], Title: m[2], line: i}
				l.Concepts = append(l.Concepts, unit)
			} else if m := reExercise.FindStringSubmatch(line); m != nil {
				unit = &Unit{Kind: "exercise", ID: m[1], Type: m[2], Title: m[3], line: i}
				l.Exercises = append(l.Exercises, unit)
			} else {
				fail(i, "heading must be '## concept <id>: <title>' or '## exercise <id> <type>: <title>'")
				unit = nil
				continue
			}
			for i+1 < len(lines) {
				m := reHeader.FindStringSubmatch(lines[i+1])
				if m == nil {
					break
				}
				i++
				parts := strings.Split(m[2], ",")
				for j := range parts {
					parts[j] = strings.TrimSpace(parts[j])
				}
				switch m[1] {
				case "flags":
					unit.Flags = parts
				case "practises":
					unit.Practises = parts
				case "dir":
					unit.Dir = m[2]
				}
			}
		case strings.HasPrefix(line, "### "):
			flushText()
			if unit == nil {
				fail(i, "section outside a concept or exercise")
				continue
			}
			unit.Sections = append(unit.Sections, Section{Name: strings.TrimPrefix(line, "### ")})
			sec = &unit.Sections[len(unit.Sections)-1]
		case strings.HasPrefix(line, "```"):
			flushText()
			fields := strings.Fields(strings.TrimPrefix(line, "```"))
			b := Block{K: "code", Run: "run"}
			if len(fields) > 0 {
				b.Lang = fields[0]
			}
			if b.Lang == "output" {
				b.K, b.Lang, b.Run = "out", "", ""
			}
			for _, f := range fields[min(1, len(fields)):] {
				switch {
				case f == "snippet":
					b.Snippet = true
				case strings.HasPrefix(f, "file="):
					b.File = strings.TrimPrefix(f, "file=")
				case f == "run=check" || f == "run=vet":
					b.Run = "check"
				default:
					fail(i, "unknown code block option %q", f)
				}
			}
			start := i + 1
			for i++; i < len(lines) && lines[i] != "```"; i++ {
			}
			if i >= len(lines) {
				fail(start-1, "code block is never closed")
				break
			}
			b.T = strings.Join(lines[start:i], "\n")
			b.from, b.to = start, i
			*target() = append(*target(), b)
		default:
			text = append(text, line)
		}
	}
	flushText()

	for _, u := range l.Exercises {
		if s := u.section("Options"); s != nil {
			for _, b := range s.Blocks {
				for _, ln := range strings.Split(b.T, "\n") {
					if m := reOption.FindStringSubmatch(ln); m != nil {
						u.Options = append(u.Options, Option{Text: m[2], Correct: m[1] == "x"})
					}
				}
			}
		}
		if s := u.section("Hints"); s != nil {
			for _, b := range s.Blocks {
				for _, ln := range strings.Split(b.T, "\n") {
					if m := reHint.FindStringSubmatch(ln); m != nil {
						u.Hints = append(u.Hints, m[1])
					} else if strings.TrimSpace(ln) != "" && len(u.Hints) > 0 {
						u.Hints[len(u.Hints)-1] += " " + strings.TrimSpace(ln)
					}
				}
			}
		}
		if s := u.section("Checklist"); s != nil {
			for _, b := range s.Blocks {
				for _, ln := range strings.Split(b.T, "\n") {
					if m := reCheck.FindStringSubmatch(ln); m != nil {
						u.Checklist = append(u.Checklist, m[1])
					}
				}
			}
		}
	}
	return l, errs
}

// ---------- running code ----------

// group is code blocks that run together, and the output block that must match.
type group struct {
	where string
	code  []Block
	out   *Block
	file  string // lesson file, for -update
}

func normalize(s, tmp string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	if tmp != "" {
		s = strings.ReplaceAll(s, tmp+string(os.PathSeparator), "")
		s = strings.ReplaceAll(s, filepath.ToSlash(tmp)+"/", "")
		s = strings.ReplaceAll(s, tmp, "")
	}
	var keep []string
	for _, ln := range strings.Split(s, "\n") {
		if strings.HasPrefix(ln, "# ") || reExit.MatchString(ln) {
			continue
		}
		if strings.Contains(ln, ".go:") || strings.Contains(ln, ".py\"") || strings.Contains(ln, ".rs:") {
			ln = strings.ReplaceAll(ln, `\`, "/")
		}
		keep = append(keep, strings.TrimRight(ln, " \t"))
	}
	return strings.Trim(strings.Join(keep, "\n"), "\n")
}

func runCmd(dir string, args []string) (string, error) {
	if len(args) == 0 {
		return "", errors.New("empty command")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = dir
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	err := cmd.Run()
	return buf.String(), err
}

func writeFiles(root string, files map[string]string, repl *strings.Replacer) error {
	for name, body := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if repl != nil {
			body = repl.Replace(body)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// runGroup runs one example in a fresh temp dir and returns what it printed.
func runGroup(g group, lang *Lang) (string, error) {
	tmp, err := os.MkdirTemp("", "lessoncheck-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	if err := writeFiles(tmp, lang.Setup, nil); err != nil {
		return "", err
	}
	run := "run"
	files := map[string]string{}
	for _, b := range g.code {
		name := b.File
		if name == "" {
			name = lang.File
			run = b.Run
		}
		files[name] = b.T + "\n"
	}
	if err := writeFiles(tmp, files, nil); err != nil {
		return "", err
	}
	args := lang.Run
	if run == "check" {
		if len(lang.Check) == 0 {
			return "", fmt.Errorf("a run=check block needs a 'check' command for %s in track.json", lang.Key)
		}
		args = lang.Check
	}
	out, _ := runCmd(tmp, args)
	return normalize(out, tmp), nil
}

// ---------- exercises ----------

func parseTxtar(path string) (map[string]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	files := map[string]string{}
	var cur string
	var buf []string
	flush := func() {
		if cur != "" {
			files[cur] = strings.TrimRight(strings.Join(buf, "\n"), "\n") + "\n"
		}
		buf = nil
	}
	for _, ln := range strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n") {
		if strings.HasPrefix(ln, "-- ") && strings.HasSuffix(ln, " --") && len(ln) > 6 {
			flush()
			cur = strings.TrimSuffix(strings.TrimPrefix(ln, "-- "), " --")
			continue
		}
		buf = append(buf, ln)
	}
	flush()
	return files, nil
}

// testIn builds a fresh copy of the learning module's layout, writes the files
// into practice/<dir>, and runs the track's test command. It reports whether
// the tests passed.
func testIn(tr *Track, pkgDir string, files map[string]string) (bool, string, error) {
	tmp, err := os.MkdirTemp("", "lessoncheck-ex-")
	if err != nil {
		return false, "", err
	}
	defer os.RemoveAll(tmp)
	repl := strings.NewReplacer("{module}", tr.Module)
	if err := writeFiles(tmp, tr.Exercise.Setup, repl); err != nil {
		return false, "", err
	}
	put := map[string]string{}
	for name, body := range files {
		put[pkgDir+"/"+name] = body
	}
	if err := writeFiles(tmp, put, nil); err != nil {
		return false, "", err
	}
	out, err := runCmd(tmp, testArgs(tr, pkgDir))
	return err == nil, normalize(out, tmp), nil
}

func testArgs(tr *Track, pkgDir string) []string {
	args := make([]string, len(tr.Exercise.Test))
	for i, a := range tr.Exercise.Test {
		args[i] = strings.ReplaceAll(a, "{dir}", pkgDir)
	}
	return args
}

// ---------- Markdown tables ----------

type Row map[string]string

// tableAfter parses the first Markdown table after a heading line.
func tableAfter(md, heading string) []Row {
	lines := strings.Split(strings.ReplaceAll(md, "\r\n", "\n"), "\n")
	var head []string
	var rows []Row
	inside := false
	for _, ln := range lines {
		if strings.TrimSpace(ln) == heading {
			inside = true
			continue
		}
		if !inside {
			continue
		}
		if strings.HasPrefix(ln, "#") {
			break
		}
		if !strings.HasPrefix(ln, "|") {
			if head != nil {
				break
			}
			continue
		}
		cells := strings.Split(strings.Trim(ln, "|"), "|")
		for i := range cells {
			cells[i] = strings.TrimSpace(cells[i])
		}
		if head == nil {
			head = cells
			continue
		}
		if strings.HasPrefix(cells[0], "---") {
			continue
		}
		r := Row{}
		for i, h := range head {
			if i < len(cells) {
				r[h] = cells[i]
			}
		}
		rows = append(rows, r)
	}
	return rows
}

// ---------- main ----------

// Brief is one build step's brief (R17): the thing that makes a build step a
// valid entry point, so a milestone can be started before its lessons are read.
//
// It lives in _builds/<build id>/ — the leading underscore keeps the Go tool out
// of a directory that holds test files belonging to another module.
type Brief struct {
	Build    string            `json:"build"`
	Dir      string            `json:"dir"`
	Markdown string            `json:"markdown"`
	Needs    []string          `json:"needs"`
	Tests    map[string]string `json:"tests"`
}

var needsLineRe = regexp.MustCompile(`(?m)needs:\s*(.+)$`)
var topicIDRe = regexp.MustCompile(`[a-z]+\.[a-z0-9-]+`)

// loadBriefs reads every build brief and checks it against the curriculum: the
// build must exist, every topic it points at must be real, and it must carry
// tests plus a Compare section. Without those a brief is a task with no
// definition of done and no way back to the explanation, which is the whole
// point of R17.
func loadBriefs(root string, cur Curriculum, p *problems) map[string]Brief {
	briefs := map[string]Brief{}

	dir := filepath.Join(root, "_builds")
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return briefs
	}
	if err != nil {
		p.add("_builds: %v", err)
		return briefs
	}

	buildIDs := map[string]bool{}
	for _, b := range cur.Builds {
		buildIDs[b.ID] = true
	}
	topicIDs := map[string]bool{}
	for _, t := range cur.Topics {
		topicIDs[t.ID] = true
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		id := e.Name()
		at := "_builds/" + id

		if !buildIDs[id] {
			p.add("%s: no build step with id %q in the curriculum", at, id)
			continue
		}

		mdPath := filepath.Join(dir, id, "README.md")
		md, err := os.ReadFile(mdPath)
		if err != nil {
			p.add("%s: %v", at+"/README.md", err)
			continue
		}
		body := string(md)

		if !strings.Contains(body, "## Compare") {
			p.add("%s: a brief needs a '## Compare' section — the reference to read once the tests pass", at)
		}
		if !strings.Contains(body, "## Done when") {
			p.add("%s: a brief needs a '## Done when' section — the tests are the only definition of done", at)
		}

		// Every 'needs:' tag has to name a topic that exists, or being stuck
		// leads nowhere.
		seen := map[string]bool{}
		var needs []string
		for _, m := range needsLineRe.FindAllStringSubmatch(body, -1) {
			for _, t := range topicIDRe.FindAllString(m[1], -1) {
				if !topicIDs[t] {
					p.add("%s: needs: %s — no such topic in the curriculum", at, t)
					continue
				}
				if !seen[t] {
					seen[t] = true
					needs = append(needs, t)
				}
			}
		}
		if len(needs) == 0 {
			p.add("%s: no 'needs: <topic-id>' tags — without them a stuck reader has nowhere to go (R17)", at)
		}
		sort.Strings(needs)

		tests := map[string]string{}
		files, err := os.ReadDir(filepath.Join(dir, id))
		if err != nil {
			p.add("%s: %v", at, err)
			continue
		}
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), "_test.go") {
				continue
			}
			t, err := os.ReadFile(filepath.Join(dir, id, f.Name()))
			if err != nil {
				p.add("%s/%s: %v", at, f.Name(), err)
				continue
			}
			tests[f.Name()] = string(t)
		}
		if len(tests) == 0 {
			p.add("%s: no *_test.go — a brief with no contract tests has no definition of done (R17)", at)
		}

		briefs[id] = Brief{Build: id, Dir: at, Markdown: body, Needs: needs, Tests: tests}
	}
	return briefs
}

type problems struct {
	mu   sync.Mutex
	list []string
}

func (p *problems) add(f string, a ...any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.list = append(p.list, fmt.Sprintf(f, a...))
}

func readStrict(path string, v any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	return nil
}

func main() {
	update := flag.Bool("update", false, "rewrite output blocks with what the code really prints")
	flag.Parse()

	root, err := os.Getwd()
	if err != nil {
		fatal(err)
	}
	var tr Track
	if err := readStrict(filepath.Join(root, "track.json"), &tr); err != nil {
		fatal(fmt.Errorf("run this from the learning folder; track.json: %w", err))
	}
	if tr.BonusCap == 0 {
		tr.BonusCap = 2
	}
	if tr.Plan == "" {
		tr.Plan = "plan.md"
	}
	langs := map[string]*Lang{tr.Language.Key: &tr.Language}
	if tr.Contrast != nil {
		langs[tr.Contrast.Key] = tr.Contrast
	}
	var product string
	if tr.Product != nil {
		product = filepath.FromSlash(tr.Product.Repo)
		if !filepath.IsAbs(product) {
			product = filepath.Join(root, product)
		}
		product, err = filepath.Abs(product)
		if err != nil {
			fatal(err)
		}
		if _, err := os.Stat(product); err != nil {
			fatal(fmt.Errorf("product repo not found at %s (track.json product.repo)", product))
		}
	}
	var p problems

	// Curriculum.
	var cur Curriculum
	if err := readStrict(filepath.Join(root, "board", "curriculum.json"), &cur); err != nil {
		fatal(err)
	}
	trackKeys := map[string]bool{}
	for _, t := range cur.Tracks {
		trackKeys[t.Key] = true
	}
	if len(trackKeys) == 0 {
		p.add("curriculum: 'tracks' must list at least one track")
	}
	msOrder := map[string]int{}
	msCode := map[string]string{}
	for _, m := range cur.Milestones {
		msOrder[m.ID] = m.Order
		msCode[m.ID] = m.Code
	}
	ids := map[string]string{}
	seen := func(id, what string) {
		if prev, dup := ids[id]; dup {
			p.add("curriculum: id %q is used by both a %s and a %s", id, prev, what)
		}
		ids[id] = what
	}
	for _, m := range cur.Milestones {
		seen(m.ID, "milestone")
	}
	builds := map[string]*Build{}
	for i := range cur.Builds {
		b := &cur.Builds[i]
		seen(b.ID, "build step")
		builds[b.ID] = b
		if _, ok := msOrder[b.Milestone]; !ok {
			p.add("curriculum: build %s: unknown milestone %q", b.ID, b.Milestone)
		}
		if !oneOf(b.Owner, "you", "claude", "both") {
			p.add("curriculum: build %s: owner must be you, claude or both", b.ID)
		}
		if !oneOf(b.State, "planned", "in-progress", "done") {
			p.add("curriculum: build %s: state must be planned, in-progress or done", b.ID)
		}
	}
	for _, c := range cur.Capabilities {
		seen(c.ID, "capability")
		if _, ok := msOrder[c.Milestone]; !ok {
			p.add("curriculum: capability %s: unknown milestone %q", c.ID, c.Milestone)
		}
		if !oneOf(c.State, "planned", "in-progress", "working") {
			p.add("curriculum: capability %s: state must be planned, in-progress or working", c.ID)
		}
	}
	topics := map[string]*Topic{}
	bonusPer := map[string]int{}
	for i := range cur.Topics {
		t := &cur.Topics[i]
		seen(t.ID, "topic")
		topics[t.ID] = t
		if !trackKeys[t.Track] {
			p.add("curriculum: topic %s: track %q is not in 'tracks'", t.ID, t.Track)
		}
		switch t.Kind {
		case "core", "bonus":
			if t.Milestone == nil {
				p.add("curriculum: topic %s: a %s topic needs a milestone", t.ID, t.Kind)
			} else if _, ok := msOrder[*t.Milestone]; !ok {
				p.add("curriculum: topic %s: unknown milestone %q", t.ID, *t.Milestone)
			}
			if t.Kind == "core" {
				b := builds[deref(t.UsedBy)]
				switch {
				case t.UsedBy == nil || b == nil:
					p.add("curriculum: core topic %s must name the build step that uses it (usedBy)", t.ID)
				case t.Milestone != nil && msOrder[b.Milestone] < msOrder[*t.Milestone]:
					p.add("curriculum: topic %s is taught in %s but used earlier, by %s", t.ID, *t.Milestone, b.ID)
				}
			} else {
				if t.UsedBy != nil {
					p.add("curriculum: bonus topic %s must not have usedBy", t.ID)
				}
				if t.Milestone != nil {
					bonusPer[*t.Milestone]++
				}
			}
		case "not-planned":
			if t.Reason == "" || t.Milestone != nil || t.UsedBy != nil {
				p.add("curriculum: not-planned topic %s needs a reason and no milestone or usedBy", t.ID)
			}
		default:
			p.add("curriculum: topic %s: kind must be core, bonus or not-planned", t.ID)
		}
		if !oneOf(t.Depth, "core", "interview", "advanced") {
			p.add("curriculum: topic %s: depth must be core, interview or advanced", t.ID)
		}
	}
	for m, n := range bonusPer {
		if n > tr.BonusCap {
			p.add("curriculum: milestone %s has %d bonus topics; the cap is %d", m, n, tr.BonusCap)
		}
	}

	// Lessons.
	var lessons []*Lesson
	files, _ := filepath.Glob(filepath.Join(root, "lessons", "*", "*.md"))
	sort.Strings(files)
	lessonIDs := map[string]bool{}
	for _, f := range files {
		rel := filepath.ToSlash(strings.TrimPrefix(f, root+string(os.PathSeparator)))
		l, errs := parseLesson(f, rel)
		for _, e := range errs {
			p.add("%s", e)
		}
		if l == nil {
			continue
		}
		if lessonIDs[l.ID] {
			p.add("%s: duplicate lesson id %q", rel, l.ID)
		}
		lessonIDs[l.ID] = true
		if _, ok := msOrder[l.Milestone]; !ok {
			p.add("%s: unknown milestone %q", rel, l.Milestone)
		}
		lessons = append(lessons, l)
	}

	contrastSection := ""
	if tr.Contrast != nil {
		contrastSection = "In " + tr.Contrast.Name
	}
	taught := map[string]string{} // topic -> lesson
	for _, l := range lessons {
		if n := len(l.Concepts); n < 3 || n > 8 {
			p.add("%s: a lesson has 3–8 concepts; this one has %d", l.File, n)
		}
		for _, c := range l.Concepts {
			at := fmt.Sprintf("%s:%d: concept %s", l.File, c.line+1, c.ID)
			t := topics[c.ID]
			if t == nil {
				p.add("%s: not a topic in curriculum.json", at)
				continue
			}
			if prev, dup := taught[c.ID]; dup {
				p.add("%s: already taught in %s", at, prev)
			}
			taught[c.ID] = l.ID
			t.Lesson = l.ID
			if t.Milestone == nil || *t.Milestone != l.Milestone {
				p.add("%s: the curriculum puts this topic in %s, not %s", at, deref(t.Milestone), l.Milestone)
			}
			if len(c.Flags) != 2 || !oneOf(c.Flags[0], "core", "interview", "advanced") || !oneOf(c.Flags[1], "transfers", "differs", "new") {
				p.add("%s: flags must be '<core|interview|advanced>, <transfers|differs|new>'", at)
			}
			allowed := map[string]bool{"Example": true, "Explain": true, "More": true, "Takeaway": true}
			if contrastSection != "" {
				allowed[contrastSection] = true
			}
			for _, s := range c.Sections {
				if !allowed[s.Name] {
					p.add("%s: unknown section %q", at, s.Name)
				}
				// The comparison section is collapsed on the board, so nothing
				// about the language being learned may live only there.
				if contrastSection != "" && s.Name == contrastSection {
					for _, b := range s.Blocks {
						if b.K == "code" && b.Lang == tr.Language.Key {
							p.add("%s: %s code in the %q section is hidden when it's collapsed; move it to More", at, tr.Language.Name, contrastSection)
						}
					}
				}
			}
			need := []string{"Example", "Explain", "Takeaway"}
			if contrastSection != "" && t.Track == tr.Language.Key {
				need = append(need, contrastSection)
			}
			for _, n := range need {
				if c.section(n) == nil {
					p.add("%s: missing section %q", at, n)
				}
			}
			if s := c.section("Explain"); s != nil {
				words := 0
				for _, b := range s.Blocks {
					if b.K != "md" {
						p.add("%s: Explain must be prose only", at)
					}
					words += len(strings.Fields(b.T))
				}
				if words > 80 {
					p.add("%s: Explain is %d words; the format allows 80", at, words)
				}
			}
			if s := c.section("Takeaway"); s != nil && (len(s.Blocks) != 1 || strings.Contains(s.Blocks[0].T, "\n")) {
				p.add("%s: Takeaway must be a single line", at)
			}
			if s := c.section("Example"); s != nil {
				hasRun := false
				for _, b := range s.Blocks {
					if b.K == "out" {
						hasRun = true
					}
				}
				if !hasRun {
					p.add("%s: Example needs code with its output", at)
				}
			}
		}
	}

	// Exercises: shape, and which concepts they practise.
	practisedBy := map[string][]string{}
	var allExercises []*Unit
	for _, l := range lessons {
		auto := 0
		for _, e := range l.Exercises {
			allExercises = append(allExercises, e)
			at := fmt.Sprintf("%s:%d: exercise %s", l.File, e.line+1, e.ID)
			seen(e.ID, "exercise")
			if len(e.Practises) == 0 {
				p.add("%s: must name the concepts it practises", at)
			}
			for _, tp := range e.Practises {
				practisedBy[tp] = append(practisedBy[tp], e.ID)
			}
			has := func(names ...string) {
				for _, n := range names {
					if e.section(n) == nil {
						p.add("%s: missing section %q", at, n)
					}
				}
			}
			switch e.Type {
			case "predict", "choice":
				auto++
				has("Prompt", "Answer", "Why")
				if e.Type == "choice" {
					correct := 0
					for _, o := range e.Options {
						if o.Correct {
							correct++
						}
					}
					if len(e.Options) < 2 || correct != 1 {
						p.add("%s: a choice needs at least two options and exactly one [x]", at)
					}
				}
			case "write", "fix":
				has("Task", "Hints", "Why")
				if e.Type == "write" && len(e.Hints) != 3 {
					p.add("%s: a write exercise has exactly three hints (nudge, approach, nearly the answer); found %d", at, len(e.Hints))
				}
				if e.Type == "fix" && len(e.Hints) == 0 {
					p.add("%s: a fix exercise needs hints", at)
				}
				if e.Dir == "" {
					p.add("%s: needs 'dir:' (its folder under practice/)", at)
				}
			case "explain":
				has("Task", "Checklist")
				if len(e.Checklist) < 2 {
					p.add("%s: an explain exercise needs a checklist of at least two points", at)
				}
			}
		}
		if auto < 2 {
			p.add("%s: needs at least two auto-checked exercises (predict or choice) so the lesson can be tested out of; has %d", l.File, auto)
		}
	}
	for _, e := range allExercises {
		for _, tp := range e.Practises {
			if _, ok := taught[tp]; !ok {
				p.add("exercise %s practises %s, which no lesson teaches", e.ID, tp)
			}
		}
	}
	for _, l := range lessons {
		for _, c := range l.Concepts {
			c.PractisedBy = practisedBy[c.ID]
			if len(c.PractisedBy) == 0 {
				p.add("%s: concept %s is not practised by any exercise", l.File, c.ID)
			}
		}
	}
	for _, b := range cur.Builds {
		if b.Exercise != nil {
			found := false
			for _, e := range allExercises {
				found = found || e.ID == *b.Exercise
			}
			if !found {
				p.add("curriculum: build %s points to exercise %s, which doesn't exist", b.ID, *b.Exercise)
			}
		}
	}
	written := map[string]bool{}
	for _, l := range lessons {
		written[l.Milestone] = true
	}
	for _, t := range cur.Topics {
		if t.Kind == "core" && t.Milestone != nil && written[*t.Milestone] && t.Lesson == "" {
			p.add("curriculum: %s has lessons, but core topic %s isn't taught in any of them", *t.Milestone, t.ID)
		}
	}

	// Collect every example to run.
	var groups []group
	for _, l := range lessons {
		collect := func(where string, blocks []Block) {
			var pending []Block
			for i := range blocks {
				b := &blocks[i]
				switch {
				case b.K == "code" && b.Snippet:
				case b.K == "code":
					if len(pending) > 0 && pending[0].Lang != b.Lang {
						p.add("%s: a %s block and a %s block are grouped together", where, pending[0].Lang, b.Lang)
					}
					pending = append(pending, *b)
				case b.K == "out":
					if len(pending) == 0 {
						p.add("%s: output block with no code before it", where)
						continue
					}
					groups = append(groups, group{where: where, code: pending, out: b, file: l.File})
					pending = nil
				default:
					if len(pending) > 0 {
						p.add("%s: code block is shown but never run; add an output block after it or mark it 'snippet'", where)
						pending = nil
					}
				}
			}
			if len(pending) > 0 {
				p.add("%s: code block is shown but never run; add an output block after it or mark it 'snippet'", where)
			}
		}
		for _, c := range l.Concepts {
			for si := range c.Sections {
				collect(fmt.Sprintf("%s › %s › %s", l.File, c.ID, c.Sections[si].Name), c.Sections[si].Blocks)
			}
		}
		for _, e := range l.Exercises {
			if e.Type == "predict" || e.Type == "choice" {
				// The prompt's code and the answer's output are one group: the
				// answer stays hidden on the board until the learner commits.
				prompt, answer := e.section("Prompt"), e.section("Answer")
				if prompt == nil || answer == nil {
					continue
				}
				var code []Block
				for _, b := range prompt.Blocks {
					if b.K == "code" && !b.Snippet {
						code = append(code, b)
					}
				}
				var out *Block
				for i := range answer.Blocks {
					if answer.Blocks[i].K == "out" {
						out = &answer.Blocks[i]
					}
				}
				if len(code) == 0 || out == nil {
					p.add("%s › %s: needs code in Prompt and an output block in Answer", l.File, e.ID)
					continue
				}
				groups = append(groups, group{where: fmt.Sprintf("%s › %s", l.File, e.ID), code: code, out: out, file: l.File})
				continue
			}
			for si := range e.Sections {
				collect(fmt.Sprintf("%s › %s › %s", l.File, e.ID, e.Sections[si].Name), e.Sections[si].Blocks)
			}
		}
	}

	// Every group must be in a language track.json knows how to run; Go
	// examples must be gofmt-formatted when the track asks for it.
	for _, g := range groups {
		lang := langs[g.code[0].Lang]
		if lang == nil {
			p.add("%s: no way to run %q code; add it to track.json (language or contrast) or mark the block 'snippet'", g.where, g.code[0].Lang)
			continue
		}
		if !lang.Gofmt {
			continue
		}
		for _, b := range g.code {
			formatted, err := format.Source([]byte(b.T + "\n"))
			if err != nil {
				p.add("%s: example doesn't parse as Go: %v", g.where, err)
			} else if string(formatted) != b.T+"\n" {
				p.add("%s: example isn't gofmt-formatted", g.where)
			}
		}
	}

	// Run every example, four at a time.
	type result struct {
		got string
		err error
	}
	results := make([]result, len(groups))
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for i := range groups {
		lang := langs[groups[i].code[0].Lang]
		if lang == nil {
			continue
		}
		wg.Add(1)
		go func(i int, lang *Lang) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			got, err := runGroup(groups[i], lang)
			results[i] = result{got, err}
		}(i, lang)
	}
	wg.Wait()
	type edit struct {
		from, to int
		text     string
	}
	changed := map[string][]edit{}
	primaryRuns, contrastRuns := 0, 0
	for i, g := range groups {
		if langs[g.code[0].Lang] == nil {
			continue
		}
		r := results[i]
		if r.err != nil {
			p.add("%s: could not run: %v", g.where, r.err)
			continue
		}
		if g.code[0].Lang == tr.Language.Key {
			primaryRuns++
		} else {
			contrastRuns++
		}
		want := strings.Trim(strings.ReplaceAll(g.out.T, "\r\n", "\n"), "\n")
		if r.got == want {
			continue
		}
		if *update {
			changed[g.file] = append(changed[g.file], edit{g.out.from, g.out.to, r.got})
			continue
		}
		p.add("%s: shown output does not match a real run\n    shown:  %q\n    actual: %q", g.where, want, r.got)
	}
	if *update {
		n := 0
		for _, l := range lessons {
			edits := changed[l.File]
			if len(edits) == 0 {
				continue
			}
			sort.Slice(edits, func(a, b int) bool { return edits[a].from > edits[b].from })
			lines := append([]string(nil), l.lines...)
			for _, ed := range edits {
				var repl []string
				if ed.text != "" {
					repl = strings.Split(ed.text, "\n")
				}
				lines = append(lines[:ed.from], append(repl, lines[ed.to:]...)...)
				n++
			}
			if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(l.File)), []byte(strings.Join(lines, "\n")), 0o644); err != nil {
				fatal(err)
			}
		}
		fmt.Printf("updated %d output block(s) from real runs; re-read the explanations next to them, then run again without -update\n", n)
		return
	}

	// Exercises: the starter must fail, the reference must pass, and the
	// learner's practice copy keeps the original tests.
	startersFail, refsPass, practicePass, practiceTotal := 0, 0, 0, 0
	for _, l := range lessons {
		for _, e := range l.Exercises {
			if e.Type != "write" && e.Type != "fix" {
				continue
			}
			bundle := filepath.Join(root, "lessons", l.Milestone, "exercises", e.ID+".txtar")
			fs, err := parseTxtar(bundle)
			if err != nil {
				p.add("exercise %s: %v", e.ID, err)
				continue
			}
			ex := &ExerciseFiles{Starter: map[string]string{}, Solution: map[string]string{}, Tests: map[string]string{}}
			for name, body := range fs {
				switch {
				case strings.HasPrefix(name, "starter/"):
					ex.Starter[strings.TrimPrefix(name, "starter/")] = body
				case strings.HasPrefix(name, "solution/"):
					ex.Solution[strings.TrimPrefix(name, "solution/")] = body
				default:
					ex.Tests[name] = body
				}
				if tr.Language.Gofmt && strings.HasSuffix(name, ".go") {
					if formatted, err := format.Source([]byte(body)); err != nil || string(formatted) != body {
						p.add("exercise %s: %s isn't gofmt-formatted Go", e.ID, name)
					}
				}
			}
			if len(ex.Starter) == 0 || len(ex.Solution) == 0 || len(ex.Tests) == 0 {
				p.add("exercise %s: the bundle needs starter/, solution/ and test files", e.ID)
				continue
			}
			e.Files = ex
			pkgDir := "practice/" + e.Dir
			e.Practice = pkgDir
			e.Command = strings.Join(testArgs(&tr, pkgDir), " ")
			withTests := func(code map[string]string) map[string]string {
				m := map[string]string{}
				for k, v := range code {
					m[k] = v
				}
				for k, v := range ex.Tests {
					m[k] = v
				}
				return m
			}
			if ok, _, err := testIn(&tr, pkgDir, withTests(ex.Starter)); err != nil {
				p.add("exercise %s: %v", e.ID, err)
			} else if ok {
				p.add("exercise %s: the starter already passes its tests, so there's nothing to do", e.ID)
			} else {
				startersFail++
			}
			if ok, out, err := testIn(&tr, pkgDir, withTests(ex.Solution)); err != nil {
				p.add("exercise %s: %v", e.ID, err)
			} else if !ok {
				p.add("exercise %s: the reference solution fails its tests:\n%s", e.ID, out)
			} else {
				refsPass++
			}
			// The learner's workspace: created from the starter if missing,
			// never overwritten. Its tests must stay the originals.
			ws := filepath.Join(root, filepath.FromSlash(pkgDir))
			if _, err := os.Stat(ws); errors.Is(err, os.ErrNotExist) {
				if err := writeFiles(ws, withTests(ex.Starter), nil); err != nil {
					fatal(err)
				}
				fmt.Printf("created %s from the starter\n", pkgDir)
			}
			for name, body := range ex.Tests {
				got, err := os.ReadFile(filepath.Join(ws, name))
				if err != nil || strings.ReplaceAll(string(got), "\r\n", "\n") != body {
					p.add("exercise %s: %s/%s differs from the original test; restore it from %s", e.ID, pkgDir, name, filepath.ToSlash(strings.TrimPrefix(bundle, root+string(os.PathSeparator))))
				}
			}
			practiceTotal++
			if _, err := runCmd(root, testArgs(&tr, pkgDir)); err == nil {
				practicePass++
			}
		}
	}

	// The plan's topic tables must agree with the curriculum.
	plan, err := os.ReadFile(filepath.Join(root, tr.Plan))
	if err != nil {
		p.add("%s: %v", tr.Plan, err)
	}
	inPlan := map[string]bool{}
	for _, ln := range strings.Split(strings.ReplaceAll(string(plan), "\r\n", "\n"), "\n") {
		m := reTopicRow.FindStringSubmatch(ln)
		if m == nil || topics[m[1]] == nil {
			if m != nil && strings.Count(ln, "|") >= 6 {
				p.add("%s: topic %s is not in curriculum.json", tr.Plan, m[1])
			}
			continue
		}
		cells := strings.Split(ln, "|")
		if len(cells) < 7 {
			continue
		}
		id, ms, kind := m[1], strings.TrimSpace(cells[3]), strings.TrimSpace(cells[5])
		inPlan[id] = true
		t := topics[id]
		wantMs := "—"
		if t.Milestone != nil {
			wantMs = msCode[*t.Milestone]
		}
		planKind := kind
		if strings.HasPrefix(kind, "not planned") {
			planKind = "not-planned"
		}
		if ms != wantMs || planKind != t.Kind {
			p.add("%s: topic %s says %s/%s, curriculum.json says %s/%s", tr.Plan, id, ms, planKind, wantMs, t.Kind)
		}
	}
	for id := range topics {
		if !inPlan[id] {
			p.add("%s: topic %s is missing from the tables", tr.Plan, id)
		}
	}

	// Requirements (learning here, product in its repo) and the log.
	reqMD, err := os.ReadFile(filepath.Join(root, "requirements.md"))
	if err != nil {
		p.add("requirements.md: %v", err)
	}
	var productMD []byte
	docs := []struct {
		source string
		md     string
	}{{"learning", string(reqMD)}}
	if tr.Product != nil && tr.Product.Requirements != "" {
		productMD, err = os.ReadFile(filepath.Join(product, filepath.FromSlash(tr.Product.Requirements)))
		if err != nil {
			p.add("product requirements: %v", err)
		}
		docs = append([]struct {
			source string
			md     string
		}{{"product", string(productMD)}}, docs...)
	}
	var open, decided []Row
	for _, doc := range docs {
		d := tableAfter(doc.md, "### Decided")
		if len(d) == 0 {
			p.add("%s requirements: couldn't read the Decided table", doc.source)
		}
		for _, r := range d {
			r["source"] = doc.source
			decided = append(decided, r)
		}
		for _, r := range tableAfter(doc.md, "### Still open") {
			if r["id"] == "—" || r["id"] == "" {
				continue
			}
			r["source"] = doc.source
			open = append(open, r)
		}
	}
	// The product document lists the capabilities; they must match the board's.
	if tr.Product != nil && tr.Product.CapabilitiesHeading != "" {
		caps := tableAfter(string(productMD), tr.Product.CapabilitiesHeading)
		if len(caps) != len(cur.Capabilities) {
			p.add("product requirements: %d capabilities listed, curriculum.json has %d", len(caps), len(cur.Capabilities))
		}
		for _, c := range cur.Capabilities {
			found := false
			for _, r := range caps {
				if r["id"] != c.ID {
					continue
				}
				found = true
				if tr.Product.PilotColumn != "" {
					pilot := strings.HasPrefix(strings.ToLower(r[tr.Product.PilotColumn]), "yes")
					if pilot != c.Pilot {
						p.add("product requirements: %s is pilot=%v there but pilot=%v in curriculum.json", c.ID, pilot, c.Pilot)
					}
				}
			}
			if !found {
				p.add("product requirements: capability %s is missing from its table", c.ID)
			}
		}
	}
	logMD, err := os.ReadFile(filepath.Join(root, "log.md"))
	if err != nil {
		p.add("log.md: %v", err)
	}

	// Protected product paths untouched since the base commit.
	protected := "n/a"
	if tr.Product != nil && len(tr.Product.ProtectedPaths) > 0 {
		top, err := runCmd(product, []string{"git", "rev-parse", "--show-toplevel"})
		if err != nil {
			p.add("git in %s: %v", product, err)
		} else {
			repo := strings.TrimSpace(top)
			out, err := runCmd(repo, append([]string{"git", "diff", "--name-only", tr.Product.BaseCommit, "--"}, tr.Product.ProtectedPaths...))
			if err != nil {
				p.add("git diff: %v: %s", err, out)
			}
			st, err := runCmd(repo, append([]string{"git", "status", "--porcelain", "--"}, tr.Product.ProtectedPaths...))
			if err != nil {
				p.add("git status: %v: %s", err, st)
			}
			for _, ln := range strings.Split(strings.TrimSpace(out+"\n"+st), "\n") {
				if strings.TrimSpace(ln) != "" {
					p.add("protected product file changed: %s", strings.TrimSpace(ln))
				}
			}
			protected = "yes"
		}
	}

	briefs := loadBriefs(root, cur, &p)

	if len(p.list) > 0 {
		sort.Strings(p.list)
		fmt.Fprintf(os.Stderr, "lessoncheck: %d problem(s)\n", len(p.list))
		for _, e := range p.list {
			fmt.Fprintln(os.Stderr, "  "+e)
		}
		os.Exit(1)
	}

	// Everything holds: write what the board displays.
	type langView struct {
		Key, Name, File, RunLabel, CheckLabel, Hljs string
	}
	view := func(l *Lang) *langView {
		if l == nil {
			return nil
		}
		return &langView{Key: l.Key, Name: l.Name, File: l.File, RunLabel: l.RunLabel, CheckLabel: strings.Join(l.Check, " "), Hljs: l.Hljs}
	}
	content := map[string]any{
		"track": map[string]any{
			"title": tr.Title, "eyebrow": tr.Eyebrow, "lede": tr.Lede, "route": tr.Route,
			"language": view(&tr.Language), "contrast": view(tr.Contrast),
		},
		"curriculum": cur,
		"lessons":    lessons,
		"briefs":     briefs,
		"docs":       map[string]string{"requirements": string(reqMD), "productRequirements": string(productMD), "log": string(logMD)},
		"open":       open,
		"decided":    decided,
		"workdir":    root,
	}
	body, err := json.Marshal(content)
	if err != nil {
		fatal(err)
	}
	sum := sha256.Sum256(body)
	content["version"] = fmt.Sprintf("%x", sum[:6])
	body, err = json.MarshalIndent(content, "", " ")
	if err != nil {
		fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "board", "content.json"), append(body, '\n'), 0o644); err != nil {
		fatal(err)
	}

	concepts, exercises := 0, 0
	for _, l := range lessons {
		concepts += len(l.Concepts)
		exercises += len(l.Exercises)
	}
	fmt.Printf("lessoncheck ok: lessons=%d concepts=%d exercises=%d examples-verified=%d contrast-examples-verified=%d starters-fail=%d references-pass=%d practice-passing=%d/%d topics=%d protected-untouched=%s version=%s\n",
		len(lessons), concepts, exercises, primaryRuns, contrastRuns, startersFail, refsPass, practicePass, practiceTotal, len(cur.Topics), protected, content["version"])
}

func oneOf(s string, opts ...string) bool {
	for _, o := range opts {
		if s == o {
			return true
		}
	}
	return false
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "lessoncheck:", err)
	os.Exit(2)
}
