package scripts_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type checkChangedFixture struct {
	root    string
	env     []string
	goCalls string
	stdin   string
}

func setupCheckChangedFixture(t *testing.T) *checkChangedFixture {
	t.Helper()
	root := t.TempDir()
	script, err := os.ReadFile("check_changed.sh")
	if err != nil {
		t.Fatal(err)
	}
	logDir := t.TempDir()
	fix := &checkChangedFixture{
		root:    root,
		goCalls: filepath.Join(logDir, "go.log"),
		stdin:   filepath.Join(logDir, "changed.log"),
	}
	files := map[string]string{
		"scripts/check_changed.sh": string(script),
		"scripts/migrate.sh":       "#!/usr/bin/env bash\nexit \"${MIGRATE_EXIT:-0}\"\n",
		"scripts/test_services.sh": "TEST_DB_PORT=3307\nTEST_VALKEY_PORT=6380\n",
		"bin/go": `#!/usr/bin/env bash
echo "go $*" >> "$GO_CALLS"
if [ "$1" = run ]; then
    cat > "$CHANGED_INPUT"
    [ "${SELECT_EXIT:-0}" -eq 0 ] || exit "$SELECT_EXIT"
    echo ./selected
    exit 0
fi
case "$*" in *./selected*|*./...*) exit "${SELECTED_TEST_EXIT:-0}" ;; esac
exit 0
`,
		"tracked.txt": "base\n",
		"staged.txt":  "base\n",
		".gitignore":  "bin/\nignored.txt\n",
	}
	for name, content := range files {
		target := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	fix.env = []string{
		"PATH=" + filepath.Join(root, "bin") + ":" + os.Getenv("PATH"),
		"GO_CALLS=" + fix.goCalls,
		"CHANGED_INPUT=" + fix.stdin,
		"TEST_TIMEOUT=",
		"GOFLAGS=",
	}
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "user.name", "test"},
		{"config", "user.email", "test@example.com"},
		{"add", "-A"},
		{"commit", "-qm", "base"},
	} {
		if out, code := runCmd(t, root, nil, "git", args...); code != 0 {
			t.Fatalf("git %v: %s", args, out)
		}
	}
	return fix
}

func (f *checkChangedFixture) setOriginMain(t *testing.T) {
	t.Helper()
	if out, code := runCmd(t, f.root, nil, "git", "update-ref", "refs/remotes/origin/main", "HEAD"); code != 0 {
		t.Fatalf("git update-ref: %s", out)
	}
}

func (f *checkChangedFixture) run(t *testing.T, env ...string) (string, int) {
	t.Helper()
	return runCmd(t, f.root, append(f.env, env...), "bash", "./scripts/check_changed.sh")
}

func TestCheckChanged_PassesEveryKindOfChangeToSelection(t *testing.T) {
	fix := setupCheckChangedFixture(t)
	fix.setOriginMain(t)
	write := func(name string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(fix.root, name), []byte("changed\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("committed.txt")
	if out, code := runCmd(t, fix.root, nil, "git", "add", "committed.txt"); code != 0 {
		t.Fatal(out)
	}
	if out, code := runCmd(t, fix.root, nil, "git", "commit", "-qm", "committed"); code != 0 {
		t.Fatal(out)
	}
	write("staged.txt")
	if out, code := runCmd(t, fix.root, nil, "git", "add", "staged.txt"); code != 0 {
		t.Fatal(out)
	}
	write("tracked.txt")
	write("untracked.txt")
	write("ignored.txt")

	out, code := fix.run(t)
	if code != 0 {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	changed := readLog(t, fix.stdin)
	for _, want := range []string{"committed.txt", "staged.txt", "tracked.txt", "untracked.txt"} {
		if !strings.Contains(changed, want) {
			t.Errorf("selection input lacks %s:\n%s", want, changed)
		}
	}
	if strings.Contains(changed, "ignored.txt") {
		t.Errorf("selection input contains an ignored file:\n%s", changed)
	}
	calls := readLog(t, fix.goCalls)
	if want := "go test -count=1 ./internal/architecture ./internal/core ./selected"; !strings.Contains(calls, want) {
		t.Errorf("go calls lack %q:\n%s", want, calls)
	}
	if !strings.Contains(out, "./selected") {
		t.Errorf("selected packages were not printed:\n%s", out)
	}
}

func TestCheckChanged_FallsBackToFullSuite(t *testing.T) {
	tests := []struct {
		name      string
		hasOrigin bool
		env       []string
	}{
		{name: "missing origin/main"},
		{name: "selection failure", hasOrigin: true, env: []string{"SELECT_EXIT=1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fix := setupCheckChangedFixture(t)
			if tt.hasOrigin {
				fix.setOriginMain(t)
			}
			out, code := fix.run(t, tt.env...)
			if code != 0 {
				t.Fatalf("exit %d:\n%s", code, out)
			}
			if !strings.Contains(out, "Full suite:") {
				t.Errorf("fallback reason was not printed:\n%s", out)
			}
			if calls := readLog(t, fix.goCalls); !strings.Contains(calls, "./internal/core ./...") {
				t.Errorf("full suite was not run:\n%s", calls)
			}
		})
	}
}

func TestCheckChanged_ReportsFailures(t *testing.T) {
	tests := []struct {
		name     string
		noOrigin bool
		env      string
	}{
		{name: "selected test failure", env: "SELECTED_TEST_EXIT=1"},
		{name: "full-suite fallback failure", noOrigin: true, env: "SELECTED_TEST_EXIT=3"},
		{name: "service or migration failure", env: "MIGRATE_EXIT=1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fix := setupCheckChangedFixture(t)
			if !tt.noOrigin {
				fix.setOriginMain(t)
			}
			out, code := fix.run(t, tt.env)
			if code == 0 {
				t.Fatalf("expected a non-zero exit:\n%s", out)
			}
			if strings.Contains(out, "Selected tests passed") {
				t.Errorf("success was reported after a failure:\n%s", out)
			}
		})
	}
}
