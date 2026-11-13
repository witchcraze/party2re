package scripts_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type verifyFixture struct {
	root         string
	binDir       string
	dockerRecord string
	goRecord     string
	env          []string
}

func setupVerifyFixture(t *testing.T, isGit bool) *verifyFixture {
	t.Helper()
	root := t.TempDir()

	// Copy scripts/verify.sh
	verifyScript, err := os.ReadFile("verify.sh")
	if err != nil {
		t.Fatal(err)
	}
	scriptsDir := filepath.Join(root, "scripts")
	if err := os.MkdirAll(scriptsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(scriptsDir, "verify.sh"), verifyScript, 0o755); err != nil {
		t.Fatal(err)
	}

	testServices, err := os.ReadFile("test_services.sh")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(scriptsDir, "test_services.sh"), testServices, 0o644); err != nil {
		t.Fatal(err)
	}

	// Copy .githooks/pre-push
	prePushScript, err := os.ReadFile(filepath.Join("..", ".githooks", "pre-push"))
	if err == nil {
		githooksDir := filepath.Join(root, ".githooks")
		if err := os.MkdirAll(githooksDir, 0o755); err == nil {
			_ = os.WriteFile(filepath.Join(githooksDir, "pre-push"), prePushScript, 0o755)
		}
	}

	// Mock migrate.sh
	migrateScript := "#!/usr/bin/env bash\nexit \"${MIGRATE_EXIT:-0}\"\n"
	if err := os.WriteFile(filepath.Join(scriptsDir, "migrate.sh"), []byte(migrateScript), 0o755); err != nil {
		t.Fatal(err)
	}

	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}

	logDir := t.TempDir()
	dockerRecord := filepath.Join(logDir, "docker.log")
	goRecord := filepath.Join(logDir, "go.log")

	// Stand-in go
	mockGo := `#!/usr/bin/env bash
if [ -n "${GO_RECORD:-}" ]; then
    echo "go $@" >> "$GO_RECORD"
fi
if [ "$1" = "list" ] && [ "${2:-}" = "./..." ]; then
    printf 'github.com/witchcraze/party2re/internal/architecture\ngithub.com/witchcraze/party2re/internal/sample\n'
    if [ "${GO_LIST_PARTIAL_FAIL:-0}" != "0" ]; then
        exit 1
    fi
fi
if [ -n "${GO_MUTATE:-}" ]; then
    eval "$GO_MUTATE"
fi
exit "${GO_EXIT:-0}"
`
	if err := os.WriteFile(filepath.Join(binDir, "go"), []byte(mockGo), 0o755); err != nil {
		t.Fatal(err)
	}

	// Stand-in docker
	mockDocker := `#!/usr/bin/env bash
if [ -n "${DOCKER_RECORD:-}" ]; then
    echo "docker $@" >> "$DOCKER_RECORD"
fi
if [ -n "${DOCKER_MUTATE:-}" ]; then
    eval "$DOCKER_MUTATE"
fi
if [ "${DOCKER_EXEC_APP_SH:-0}" != "0" ] && [ "$1" = "compose" ] && [ "$2" = "run" ] && [ "$3" = "--rm" ] && [ "$4" = "app" ] && [ "$5" = "sh" ] && [ "$6" = "-c" ]; then
    PATH="${CONTAINER_PATH:-$PATH}" sh -c "$7"
    exit $?
fi
exit "${DOCKER_EXIT:-0}"
`
	if err := os.WriteFile(filepath.Join(binDir, "docker"), []byte(mockDocker), 0o755); err != nil {
		t.Fatal(err)
	}

	// .gitignore
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("/.cache/\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// sample.go
	initialGo := "package sample\n\nconst Value = 1\n"
	if err := os.WriteFile(filepath.Join(root, "sample.go"), []byte(initialGo), 0o644); err != nil {
		t.Fatal(err)
	}

	if isGit {
		runCmd(t, root, nil, "git", "init", "-q")
		runCmd(t, root, nil, "git", "config", "user.name", "test")
		runCmd(t, root, nil, "git", "config", "user.email", "test@example.com")
	}

	env := []string{
		"PATH=" + binDir + ":" + os.Getenv("PATH"),
		"DOCKER_RECORD=" + dockerRecord,
		"GO_RECORD=" + goRecord,
		"FORCE_VERIFY=0",
		"TEST_TIMEOUT=",
		"PARTY2_TEST_TIMEOUT=",
		"GOFLAGS=",
	}

	return &verifyFixture{
		root:         root,
		binDir:       binDir,
		dockerRecord: dockerRecord,
		goRecord:     goRecord,
		env:          env,
	}
}

func runCmd(t *testing.T, dir string, env []string, name string, args ...string) (string, int) {
	t.Helper()
	lookupPath := os.Getenv("PATH")
	for _, e := range env {
		if strings.HasPrefix(e, "PATH=") {
			lookupPath = strings.TrimPrefix(e, "PATH=")
			break
		}
	}
	for _, p := range filepath.SplitList(lookupPath) {
		candidate := filepath.Join(p, name)
		if fi, err := os.Stat(candidate); err == nil && !fi.IsDir() && fi.Mode()&0o111 != 0 {
			name = candidate
			break
		}
	}
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	code := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		} else {
			t.Fatalf("command failed to start: %v", err)
		}
	}
	return out.String(), code
}

func TestTestServices_SelectsStackOrFallsBack(t *testing.T) {
	script, err := filepath.Abs("test_services.sh")
	if err != nil {
		t.Fatal(err)
	}
	probe := `ROOT_DIR=/repo; . "$0"; echo "selected=${COMPOSE_FILE:-}|$TEST_DB_PORT|$TEST_VALKEY_PORT"`

	for _, tc := range []struct {
		name, upExit, want string
		warns              bool
	}{
		{name: "stack starts", upExit: "0", want: "selected=/repo/compose.test.yaml|4407|7380"},
		{name: "stack unavailable", upExit: "1", want: "selected=|3306|6379", warns: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			binDir := t.TempDir()
			fake := "#!/usr/bin/env bash\n[ \"$2\" = up ] && exit " + tc.upExit + "\nexit 0\n"
			if err := os.WriteFile(filepath.Join(binDir, "docker"), []byte(fake), 0o755); err != nil {
				t.Fatal(err)
			}
			env := []string{"PATH=" + binDir + ":" + os.Getenv("PATH"), "PARTY2_TEST_DB_PORT=4407", "PARTY2_TEST_VALKEY_PORT=7380"}
			out, code := runCmd(t, t.TempDir(), env, "bash", "-c", probe, script)
			if code != 0 || !strings.Contains(out, tc.want) {
				t.Fatalf("code=%d, want %q:\n%s", code, tc.want, out)
			}
			if strings.Contains(out, "WARNING") != tc.warns {
				t.Fatalf("warning presence should be %v:\n%s", tc.warns, out)
			}
		})
	}
}

func TestVerify_ClearsTestCacheOnSchemaChangeOrForce(t *testing.T) {
	fix := setupVerifyFixture(t, true)
	if err := os.MkdirAll(filepath.Join(fix.root, "migrations"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFixture := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(fix.root, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	for _, step := range []struct {
		name      string
		change    func()
		env       []string
		wantClear bool
	}{
		{name: "first run has no stamp", change: func() {}, wantClear: true},
		{name: "source change keeps cache", change: func() { writeFixture("sample.go", "package sample\n\nconst Value = 2\n") }},
		{name: "migration change clears cache", change: func() { writeFixture("migrations/001.sql", "SELECT 1;\n") }, wantClear: true},
		{name: "force clears cache", change: func() {}, env: []string{"FORCE_VERIFY=1"}, wantClear: true},
	} {
		step.change()
		if err := os.WriteFile(fix.goRecord, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		out, code := runCmd(t, fix.root, append(fix.env, step.env...), "bash", "./scripts/verify.sh")
		if code != 0 {
			t.Fatalf("%s: verify failed with code %d:\n%s", step.name, code, out)
		}
		goLog, err := os.ReadFile(fix.goRecord)
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Contains(string(goLog), "go clean -testcache"); got != step.wantClear {
			t.Fatalf("%s: cleared = %v, want %v:\n%s", step.name, got, step.wantClear, goLog)
		}
		if !strings.Contains(string(goLog), "go test -timeout=30m github.com/witchcraze/party2re/internal/sample") {
			t.Fatalf("%s: full suite must allow result caching and exclude internal/architecture:\n%s", step.name, goLog)
		}
	}
}

func TestVerify_UnchangedTreeCached(t *testing.T) {
	fix := setupVerifyFixture(t, true)

	// First run: full verification
	out1, code1 := runCmd(t, fix.root, fix.env, "bash", "./scripts/verify.sh")
	if code1 != 0 {
		t.Fatalf("first run failed with code %d:\n%s", code1, out1)
	}
	if !strings.Contains(out1, "ALL VERIFICATION CHECKS PASSED!") {
		t.Fatalf("first run missing success message:\n%s", out1)
	}

	stampPath := filepath.Join(fix.root, ".cache", "verify_tree_stamp")
	stampBytes, err := os.ReadFile(stampPath)
	if err != nil {
		t.Fatalf("stamp file not found: %v", err)
	}
	stamp1 := strings.TrimSpace(string(stampBytes))
	if stamp1 == "" {
		t.Fatal("stamp file is empty")
	}

	// Clear docker record
	_ = os.Remove(fix.dockerRecord)

	// Second run: cached, skips duplicate verification
	out2, code2 := runCmd(t, fix.root, fix.env, "bash", "./scripts/verify.sh")
	if code2 != 0 {
		t.Fatalf("second run failed with code %d:\n%s", code2, out2)
	}
	if !strings.Contains(out2, "Skipping duplicate verification") {
		t.Fatalf("second run did not skip verification:\n%s", out2)
	}

	// Verify docker build was not invoked on the second run
	if _, err := os.Stat(fix.dockerRecord); !os.IsNotExist(err) {
		t.Fatal("docker was invoked on second run despite cached tree")
	}
}

func TestVerify_RejectsChangesDuringVerification(t *testing.T) {
	fix := setupVerifyFixture(t, true)

	// Inject fault: stand-in docker rewrites sample.go during smoke build step
	mutateEnv := append(fix.env, `DOCKER_MUTATE=echo "package sample; const Value = 2" > sample.go`)

	out1, code1 := runCmd(t, fix.root, mutateEnv, "bash", "./scripts/verify.sh")
	if code1 == 0 {
		t.Fatalf("expected verify.sh to fail on changed tree, but exited 0:\n%s", out1)
	}
	if !strings.Contains(out1, "Working tree changed during verification") {
		t.Fatalf("expected error message about changed working tree, got:\n%s", out1)
	}

	stampPath := filepath.Join(fix.root, ".cache", "verify_tree_stamp")
	if stampContent, err := os.ReadFile(stampPath); err == nil {
		t.Fatalf("stamp file was written despite verification failure: %q", string(stampContent))
	}

	// Subsequent run without mutation: verify.sh must NOT skip checks for Value = 2
	_ = os.Remove(fix.dockerRecord)
	out2, code2 := runCmd(t, fix.root, fix.env, "bash", "./scripts/verify.sh")
	if code2 != 0 {
		t.Fatalf("subsequent run failed with code %d:\n%s", code2, out2)
	}
	if strings.Contains(out2, "Skipping duplicate verification") {
		t.Fatalf("subsequent run skipped verification of unverified changes:\n%s", out2)
	}
	if !strings.Contains(out2, "ALL VERIFICATION CHECKS PASSED!") {
		t.Fatalf("subsequent run missing success message:\n%s", out2)
	}

	// Third run: unchanged Value = 2, now it should skip
	_ = os.Remove(fix.dockerRecord)
	out3, code3 := runCmd(t, fix.root, fix.env, "bash", "./scripts/verify.sh")
	if code3 != 0 {
		t.Fatalf("third run failed with code %d:\n%s", code3, out3)
	}
	if !strings.Contains(out3, "Skipping duplicate verification") {
		t.Fatalf("third run did not skip verification:\n%s", out3)
	}
}

func TestVerify_AutoFormattingChangesVerifiedAndCached(t *testing.T) {
	fix := setupVerifyFixture(t, true)

	// Write unformatted Go file
	unformatted := "package sample\n\nfunc   Foo()   {\nprintln(1)\n}\n"
	if err := os.WriteFile(filepath.Join(fix.root, "sample.go"), []byte(unformatted), 0o644); err != nil {
		t.Fatal(err)
	}

	out1, code1 := runCmd(t, fix.root, fix.env, "bash", "./scripts/verify.sh")
	if code1 != 0 {
		t.Fatalf("run failed with code %d:\n%s", code1, out1)
	}
	if !strings.Contains(out1, "Auto-formatting files:") {
		t.Fatalf("expected auto-formatting output, got:\n%s", out1)
	}
	if !strings.Contains(out1, "ALL VERIFICATION CHECKS PASSED!") {
		t.Fatalf("missing success message:\n%s", out1)
	}

	// Verify sample.go was formatted
	formattedBytes, err := os.ReadFile(filepath.Join(fix.root, "sample.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(formattedBytes) == unformatted {
		t.Fatal("sample.go was not formatted")
	}

	// Second run: tree is now formatted and cached, should skip
	out2, code2 := runCmd(t, fix.root, fix.env, "bash", "./scripts/verify.sh")
	if code2 != 0 {
		t.Fatalf("second run failed with code %d:\n%s", code2, out2)
	}
	if !strings.Contains(out2, "Skipping duplicate verification") {
		t.Fatalf("second run did not skip verification:\n%s", out2)
	}
}

func TestVerify_ForceVerify(t *testing.T) {
	fix := setupVerifyFixture(t, true)

	// First run
	_, code1 := runCmd(t, fix.root, fix.env, "bash", "./scripts/verify.sh")
	if code1 != 0 {
		t.Fatalf("first run failed with code %d", code1)
	}

	// Second run with FORCE_VERIFY=1
	_ = os.Remove(fix.dockerRecord)
	forceEnv := append(fix.env, "FORCE_VERIFY=1")
	out2, code2 := runCmd(t, fix.root, forceEnv, "bash", "./scripts/verify.sh")
	if code2 != 0 {
		t.Fatalf("second run failed with code %d:\n%s", code2, out2)
	}
	if strings.Contains(out2, "Skipping duplicate verification") {
		t.Fatalf("FORCE_VERIFY=1 still skipped verification:\n%s", out2)
	}
	if _, err := os.Stat(fix.dockerRecord); os.IsNotExist(err) {
		t.Fatal("docker was not invoked when FORCE_VERIFY=1")
	}
}

func TestVerify_NonGitEnvironment(t *testing.T) {
	fix := setupVerifyFixture(t, false)

	out, code := runCmd(t, fix.root, fix.env, "bash", "./scripts/verify.sh")
	if code != 0 {
		t.Fatalf("verify failed in non-git environment with code %d:\n%s", code, out)
	}
	if !strings.Contains(out, "ALL VERIFICATION CHECKS PASSED!") {
		t.Fatalf("missing success message in non-git environment:\n%s", out)
	}

	stampPath := filepath.Join(fix.root, ".cache", "verify_tree_stamp")
	if _, err := os.Stat(stampPath); !os.IsNotExist(err) {
		t.Fatal("stamp file was created in non-git environment")
	}
}

func TestPrePush_PropagatesFailureOnChangedTree(t *testing.T) {
	fix := setupVerifyFixture(t, true)

	prePushScript := filepath.Join(fix.root, ".githooks", "pre-push")
	if _, err := os.Stat(prePushScript); os.IsNotExist(err) {
		t.Skip(".githooks/pre-push not found in fixture")
	}

	mutateEnv := append(fix.env, `DOCKER_MUTATE=echo "package sample; const Value = 2" > sample.go`)
	out, code := runCmd(t, fix.root, mutateEnv, "bash", prePushScript)
	if code == 0 {
		t.Fatalf("expected pre-push to fail when verify fails, but exited 0:\n%s", out)
	}
	if strings.Contains(out, "All checks passed. Proceeding with push.") {
		t.Fatalf("pre-push reported success despite failure:\n%s", out)
	}
}

func TestVerify_TimeoutConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name        string
		extraEnv    []string
		wantTimeout string
	}{
		{
			name:        "default 30m timeout",
			wantTimeout: "-timeout=30m",
		},
		{
			name:        "override via TEST_TIMEOUT",
			extraEnv:    []string{"TEST_TIMEOUT=45m"},
			wantTimeout: "-timeout=45m",
		},
		{
			name:        "override via PARTY2_TEST_TIMEOUT",
			extraEnv:    []string{"PARTY2_TEST_TIMEOUT=1h"},
			wantTimeout: "-timeout=1h",
		},
		{
			name:        "preserve GOFLAGS with -timeout",
			extraEnv:    []string{"GOFLAGS=-timeout=20m"},
			wantTimeout: "",
		},
		{
			name:        "preserve unrelated GOFLAGS with default timeout",
			extraEnv:    []string{"GOFLAGS=-p=2"},
			wantTimeout: "-timeout=30m",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fix := setupVerifyFixture(t, false)
			env := append(fix.env, tc.extraEnv...)
			out, code := runCmd(t, fix.root, env, "bash", "./scripts/verify.sh")
			if code != 0 {
				t.Fatalf("verify failed with code %d:\n%s", code, out)
			}
			goLog, err := os.ReadFile(fix.goRecord)
			if err != nil {
				t.Fatalf("could not read go record: %v", err)
			}
			var fullSuiteCall string
			for _, line := range strings.Split(string(goLog), "\n") {
				if strings.HasPrefix(line, "go test") && strings.Contains(line, "github.com/witchcraze/party2re/internal/sample") {
					fullSuiteCall = line
					break
				}
			}
			if fullSuiteCall == "" {
				t.Fatalf("full suite test call not found in go record:\n%s", string(goLog))
			}
			if tc.wantTimeout == "" {
				if strings.Contains(fullSuiteCall, "-timeout=") {
					t.Fatalf("expected no -timeout flag when GOFLAGS contains -timeout, got: %s", fullSuiteCall)
				}
			} else {
				if !strings.Contains(fullSuiteCall, tc.wantTimeout) {
					t.Fatalf("expected %s in full suite test call, got: %s", tc.wantTimeout, fullSuiteCall)
				}
			}
		})
	}
}

func extractComposeAppCommand(t *testing.T, composePath string) []string {
	t.Helper()
	content, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("could not read compose.yaml: %v", err)
	}
	for _, line := range strings.Split(string(content), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "command:") {
			rawJSON := strings.TrimSpace(strings.TrimPrefix(trimmed, "command:"))
			var cmd []string
			if err := json.Unmarshal([]byte(rawJSON), &cmd); err != nil {
				t.Fatalf("failed to parse compose command as JSON array: %v", err)
			}
			return cmd
		}
	}
	t.Fatalf("command not found in %s", composePath)
	return nil
}

func extractDockerfileCMD(t *testing.T, dockerfilePath string) []string {
	t.Helper()
	content, err := os.ReadFile(dockerfilePath)
	if err != nil {
		t.Fatalf("could not read Dockerfile: %v", err)
	}
	for _, line := range strings.Split(string(content), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "CMD ") {
			rawJSON := strings.TrimSpace(strings.TrimPrefix(trimmed, "CMD "))
			var cmd []string
			if err := json.Unmarshal([]byte(rawJSON), &cmd); err != nil {
				t.Fatalf("failed to parse CMD as JSON array: %v", err)
			}
			return cmd
		}
	}
	t.Fatalf("CMD not found in %s", dockerfilePath)
	return nil
}

func TestCIWorkflow_RunsWholeSuiteWithoutResultCache(t *testing.T) {
	ciBytes, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	ci := string(ciBytes)

	// Ensure CI partitions all packages dynamically across shards without result-cache replay.
	for _, want := range []string{
		"verify-shard:",
		"shard: [0, 1, 2]",
		`shard_output=$(./scripts/ci_shard.sh "${{ matrix.shard }}" 3)`,
		"go test -timeout=30m -count=1 -coverprofile=coverage.out -covermode=atomic",
		"verify:",
		"needs: [verify-shard]",
		"if: ${{ always() }}",
		"needs.verify-shard.result != 'success'",
		"go tool cover -func=coverage.out",
	} {
		if !strings.Contains(ci, want) {
			t.Fatalf("ci.yml missing required sharded verification directive %q", want)
		}
	}
	if strings.Contains(ci, "< <(./scripts/ci_shard.sh") {
		t.Fatal("ci.yml must not use bash process substitution for ci_shard.sh (loses exit code)")
	}
	if strings.Contains(ci, "if: ${{ !cancelled() }}") {
		t.Fatal("ci.yml verify gate must use always() so cancelled shards never report skipped and pass branch protection")
	}

	// Regression test: verify the actual CI shard shell block aborts before `go test` when ci_shard.sh fails after partial stdout.
	t.Run("shard step aborts on partial ci_shard failure", func(t *testing.T) {
		marker := "        run: |\n"
		stepMarker := "- name: Run shard tests with atomic coverage"
		idx := strings.Index(ci, stepMarker)
		if idx < 0 {
			t.Fatalf("could not locate %q in ci.yml", stepMarker)
		}
		rest := ci[idx:]
		runIdx := strings.Index(rest, marker)
		if runIdx < 0 {
			t.Fatalf("could not locate run block in %q", stepMarker)
		}
		var blockLines []string
		for _, line := range strings.Split(rest[runIdx+len(marker):], "\n") {
			if line != "" && !strings.HasPrefix(line, "          ") {
				break
			}
			blockLines = append(blockLines, strings.TrimPrefix(line, "          "))
		}
		script := strings.ReplaceAll(strings.Join(blockLines, "\n"), "${{ matrix.shard }}", "0")

		fix := setupVerifyFixture(t, false)
		shardScript, err := os.ReadFile("ci_shard.sh")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(fix.root, "scripts", "ci_shard.sh"), shardScript, 0o755); err != nil {
			t.Fatal(err)
		}

		if out, code := runCmd(t, fix.root, fix.env, "bash", "-eo", "pipefail", "-c", script); code != 0 {
			t.Fatalf("happy-path CI shard script failed (code %d):\n%s", code, out)
		}
		goLogOK, err := os.ReadFile(fix.goRecord)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(goLogOK), "go test -timeout=30m -count=1") {
			t.Fatalf("expected happy-path CI shard script to run go test:\n%s", goLogOK)
		}

		if err := os.WriteFile(fix.goRecord, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		failEnv := append([]string{}, fix.env...)
		failEnv = append(failEnv, "GO_LIST_PARTIAL_FAIL=1")
		if out, code := runCmd(t, fix.root, failEnv, "bash", "-eo", "pipefail", "-c", script); code == 0 {
			t.Fatalf("expected CI shard script to abort when ci_shard.sh fails after partial stdout, got:\n%s", out)
		}
		goLogFail, err := os.ReadFile(fix.goRecord)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(goLogFail), "go test ") {
			t.Fatalf("CI shard script must not execute go test after ci_shard.sh fails:\n%s", goLogFail)
		}
	})
}

func TestCIShard_PartitionsAllPackagesWithoutDuplicatesOrOmissions(t *testing.T) {
	allOut, code := runCmd(t, "..", os.Environ(), "go", "list", "./...")
	if code != 0 {
		t.Fatalf("go list ./... failed (code %d):\n%s", code, allOut)
	}
	var expected []string
	for _, line := range strings.Split(strings.TrimSpace(allOut), "\n") {
		if pkg := strings.TrimSpace(line); pkg != "" {
			expected = append(expected, pkg)
		}
	}
	if len(expected) == 0 {
		t.Fatal("expected non-empty repository package list")
	}

	for _, count := range []int{2, 3} {
		t.Run(fmt.Sprintf("%d-shards", count), func(t *testing.T) {
			seen := make(map[string]int, len(expected))
			for shard := 0; shard < count; shard++ {
				out, code := runCmd(t, "..", os.Environ(), "./scripts/ci_shard.sh", fmt.Sprintf("%d", shard), fmt.Sprintf("%d", count))
				if code != 0 {
					t.Fatalf("ci_shard.sh %d %d failed (code %d):\n%s", shard, count, code, out)
				}
				var shardCount int
				for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
					pkg := strings.TrimSpace(line)
					if pkg == "" {
						continue
					}
					shardCount++
					if prev, dup := seen[pkg]; dup {
						t.Fatalf("package %s assigned to both shard %d and shard %d", pkg, prev, shard)
					}
					seen[pkg] = shard
				}
				if shardCount == 0 {
					t.Fatalf("shard %d of %d received zero packages", shard, count)
				}
			}
			for _, pkg := range expected {
				if _, ok := seen[pkg]; !ok {
					t.Fatalf("package %s omitted from %d-shard partition", pkg, count)
				}
			}
			if len(seen) != len(expected) {
				t.Fatalf("partitioned package count = %d, want %d", len(seen), len(expected))
			}
		})
	}

	t.Run("synthetic new and missing packages via stdin", func(t *testing.T) {
		synthetic := strings.Join([]string{
			"github.com/witchcraze/party2re/internal/newpkg",
			"github.com/witchcraze/party2re/internal/casino",
			"github.com/witchcraze/party2re/internal/alpha",
			"github.com/witchcraze/party2re/internal/myscripts",
		}, "\n") + "\n"
		seen := make(map[string]int)
		for shard := 0; shard < 3; shard++ {
			cmd := exec.Command("./scripts/ci_shard.sh", fmt.Sprintf("%d", shard), "3", "--stdin")
			cmd.Dir = ".."
			cmd.Stdin = strings.NewReader(synthetic)
			outBytes, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("ci_shard.sh stdin failed (shard %d): %v\n%s", shard, err, string(outBytes))
			}
			for _, line := range strings.Split(strings.TrimSpace(string(outBytes)), "\n") {
				if pkg := strings.TrimSpace(line); pkg != "" {
					if prev, dup := seen[pkg]; dup {
						t.Fatalf("synthetic package %s duplicated across shards %d and %d", pkg, prev, shard)
					}
					seen[pkg] = shard
				}
			}
		}
		if len(seen) != 4 {
			t.Fatalf("expected all 4 synthetic packages partitioned, got %v", seen)
		}
	})

	t.Run("rejects invalid arguments and partial go list failure", func(t *testing.T) {
		if out, code := runCmd(t, "..", os.Environ(), "./scripts/ci_shard.sh", "3", "3"); code == 0 {
			t.Fatalf("expected out-of-range shard index to fail, got:\n%s", out)
		}
		binDir := t.TempDir()
		fakeGo := filepath.Join(binDir, "go")
		if err := os.WriteFile(fakeGo, []byte("#!/usr/bin/env bash\necho github.com/witchcraze/party2re/internal/sample\nexit 1\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		env := append(os.Environ(), "PATH="+binDir+":"+os.Getenv("PATH"))
		if out, code := runCmd(t, "..", env, "./scripts/ci_shard.sh", "0", "3"); code == 0 {
			t.Fatalf("expected partial go list failure to abort ci_shard.sh, got:\n%s", out)
		}
	})
}

func TestCompose_AppCommandTimeoutConfiguration(t *testing.T) {
	rawCmd := extractComposeAppCommand(t, filepath.Join("..", "compose.yaml"))
	cmdArgs := make([]string, len(rawCmd))
	for i, arg := range rawCmd {
		cmdArgs[i] = strings.ReplaceAll(arg, "$$", "$")
	}

	for _, tc := range []struct {
		name        string
		extraEnv    []string
		wantTimeout string
	}{
		{
			name:        "default 30m timeout",
			wantTimeout: "-timeout=30m",
		},
		{
			name:        "override via TEST_TIMEOUT",
			extraEnv:    []string{"TEST_TIMEOUT=45m"},
			wantTimeout: "-timeout=45m",
		},
		{
			name:        "override via PARTY2_TEST_TIMEOUT",
			extraEnv:    []string{"PARTY2_TEST_TIMEOUT=1h"},
			wantTimeout: "-timeout=1h",
		},
		{
			name:        "preserve GOFLAGS with -timeout",
			extraEnv:    []string{"GOFLAGS=-timeout=20m"},
			wantTimeout: "",
		},
		{
			name:        "preserve GOFLAGS with -test.timeout",
			extraEnv:    []string{"GOFLAGS=-test.timeout=20m"},
			wantTimeout: "",
		},
		{
			name:        "preserve unrelated GOFLAGS with default timeout",
			extraEnv:    []string{"GOFLAGS=-p=2"},
			wantTimeout: "-timeout=30m",
		},
		{
			name:        "explicit TEST_TIMEOUT takes precedence over GOFLAGS",
			extraEnv:    []string{"GOFLAGS=-timeout=20m", "TEST_TIMEOUT=45m"},
			wantTimeout: "-timeout=45m",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fix := setupVerifyFixture(t, false)
			env := append(fix.env, tc.extraEnv...)
			out, code := runCmd(t, fix.root, env, cmdArgs[0], cmdArgs[1:]...)
			if code != 0 {
				t.Fatalf("compose app command failed with code %d:\n%s", code, out)
			}
			goLog, err := os.ReadFile(fix.goRecord)
			if err != nil {
				t.Fatalf("could not read go record: %v", err)
			}
			var testCall string
			for _, line := range strings.Split(string(goLog), "\n") {
				if strings.HasPrefix(line, "go test") && strings.Contains(line, "./...") {
					testCall = line
					break
				}
			}
			if testCall == "" {
				t.Fatalf("go test call not found in go record:\n%s", string(goLog))
			}
			if tc.wantTimeout == "" {
				if strings.Contains(testCall, "-timeout=") {
					t.Fatalf("expected no -timeout flag when GOFLAGS contains timeout override, got: %s", testCall)
				}
			} else {
				if !strings.Contains(testCall, tc.wantTimeout) {
					t.Fatalf("expected %s in test call, got: %s", tc.wantTimeout, testCall)
				}
			}
		})
	}
}

func TestDockerfileDev_CMDTimeoutConfiguration(t *testing.T) {
	cmdArgs := extractDockerfileCMD(t, filepath.Join("..", "Dockerfile.dev"))

	for _, tc := range []struct {
		name        string
		extraEnv    []string
		wantTimeout string
	}{
		{
			name:        "default 30m timeout",
			wantTimeout: "-timeout=30m",
		},
		{
			name:        "override via TEST_TIMEOUT",
			extraEnv:    []string{"TEST_TIMEOUT=45m"},
			wantTimeout: "-timeout=45m",
		},
		{
			name:        "override via PARTY2_TEST_TIMEOUT",
			extraEnv:    []string{"PARTY2_TEST_TIMEOUT=1h"},
			wantTimeout: "-timeout=1h",
		},
		{
			name:        "preserve GOFLAGS with -timeout",
			extraEnv:    []string{"GOFLAGS=-timeout=20m"},
			wantTimeout: "",
		},
		{
			name:        "preserve GOFLAGS with -test.timeout",
			extraEnv:    []string{"GOFLAGS=-test.timeout=20m"},
			wantTimeout: "",
		},
		{
			name:        "preserve unrelated GOFLAGS with default timeout",
			extraEnv:    []string{"GOFLAGS=-p=2"},
			wantTimeout: "-timeout=30m",
		},
		{
			name:        "explicit TEST_TIMEOUT takes precedence over GOFLAGS",
			extraEnv:    []string{"GOFLAGS=-timeout=20m", "TEST_TIMEOUT=45m"},
			wantTimeout: "-timeout=45m",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fix := setupVerifyFixture(t, false)
			env := append(fix.env, tc.extraEnv...)
			out, code := runCmd(t, fix.root, env, cmdArgs[0], cmdArgs[1:]...)
			if code != 0 {
				t.Fatalf("Dockerfile.dev CMD failed with code %d:\n%s", code, out)
			}
			goLog, err := os.ReadFile(fix.goRecord)
			if err != nil {
				t.Fatalf("could not read go record: %v", err)
			}
			var testCall string
			for _, line := range strings.Split(string(goLog), "\n") {
				if strings.HasPrefix(line, "go test") && strings.Contains(line, "./...") {
					testCall = line
					break
				}
			}
			if testCall == "" {
				t.Fatalf("go test call not found in go record:\n%s", string(goLog))
			}
			if tc.wantTimeout == "" {
				if strings.Contains(testCall, "-timeout=") {
					t.Fatalf("expected no -timeout flag when GOFLAGS contains timeout override, got: %s", testCall)
				}
			} else {
				if !strings.Contains(testCall, tc.wantTimeout) {
					t.Fatalf("expected %s in test call, got: %s", tc.wantTimeout, testCall)
				}
			}
		})
	}
}

func TestVerify_DeduplicatesArchitectureAndOpenAPITests(t *testing.T) {
	fix := setupVerifyFixture(t, false)
	out, code := runCmd(t, fix.root, fix.env, "bash", "./scripts/verify.sh")
	if code != 0 {
		t.Fatalf("verify failed with code %d:\n%s", code, out)
	}
	goLog, err := os.ReadFile(fix.goRecord)
	if err != nil {
		t.Fatalf("could not read go record: %v", err)
	}

	var openAPICall, archCall, suiteCall string
	archTestCount := 0
	for _, line := range strings.Split(string(goLog), "\n") {
		if !strings.HasPrefix(line, "go test ") {
			continue
		}
		if strings.Contains(line, "-run OpenAPI") {
			openAPICall = line
		}
		if strings.Contains(line, "internal/architecture") {
			archTestCount++
			archCall = line
		}
		if strings.Contains(line, "github.com/witchcraze/party2re/internal/sample") {
			suiteCall = line
		}
	}

	if openAPICall == "" {
		t.Fatalf("expected OpenAPI test call in go record:\n%s", goLog)
	}
	if strings.Contains(openAPICall, "internal/architecture") {
		t.Fatalf("Step 3 OpenAPI check must not redundantly execute internal/architecture: %s", openAPICall)
	}
	if archCall == "" || archTestCount != 1 {
		t.Fatalf("expected internal/architecture to execute exactly once, got count=%d:\n%s", archTestCount, goLog)
	}
	if suiteCall == "" {
		t.Fatalf("expected Step 6 suite call in go record:\n%s", goLog)
	}
	if strings.Contains(suiteCall, "internal/architecture") {
		t.Fatalf("Step 6 suite call must not re-run internal/architecture: %s", suiteCall)
	}
}

func TestVerify_PartialGoListFailureAbortsStep6(t *testing.T) {
	t.Run("host path", func(t *testing.T) {
		fix := setupVerifyFixture(t, false)
		env := append(fix.env, "GO_LIST_PARTIAL_FAIL=1")
		out, code := runCmd(t, fix.root, env, "bash", "./scripts/verify.sh")
		if code == 0 {
			t.Fatalf("expected verify.sh to fail on partial go list failure, output:\n%s", out)
		}
		goLog, err := os.ReadFile(fix.goRecord)
		if err != nil {
			t.Fatalf("could not read go record: %v", err)
		}
		if strings.Contains(string(goLog), "github.com/witchcraze/party2re/internal/sample") {
			t.Fatalf("Step 6 go test must not execute after go list fails:\n%s", goLog)
		}
	})

	t.Run("docker fallback path", func(t *testing.T) {
		fix := setupVerifyFixture(t, false)
		noGoDir := t.TempDir()
		for _, tool := range []string{"awk", "bash", "cat", "dirname", "find", "git", "gofmt", "grep", "mkdir", "mktemp", "rm", "sh", "sha256sum", "tr"} {
			if p, err := exec.LookPath(tool); err == nil {
				if err := os.Symlink(p, filepath.Join(noGoDir, tool)); err != nil {
					t.Fatal(err)
				}
			}
		}
		if err := os.Symlink(filepath.Join(fix.binDir, "docker"), filepath.Join(noGoDir, "docker")); err != nil {
			t.Fatal(err)
		}

		// Verify docker fallback succeeds when go list exits 0.
		okEnv := append([]string{}, fix.env...)
		okEnv = append(okEnv,
			"PATH="+noGoDir,
			"CONTAINER_PATH="+fix.binDir+":"+os.Getenv("PATH"),
			"DOCKER_EXEC_APP_SH=1",
		)
		if out, code := runCmd(t, fix.root, okEnv, "bash", "./scripts/verify.sh"); code != 0 {
			t.Fatalf("expected docker fallback happy path to pass (code %d):\n%s", code, out)
		}
		goLogOK, err := os.ReadFile(fix.goRecord)
		if err != nil {
			t.Fatalf("could not read go record: %v", err)
		}
		if !strings.Contains(string(goLogOK), "go test -timeout=30m -count=1 github.com/witchcraze/party2re/internal/sample") {
			t.Fatalf("expected docker fallback Step 6 to run filtered suite:\n%s", goLogOK)
		}

		// Verify partial stdout + non-zero exit from go list aborts Step 6 under sh without pipefail.
		if err := os.WriteFile(fix.goRecord, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		failEnv := append(okEnv, "GO_LIST_PARTIAL_FAIL=1")
		out, code := runCmd(t, fix.root, failEnv, "bash", "./scripts/verify.sh")
		if code == 0 {
			t.Fatalf("expected docker fallback to abort when go list exits non-zero after partial output:\n%s", out)
		}
		goLogFail, err := os.ReadFile(fix.goRecord)
		if err != nil {
			t.Fatalf("could not read go record: %v", err)
		}
		if strings.Contains(string(goLogFail), "github.com/witchcraze/party2re/internal/sample") {
			t.Fatalf("docker fallback Step 6 must not run go test after go list fails:\n%s", goLogFail)
		}
	})
}
