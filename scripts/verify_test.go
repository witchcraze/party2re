package scripts_test

import (
	"bytes"
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
