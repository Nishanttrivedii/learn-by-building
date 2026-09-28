// specdump turns an OpenAPI (3.x) YAML spec into readable Markdown, so an API
// that sits behind a login or a browser session becomes a local, greppable
// reference. It writes:
// operations.md (every endpoint: headers, params, body/response schema refs,
// and its HTML description flattened to text), schemas.md (every component
// schema as a field tree, in spec order), and examples.md (every example
// payload as JSON under its own heading). Everything it prints comes from the
// spec itself.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

func get(n *yaml.Node, key string) *yaml.Node {
	if n == nil {
		return nil
	}
	if n.Kind == yaml.DocumentNode && len(n.Content) > 0 {
		n = n.Content[0]
	}
	if n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i+1]
		}
	}
	return nil
}

type kv struct {
	k string
	v *yaml.Node
}

func pairs(n *yaml.Node) []kv {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	out := make([]kv, 0, len(n.Content)/2)
	for i := 0; i+1 < len(n.Content); i += 2 {
		out = append(out, kv{n.Content[i].Value, n.Content[i+1]})
	}
	return out
}

func str(n *yaml.Node) string {
	if n == nil {
		return ""
	}
	return n.Value
}

var (
	reTag   = regexp.MustCompile(`(?s)<[^>]+>`)
	reSpace = regexp.MustCompile(`[ \t]+`)
	reBlank = regexp.MustCompile(`\n{3,}`)
)

// flatten turns an HTML description into plain text, keeping table rows as
// "a | b | c" lines so field tables stay readable.
func flatten(s string) string {
	r := strings.NewReplacer(
		"</tr>", "\n", "</td>", " | ", "</th>", " | ", "<br/>", "\n", "<br>", "\n", "</br>", "\n",
		"</li>", "\n", "<li>", "- ", "</h2>", "\n", "</h3>", "\n", "</h4>", "\n", "</p>", "\n",
		"&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`,
	)
	s = r.Replace(s)
	s = reTag.ReplaceAllString(s, "")
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		l = reSpace.ReplaceAllString(strings.TrimSpace(l), " ")
		l = strings.TrimSuffix(l, " |")
		lines[i] = l
	}
	s = strings.Join(lines, "\n")
	s = reBlank.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}

func oneLine(s string, max int) string {
	s = strings.Join(strings.Fields(flatten(s)), " ")
	if max > 0 && len(s) > max {
		s = s[:max] + "…"
	}
	return s
}

func refName(n *yaml.Node) string {
	if r := get(n, "$ref"); r != nil {
		return strings.TrimPrefix(r.Value, "#/components/schemas/")
	}
	return ""
}

func typeOf(n *yaml.Node) string {
	if rn := refName(n); rn != "" {
		return "→ " + rn
	}
	t := str(get(n, "type"))
	if t == "array" {
		return "array of " + typeOf(get(n, "items"))
	}
	if t == "" {
		for _, k := range []string{"allOf", "oneOf", "anyOf"} {
			if c := get(n, k); c != nil {
				var parts []string
				for _, e := range c.Content {
					parts = append(parts, typeOf(e))
				}
				return k + "(" + strings.Join(parts, ", ") + ")"
			}
		}
		if get(n, "properties") != nil {
			return "object"
		}
		if ap := get(n, "additionalProperties"); ap != nil {
			return "map of " + typeOf(ap)
		}
		return "any"
	}
	if f := str(get(n, "format")); f != "" {
		t += " (" + f + ")"
	}
	if t == "object" {
		if ap := get(n, "additionalProperties"); ap != nil && ap.Kind == yaml.MappingNode {
			return "map of " + typeOf(ap)
		}
	}
	return t
}

func enumOf(n *yaml.Node) string {
	e := get(n, "enum")
	if e == nil {
		return ""
	}
	var vs []string
	for _, c := range e.Content {
		vs = append(vs, c.Value)
	}
	return strings.Join(vs, " | ")
}

// fields writes an object's properties as a nested list. Inline objects and
// arrays of inline objects are expanded; $refs are named, not expanded.
func fields(b *strings.Builder, n *yaml.Node, indent string, depth int) {
	if n == nil {
		return
	}
	req := map[string]bool{}
	if r := get(n, "required"); r != nil {
		for _, x := range r.Content {
			req[x.Value] = true
		}
	}
	for _, p := range pairs(get(n, "properties")) {
		line := fmt.Sprintf("%s- `%s` %s", indent, p.k, typeOf(p.v))
		if req[p.k] {
			line += " **required**"
		}
		if e := enumOf(p.v); e != "" {
			line += " — one of: " + e
		}
		if d := oneLine(str(get(p.v, "description")), 220); d != "" {
			line += " — " + d
		}
		if ex := get(p.v, "example"); ex != nil && ex.Kind == yaml.ScalarNode && ex.Value != "" {
			line += fmt.Sprintf(" (e.g. `%s`)", oneLine(ex.Value, 80))
		}
		b.WriteString(line + "\n")
		if depth >= 6 {
			continue
		}
		child := p.v
		if str(get(child, "type")) == "array" {
			child = get(child, "items")
		}
		if ap := get(child, "additionalProperties"); ap != nil && get(child, "properties") == nil {
			child = ap
		}
		if child != nil && refName(child) == "" && get(child, "properties") != nil {
			fields(b, child, indent+"  ", depth+1)
		}
	}
	for _, k := range []string{"allOf", "oneOf", "anyOf"} {
		if c := get(n, k); c != nil {
			for _, e := range c.Content {
				if rn := refName(e); rn != "" {
					fmt.Fprintf(b, "%s- (%s) → %s\n", indent, k, rn)
				} else {
					fields(b, e, indent, depth)
				}
			}
		}
	}
}

func toJSON(n *yaml.Node) ([]byte, error) {
	var v any
	if err := n.Decode(&v); err != nil {
		return nil, err
	}
	return json.MarshalIndent(normalize(v), "", "  ")
}

// normalize converts yaml's map[string]any / map[any]any into JSON-safe maps.
func normalize(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, x := range t {
			t[k] = normalize(x)
		}
		return t
	case map[any]any:
		m := map[string]any{}
		for k, x := range t {
			m[fmt.Sprint(k)] = normalize(x)
		}
		return m
	case []any:
		for i, x := range t {
			t[i] = normalize(x)
		}
		return t
	}
	return v
}

func contentSchema(n *yaml.Node) (string, *yaml.Node) {
	for _, c := range pairs(get(n, "content")) {
		return c.k, c.v
	}
	return "", nil
}

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: specdump <spec.yaml> <outdir>")
		os.Exit(2)
	}
	raw, err := os.ReadFile(os.Args[1])
	if err != nil {
		panic(err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		panic(err)
	}
	out := os.Args[2]
	if err := os.MkdirAll(out, 0o755); err != nil {
		panic(err)
	}

	// All examples go into one file, each under a "### <key>" heading, so the
	// folder stays small in git and `grep -n "### holdShipment" examples.md`
	// finds them.
	var exs strings.Builder
	title := strings.TrimSpace(str(get(get(&doc, "info"), "title")) + " " + str(get(get(&doc, "info"), "version")))
	if title == "" {
		title = "API"
	}
	exs.WriteString("# " + title + " — examples (generated from the spec)\n\n")
	exs.WriteString("Every example payload in the spec, keyed `<operationId>.<request|response-CODE>.<example name>`.\n\n")
	nEx := 0
	addEx := func(key string, v *yaml.Node) bool {
		b, err := toJSON(v)
		if err != nil {
			return false
		}
		fmt.Fprintf(&exs, "### %s\n\n```json\n%s\n```\n\n", key, b)
		nEx++
		return true
	}

	var ops strings.Builder
	ops.WriteString("# " + title + " — operations (generated from the spec)\n\n")
	ops.WriteString("Example payloads are in examples.md, under the key shown after each request or response.\n\n")
	nOps := 0
	for _, p := range pairs(get(&doc, "paths")) {
		for _, m := range pairs(p.v) {
			if m.k == "parameters" {
				continue
			}
			nOps++
			op := m.v
			id := str(get(op, "operationId"))
			if id == "" {
				id = strings.Trim(strings.NewReplacer("/", "-", "{", "", "}", "").Replace(p.k), "-")
			}
			fmt.Fprintf(&ops, "## %s %s\n\n", strings.ToUpper(m.k), p.k)
			fmt.Fprintf(&ops, "- operationId: `%s` · tags: %s\n", id, oneLine(joinSeq(get(op, "tags")), 0))
			if s := str(get(op, "summary")); s != "" {
				fmt.Fprintf(&ops, "- summary: %s\n", oneLine(s, 0))
			}
			if sec := get(op, "security"); sec != nil {
				var names []string
				for _, e := range sec.Content {
					for _, s := range pairs(e) {
						names = append(names, s.k)
					}
				}
				fmt.Fprintf(&ops, "- security: %s\n", strings.Join(names, ", "))
			}
			var params []*yaml.Node
			if ps := get(op, "parameters"); ps != nil {
				params = ps.Content
			}
			for _, prm := range params {
				pr := prm
				if rn := str(get(prm, "$ref")); rn != "" {
					pr = get(get(get(&doc, "components"), "parameters"), strings.TrimPrefix(rn, "#/components/parameters/"))
				}
				if str(get(pr, "name")) == "" {
					continue // the spec has a bare "-" entry under benefits/bulk
				}
				rq := ""
				if str(get(pr, "required")) == "true" {
					rq = " **required**"
				}
				fmt.Fprintf(&ops, "- param `%s` in %s %s%s — %s\n", str(get(pr, "name")), str(get(pr, "in")), typeOf(get(pr, "schema")), rq, oneLine(str(get(pr, "description")), 200))
			}
			if rb := get(op, "requestBody"); rb != nil {
				ct, media := contentSchema(rb)
				fmt.Fprintf(&ops, "- request body (%s): %s\n", ct, typeOf(get(media, "schema")))
				if v := get(media, "example"); v != nil {
					key := id + ".request.example"
					if addEx(key, v) {
						fmt.Fprintf(&ops, "  - example → `%s`\n", key)
					}
				}
				for _, ex := range pairs(get(media, "examples")) {
					if v := get(ex.v, "value"); v != nil {
						key := id + ".request." + sanitize(ex.k)
						if addEx(key, v) {
							fmt.Fprintf(&ops, "  - example `%s` → `%s`\n", ex.k, key)
						}
					}
				}
			}
			for _, r := range pairs(get(op, "responses")) {
				ct, media := contentSchema(r.v)
				t := ""
				if media != nil {
					t = typeOf(get(media, "schema"))
				}
				fmt.Fprintf(&ops, "- response **%s** %s %s — %s\n", r.k, ct, t, oneLine(str(get(r.v, "description")), 200))
				if v := get(media, "example"); v != nil {
					key := id + ".response-" + r.k + ".example"
					if addEx(key, v) {
						fmt.Fprintf(&ops, "  - example → `%s`\n", key)
					}
				}
				for _, ex := range pairs(get(media, "examples")) {
					if v := get(ex.v, "value"); v != nil {
						key := id + ".response-" + r.k + "." + sanitize(ex.k)
						if addEx(key, v) {
							fmt.Fprintf(&ops, "  - example `%s` → `%s`\n", ex.k, key)
						}
					}
				}
			}
			if d := flatten(str(get(op, "description"))); d != "" {
				ops.WriteString("\n<details><summary>Description from the spec</summary>\n\n```text\n" + d + "\n```\n\n</details>\n")
			}
			ops.WriteString("\n")
		}
	}

	var sch strings.Builder
	sch.WriteString("# " + title + " — schemas (generated from the spec)\n\n")
	schemas := pairs(get(get(&doc, "components"), "schemas"))
	names := make([]string, 0, len(schemas))
	for _, s := range schemas {
		names = append(names, s.k)
	}
	sort.Strings(names)
	sch.WriteString("Index: " + strings.Join(names, ", ") + "\n\n")
	for _, s := range schemas {
		fmt.Fprintf(&sch, "## %s\n\n", s.k)
		fmt.Fprintf(&sch, "type: %s", typeOf(s.v))
		if e := enumOf(s.v); e != "" {
			fmt.Fprintf(&sch, " — one of: %s", e)
		}
		sch.WriteString("\n")
		if d := oneLine(str(get(s.v, "description")), 400); d != "" {
			sch.WriteString("\n" + d + "\n")
		}
		sch.WriteString("\n")
		fields(&sch, s.v, "", 0)
		sch.WriteString("\n")
	}
	must(os.WriteFile(filepath.Join(out, "operations.md"), []byte(ops.String()), 0o644))
	must(os.WriteFile(filepath.Join(out, "schemas.md"), []byte(sch.String()), 0o644))
	must(os.WriteFile(filepath.Join(out, "examples.md"), []byte(exs.String()), 0o644))
	fmt.Printf("operations=%d schemas=%d examples=%d\n", nOps, len(schemas), nEx)
}

func joinSeq(n *yaml.Node) string {
	if n == nil {
		return ""
	}
	var vs []string
	for _, c := range n.Content {
		vs = append(vs, c.Value)
	}
	return strings.Join(vs, ", ")
}

var reUnsafe = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

func sanitize(s string) string { return strings.Trim(reUnsafe.ReplaceAllString(s, "-"), "-") }

func must(err error) {
	if err != nil {
		panic(err)
	}
}
