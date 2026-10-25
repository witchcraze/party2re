package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestStressEntrypoint(t *testing.T) {
	script, err := os.ReadFile("stress_test.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name      string
		args      []string
		env       []string
		wantError bool
	}{
		{name: "all packages"},
		{name: "focused", args: []string{"-run", "TestFeature", "./internal/gvg"}},
		{name: "custom connections", env: []string{"PARTY2_DB_DSN=custom-db", "PARTY2_TEST_DB_ADMIN_DSN=custom-admin", "PARTY2_VALKEY_ADDR=custom-cache"}},
		{name: "service failure", env: []string{"SERVICE_EXIT=1"}, wantError: true},
		{name: "migration failure", env: []string{"MIGRATE_EXIT=1"}, wantError: true},
		{name: "test failure", env: []string{"TEST_EXIT=7"}, wantError: true},
		{name: "no matching tests", env: []string{`TEST_OUTPUT={"Action":"pass","Package":"fixture"}`}, wantError: true},
		{name: "all skipped", env: []string{`TEST_OUTPUT={"Action":"skip","Package":"fixture","Test":"TestFeature"}`}, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			bin := filepath.Join(root, "scripts")
			if err := os.Mkdir(bin, 0o755); err != nil {
				t.Fatal(err)
			}
			copyStressChecker(t, bin)
			goPath, err := exec.LookPath("go")
			if err != nil {
				t.Fatal(err)
			}
			files := map[string]string{
				"stress_test.sh": string(script),
				"migrate.sh":     "#!/bin/bash\nexit \"${MIGRATE_EXIT:-0}\"\n",
				"docker":         "#!/bin/bash\nexit \"${SERVICE_EXIT:-0}\"\n",
				"go": `#!/bin/bash
if [ "$1" = run ]; then exec "$REAL_GO" "$@"; fi
printf '%s\n' "$@" > "$STRESS_CALL"
printf '%s\n' "$PARTY2_DB_DSN" "$PARTY2_TEST_DB_ADMIN_DSN" "$PARTY2_VALKEY_ADDR" "$PARTY2_STRESS_ENABLED" >> "$STRESS_CALL"
printf '%s\n' "$TEST_OUTPUT"
exit "${TEST_EXIT:-0}"
`,
			}
			for name, content := range files {
				if err := os.WriteFile(filepath.Join(bin, name), []byte(content), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			call := filepath.Join(root, "call")
			cmd := exec.Command("bash", append([]string{filepath.Join(bin, "stress_test.sh")}, tc.args...)...)
			cmd.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"), "STRESS_CALL="+call, "REAL_GO="+goPath,
				`TEST_OUTPUT={"Action":"pass","Package":"fixture","Test":"TestFeature"}`,
				"PARTY2_DB_DSN=", "PARTY2_TEST_DB_ADMIN_DSN=", "PARTY2_VALKEY_ADDR=", "PARTY2_STRESS_ENABLED=0")
			cmd.Env = append(cmd.Env, tc.env...)
			output, err := cmd.CombinedOutput()
			if (err != nil) != tc.wantError {
				t.Fatalf("error = %v, wantError = %v\n%s", err, tc.wantError, output)
			}
			if tc.wantError {
				if tc.name == "test failure" && err.(*exec.ExitError).ExitCode() != 7 {
					t.Fatalf("Go exit code was not preserved: %v", err)
				}
				if strings.Contains(string(output), "COMPLETED") {
					t.Fatalf("failure reported success: %s", output)
				}
				if tc.name == "service failure" || tc.name == "migration failure" {
					if _, err := os.Stat(call); !os.IsNotExist(err) {
						t.Fatal("tests ran after setup failed")
					}
				}
				return
			}
			invocation, err := os.ReadFile(call)
			if err != nil {
				t.Fatal(err)
			}
			args := tc.args
			if len(args) == 0 {
				args = []string{"./..."}
			}
			connections := []string{"party2:party2@tcp(127.0.0.1:3306)/party2?parseTime=true", "root:root@tcp(127.0.0.1:3306)/?parseTime=true", "127.0.0.1:6379"}
			if tc.name == "custom connections" {
				connections = []string{"custom-db", "custom-admin", "custom-cache"}
			}
			want := append([]string{"test", "-json", "-v", "-count=1"}, args...)
			want = append(want, connections...)
			want = append(want, "1")
			if string(invocation) != strings.Join(want, "\n")+"\n" {
				t.Fatalf("invocation:\n%s\nwant:\n%s", invocation, strings.Join(want, "\n"))
			}
		})
	}
}

func TestStressEntrypointRealGo(t *testing.T) {
	root := t.TempDir()
	script, err := os.ReadFile("stress_test.sh")
	if err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"scripts/stress_test.sh": string(script),
		"scripts/migrate.sh":     "#!/bin/bash\nexit 0\n",
		"scripts/docker":         "#!/bin/bash\nexit 0\n",
		"go.mod":                 "module stressfixture\n\ngo 1.26\n",
		"fixture_test.go": `package fixture
import ("os"; "testing")
func TestPass(t *testing.T) { if os.Getenv("PARTY2_STRESS_ENABLED") != "1" { t.Fatal("deep flag missing") } }
func TestParent(t *testing.T) { t.Run("child", TestPass) }
func TestSkipped(t *testing.T) { t.Skip("fixture") }
func TestSkippedChildren(t *testing.T) { t.Run("group/child", TestSkipped) }
func TestFailure(t *testing.T) { t.Fatal("expected fixture failure") }
`,
		"unmatched/fixture_test.go": "package unmatched\nimport \"testing\"\nfunc TestOther(t *testing.T) {}\n",
	} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	copyStressChecker(t, filepath.Join(root, "scripts"))
	for _, tc := range []struct {
		name, pattern, format string
		code                  int
	}{
		{name: "normal", pattern: "^TestPass$"},
		{name: "subtest", pattern: "^TestParent$/^child$"},
		{name: "missing test", pattern: "^Nonexistent$", code: 1},
		{name: "missing subtest", pattern: "^TestParent$/^Nonexistent$", code: 1},
		{name: "skip", pattern: "^TestSkipped$", code: 1},
		{name: "skipped children", pattern: "^TestSkippedChildren$", code: 1},
		{name: "failure", pattern: "^TestFailure$", code: 1},
		{name: "json success", pattern: "^TestPass$", format: "-json"},
		{name: "json subtest", pattern: "^TestParent$/^child$", format: "-json"},
		{name: "json missing subtest", pattern: "^TestParent$/^Nonexistent$", format: "-json", code: 1},
		{name: "json failure", pattern: "^TestFailure$", format: "-json", code: 1},
		{name: "json disabled", pattern: "^TestPass$", format: "-json=false"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := []string{filepath.Join(root, "scripts/stress_test.sh"), "-run", tc.pattern, "./..."}
			if tc.format != "" {
				args = append(args, tc.format)
			}
			cmd := exec.Command("bash", args...)
			cmd.Env = append(os.Environ(), "PATH="+filepath.Join(root, "scripts")+":"+os.Getenv("PATH"), "PARTY2_STRESS_ENABLED=0")
			output, err := cmd.CombinedOutput()
			code := 0
			if err != nil {
				if exit, ok := err.(*exec.ExitError); ok {
					code = exit.ExitCode()
				} else {
					t.Fatal(err)
				}
			}
			if code != tc.code || (code != 0 && strings.Contains(string(output), "COMPLETED")) {
				t.Fatalf("exit = %d, want %d\n%s", code, tc.code, output)
			}
			if tc.code == 0 {
				if tc.format == "-json" {
					if !strings.Contains(string(output), `"Action":"pass"`) {
						t.Fatalf("missing JSON events: %s", output)
					}
				} else if !strings.Contains(string(output), "--- PASS:") || strings.Contains(string(output), `"Action":`) {
					t.Fatalf("expected normal output: %s", output)
				}
			}
		})
	}
}

func copyStressChecker(t *testing.T, scriptsDir string) {
	t.Helper()
	content, err := os.ReadFile("stresscheck/main.go")
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(scriptsDir, "stresscheck")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), content, 0o644); err != nil {
		t.Fatal(err)
	}
}
