package architecture_test

import (
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

var (
	markdownLink       = regexp.MustCompile(`\]\(([^)\s]+)\)`)
	markdownHeading    = regexp.MustCompile(`^#{1,6}\s+(.*?)\s*#*\s*$`)
	markdownAnchorTag  = regexp.MustCompile(`<a\s+(?:name|id)="([^"]+)"`)
	markdownInlineCode = regexp.MustCompile("`[^`]*`")
	markdownLinkText   = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	markdownScheme     = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*:`)
)

// markdownSkipDirs are untracked or generated trees; party2-main is the
// gitignored legacy checkout.
var markdownSkipDirs = []string{".git", "party2-main", "node_modules", ".worktrees", ".cache", "bin", "dist", "out"}

// markdownLines returns a file's lines outside fenced code blocks.
func markdownLines(text string) []string {
	var lines []string
	fence := ""
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if fence == "" && (strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~")) {
			fence = trimmed[:3]
			lines = append(lines, "")
			continue
		}
		if fence != "" {
			if strings.HasPrefix(trimmed, fence) {
				fence = ""
			}
			lines = append(lines, "")
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

// markdownSlug follows GitHub's heading anchors: lower case, punctuation
// dropped, spaces become hyphens.
func markdownSlug(heading string) string {
	heading = markdownLinkText.ReplaceAllString(heading, "$1")
	heading = strings.ReplaceAll(heading, "`", "")
	var b strings.Builder
	for _, r := range strings.ToLower(heading) {
		switch {
		case r == ' ':
			b.WriteRune('-')
		case r == '-' || r == '_' || unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsMark(r):
			b.WriteRune(r)
		}
	}
	return b.String()
}

func markdownAnchors(text string) map[string]bool {
	anchors := map[string]bool{}
	seen := map[string]int{}
	for _, line := range markdownLines(text) {
		if m := markdownHeading.FindStringSubmatch(line); m != nil {
			slug := markdownSlug(m[1])
			if n := seen[slug]; n > 0 {
				anchors[slug+"-"+strconv.Itoa(n)] = true
			} else {
				anchors[slug] = true
			}
			seen[slug]++
		}
		for _, m := range markdownAnchorTag.FindAllStringSubmatch(line, -1) {
			anchors[m[1]] = true
		}
	}
	return anchors
}

// brokenMarkdownLinks reports relative links under root whose file or
// Markdown heading anchor does not exist.
func brokenMarkdownLinks(root string) ([]string, error) {
	anchorCache := map[string]map[string]bool{}
	anchorsOf := func(path string) (map[string]bool, error) {
		if a, ok := anchorCache[path]; ok {
			return a, nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		anchorCache[path] = markdownAnchors(string(data))
		return anchorCache[path], nil
	}
	var broken []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && slices.Contains(markdownSkipDirs, d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".md") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		for i, line := range markdownLines(string(data)) {
			for _, m := range markdownLink.FindAllStringSubmatch(markdownInlineCode.ReplaceAllString(line, ""), -1) {
				target := m[1]
				if markdownScheme.MatchString(target) {
					continue
				}
				file, anchor, _ := strings.Cut(target, "#")
				if unescaped, err := url.PathUnescape(file); err == nil {
					file = unescaped
				}
				dest := path
				if file != "" {
					dest = filepath.Join(filepath.Dir(path), filepath.FromSlash(file))
				}
				where := filepath.ToSlash(rel) + ":" + strconv.Itoa(i+1) + ": " + target
				if _, err := os.Stat(dest); err != nil {
					broken = append(broken, where+" (missing file)")
					continue
				}
				if anchor == "" || !strings.HasSuffix(dest, ".md") {
					continue
				}
				if unescaped, err := url.PathUnescape(anchor); err == nil {
					anchor = unescaped
				}
				anchors, err := anchorsOf(dest)
				if err != nil {
					return err
				}
				if !anchors[strings.ToLower(anchor)] {
					broken = append(broken, where+" (missing anchor)")
				}
			}
		}
		return nil
	})
	return broken, err
}

// TestMarkdownRelativeLinksResolve keeps relative document links and heading
// anchors valid (.agents/rules/02-documentation-sync.md §4).
func TestMarkdownRelativeLinksResolve(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	broken, err := brokenMarkdownLinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(broken) > 0 {
		t.Errorf("broken relative Markdown links:\n  %s", strings.Join(broken, "\n  "))
	}
}

func TestMarkdownRelativeLinksResolve_SelfTest(t *testing.T) {
	root := t.TempDir()
	write := func(name, text string) {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("docs/target.md", "# Target\n\n## 5. Mailing Items and Money (`おくる`)\n\n## Repeat\n\n## Repeat\n\n<a id=\"custom\"></a>\n")
	write("docs/source.md", strings.Join([]string{
		"[ok](target.md) [ok](target.md#5-mailing-items-and-money-おくる) [ok](target.md#repeat-1) [ok](target.md#custom)",
		"[ok](../README.md) [ok](https://example.com/x.md#nope) [self](#source)",
		"`[ignored](missing.md)`",
		"```",
		"[ignored](missing.md)",
		"```",
		"# Source",
		"[bad](missing.md)",
		"[bad](target.md#nope)",
		"[bad](#nope)",
	}, "\n"))
	write("README.md", "# Readme\n")
	write("party2-main/legacy.md", "[ignored](missing.md)\n")
	broken, err := brokenMarkdownLinks(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"docs/source.md:8: missing.md (missing file)",
		"docs/source.md:9: target.md#nope (missing anchor)",
		"docs/source.md:10: #nope (missing anchor)",
	}
	if !slices.Equal(broken, want) {
		t.Fatalf("broken = %q\nwant %q", broken, want)
	}
}
