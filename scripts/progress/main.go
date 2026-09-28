// Command progress turns the board's saved data into files in this folder, so
// the owner's notes and progress exist on this machine and not only on
// claude.ai (requirements R15, P5).
//
// Claude downloads the board's three collections (concepts, exercises,
// lessons) as one JSON file per document, then runs, from ~/go-learning:
//
//	go run ./tools/progress -raw <download dir>
//
// It writes progress/progress.json (everything, latest), a dated copy in
// progress/snapshots/, and progress/notes.md (the notes, readable, grouped by
// lesson, with a summary of what's learned).
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// content is the part of board/content.json this tool needs for titles.
type content struct {
	Curriculum struct {
		Milestones []struct {
			ID, Code, Title string
			Order           int
		} `json:"milestones"`
	} `json:"curriculum"`
	Lessons []struct {
		ID, Title, Milestone string
		Concepts             []struct{ ID, Title string } `json:"concepts"`
		Exercises            []struct {
			ID, Title, Type string
		} `json:"exercises"`
	} `json:"lessons"`
}

type saved struct {
	CopiedAt  string                    `json:"copiedAt"`
	Concepts  map[string]map[string]any `json:"concepts"`
	Exercises map[string]map[string]any `json:"exercises"`
	Lessons   map[string]map[string]any `json:"lessons"`
}

func main() {
	raw := flag.String("raw", "", "folder holding the downloaded collections (concepts/, exercises/, lessons/)")
	date := flag.String("date", time.Now().Format("2006-01-02"), "date for the snapshot file")
	flag.Parse()
	if *raw == "" {
		fail(errors.New("-raw is required"))
	}

	var c content
	b, err := os.ReadFile(filepath.Join("board", "content.json"))
	if err != nil {
		fail(fmt.Errorf("run this from the learning folder: %w", err))
	}
	if err := json.Unmarshal(b, &c); err != nil {
		fail(err)
	}

	s := saved{CopiedAt: *date}
	s.Concepts, err = load(*raw, "concepts")
	if err != nil {
		fail(err)
	}
	s.Exercises, err = load(*raw, "exercises")
	if err != nil {
		fail(err)
	}
	s.Lessons, err = load(*raw, "lessons")
	if err != nil {
		fail(err)
	}

	out, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		fail(err)
	}
	out = append(out, '\n')
	if err := os.MkdirAll(filepath.Join("progress", "snapshots"), 0o755); err != nil {
		fail(err)
	}
	for _, p := range []string{filepath.Join("progress", "progress.json"), filepath.Join("progress", "snapshots", *date+".json")} {
		if err := os.WriteFile(p, out, 0o644); err != nil {
			fail(err)
		}
	}
	notes := render(c, s)
	if err := os.WriteFile(filepath.Join("progress", "notes.md"), []byte(notes), 0o644); err != nil {
		fail(err)
	}
	fmt.Printf("progress copied: concepts=%d exercises=%d lessons=%d notes=%d snapshot=progress/snapshots/%s.json\n",
		len(s.Concepts), len(s.Exercises), len(s.Lessons), countNotes(s), *date)
}

// load reads <raw>/<collection>/<id>.json. A file may hold the document
// itself or a wrapper with the document under "data"; both are accepted.
func load(raw, collection string) (map[string]map[string]any, error) {
	docs := map[string]map[string]any{}
	files, _ := filepath.Glob(filepath.Join(raw, collection, "*.json"))
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		var m map[string]any
		if err := json.Unmarshal(b, &m); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		if inner, ok := m["data"].(map[string]any); ok {
			m = inner
		}
		docs[strings.TrimSuffix(filepath.Base(f), ".json")] = m
	}
	return docs, nil
}

func note(m map[string]any) string {
	if m == nil {
		return ""
	}
	n, _ := m["note"].(string)
	return strings.TrimSpace(n)
}

func countNotes(s saved) int {
	n := 0
	for _, d := range s.Concepts {
		if note(d) != "" {
			n++
		}
	}
	for _, d := range s.Exercises {
		if note(d) != "" {
			n++
		}
	}
	return n
}

func render(c content, s saved) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Your notes\n\nCopied from the board on %s. The board is where you write them; this file is the copy on this machine.\n\n", s.CopiedAt)

	understood, passed := 0, 0
	for _, d := range s.Concepts {
		if v, _ := d["understood"].(bool); v {
			understood++
		}
	}
	for _, d := range s.Exercises {
		if d["state"] == "passed" {
			passed++
		}
	}
	fmt.Fprintf(&b, "**So far:** %d concepts marked understood, %d exercises done.\n", understood, passed)

	msOrder := map[string]int{}
	msName := map[string]string{}
	for _, m := range c.Curriculum.Milestones {
		msOrder[m.ID] = m.Order
		msName[m.ID] = m.Code + " · " + m.Title
	}
	lessons := c.Lessons
	sort.SliceStable(lessons, func(i, j int) bool { return msOrder[lessons[i].Milestone] < msOrder[lessons[j].Milestone] })

	wrote := false
	for _, l := range lessons {
		var items []string
		for _, cc := range l.Concepts {
			if n := note(s.Concepts[cc.ID]); n != "" {
				items = append(items, fmt.Sprintf("### %s\n\n%s\n", cc.Title, n))
			}
		}
		for _, e := range l.Exercises {
			if n := note(s.Exercises[e.ID]); n != "" {
				items = append(items, fmt.Sprintf("### Exercise: %s\n\n%s\n", e.Title, n))
			}
		}
		if len(items) == 0 {
			continue
		}
		wrote = true
		fmt.Fprintf(&b, "\n## %s — %s\n\n%s", msName[l.Milestone], l.Title, strings.Join(items, "\n"))
	}
	if !wrote {
		b.WriteString("\nNo notes yet.\n")
	}
	return b.String()
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "progress:", err)
	os.Exit(1)
}
