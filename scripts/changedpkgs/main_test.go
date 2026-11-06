package main

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// writeModule creates a real module: app -> lib -> base, testuser's external
// test -> base, and assetreader, which embeds data/* and names "docs" in a test.
func writeModule(t *testing.T) {
	t.Helper()
	files := map[string]string{
		"go.mod":                      "module example.test\n\ngo 1.26\n",
		"base/base.go":                "package base\n",
		"lib/lib.go":                  "package lib\n\nimport _ \"example.test/base\"\n",
		"app/app.go":                  "package app\n\nimport _ \"example.test/lib\"\n",
		"testuser/testuser.go":        "package testuser\n",
		"testuser/testuser_test.go":   "package testuser_test\n\nimport _ \"example.test/base\"\n",
		"leaf/leaf.go":                "package leaf\n",
		"assetreader/reader.go":       "package assetreader\n\nimport _ \"embed\"\n\n//go:embed data/input.txt\nvar Input string\n",
		"assetreader/data/input.txt":  "x\n",
		"assetreader/reader_test.go":  "package assetreader\n\nconst specPath = \"../docs/spec.md\"\n",
		"assetreader/nested/nest.go":  "package nested\n",
		"namedreader/namedreader.go":  "package namedreader\n",
		"namedreader/reader_test.go":  "package namedreader\n\nconst shared = \"assetreader/data/input.txt\"\n",
		"testdata/ignored/ignored.go": "package ignored\n",
	}
	root := t.TempDir()
	for name, content := range files {
		target := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(root)
}

func TestRunSelectsAffectedPackages(t *testing.T) {
	writeModule(t)
	tests := []struct {
		name    string
		changed string
		want    []string
		reason  string
	}{
		{name: "no changes"},
		{name: "leaf package", changed: "leaf/leaf.go", want: []string{"./leaf"}},
		{
			name:    "transitive dependents and external test importers",
			changed: "base/base.go",
			want:    []string{"./app", "./base", "./lib", "./testuser"},
		},
		{name: "test file in package", changed: "lib/lib_test.go", want: []string{"./app", "./lib"}},
		{
			name:    "embedded file selects owner and a test naming it",
			changed: "assetreader/data/input.txt",
			want:    []string{"./assetreader", "./namedreader"},
		},
		{
			name:    "deleted embedded file selects its ancestor package",
			changed: "assetreader/data/removed.txt",
			want:    []string{"./assetreader"},
		},
		{name: "ancestor packages of nested input", changed: "assetreader/nested/x.txt", want: []string{"./assetreader", "./assetreader/nested"}},
		{name: "relative-path test input", changed: "docs/spec.md", want: []string{"./assetreader"}},
		{name: "root markdown nobody reads", changed: "README.md"},
		{name: "go.mod", changed: "go.mod", want: []string{fullSuite}, reason: "go.mod"},
		{name: "go.sum", changed: "go.sum", want: []string{fullSuite}, reason: "go.sum"},
		{name: "compose", changed: "compose.yaml", want: []string{fullSuite}, reason: "compose.yaml"},
		{name: "migration", changed: "migrations/001_x.sql", want: []string{fullSuite}, reason: "migrations/001_x.sql"},
		{name: "selection tool", changed: "scripts/changedpkgs/main.go", want: []string{fullSuite}, reason: "selection logic"},
		{name: "selection script", changed: "scripts/check_changed.sh", want: []string{fullSuite}, reason: "selection logic"},
		{name: "unknown root file", changed: "unknown.yaml", want: []string{fullSuite}, reason: "unknown.yaml"},
		{name: "deleted package", changed: "removed/removed.go", want: []string{fullSuite}, reason: "removed/removed.go"},
		{name: "go file outside packages", changed: "testdata/ignored/ignored.go", want: []string{fullSuite}, reason: "testdata/ignored"},
		{name: "git-quoted path", changed: `"caf\303\251.txt"`, want: []string{fullSuite}, reason: "cannot be mapped"},
		{name: "one unmappable file wins", changed: "leaf/leaf.go\ngo.mod", want: []string{fullSuite}, reason: "go.mod"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, diag bytes.Buffer
			if err := run(strings.NewReader(tt.changed+"\n"), &out, &diag); err != nil {
				t.Fatal(err)
			}
			got := strings.Fields(out.String())
			if !slices.Equal(got, tt.want) {
				t.Errorf("patterns = %v, want %v", got, tt.want)
			}
			if tt.reason == "" && diag.Len() > 0 {
				t.Errorf("unexpected fallback reason: %s", diag.String())
			}
			if !strings.Contains(diag.String(), tt.reason) {
				t.Errorf("reason = %q, want it to mention %q", diag.String(), tt.reason)
			}
		})
	}
}

func TestRunFailsWhenPackagesCannotBeListed(t *testing.T) {
	t.Chdir(t.TempDir())
	var out, diag bytes.Buffer
	if err := run(strings.NewReader("leaf/leaf.go\n"), &out, &diag); err == nil {
		t.Fatalf("expected an error outside a module, got output %q", out.String())
	}
}
