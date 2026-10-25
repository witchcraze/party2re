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
		{name: "test failure", env: []string{"TEST_EXIT=1"}, wantError: true},
		{name: "no matching tests", env: []string{"TEST_OUTPUT=PASS"}, wantError: true},
		{name: "all skipped", env: []string{"TEST_OUTPUT=--- SKIP: TestFeature (0.00s)"}, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			bin := filepath.Join(root, "scripts")
			if err := os.Mkdir(bin, 0o755); err != nil {
				t.Fatal(err)
			}
			files := map[string]string{
				"stress_test.sh": string(script),
				"migrate.sh":     "#!/bin/bash\nexit \"${MIGRATE_EXIT:-0}\"\n",
				"docker":         "#!/bin/bash\nexit \"${SERVICE_EXIT:-0}\"\n",
				"go": `#!/bin/bash
printf '%s\n' "$@" > "$STRESS_CALL"
printf '%s\n' "$PARTY2_DB_DSN" "$PARTY2_TEST_DB_ADMIN_DSN" "$PARTY2_VALKEY_ADDR" "$PARTY2_STRESS_ENABLED" >> "$STRESS_CALL"
printf '%s\n' "${TEST_OUTPUT:---- PASS: TestFeature (0.00s)}"
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
			cmd.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"), "STRESS_CALL="+call,
				"PARTY2_DB_DSN=", "PARTY2_TEST_DB_ADMIN_DSN=", "PARTY2_VALKEY_ADDR=", "PARTY2_STRESS_ENABLED=0")
			cmd.Env = append(cmd.Env, tc.env...)
			output, err := cmd.CombinedOutput()
			if (err != nil) != tc.wantError {
				t.Fatalf("error = %v, wantError = %v\n%s", err, tc.wantError, output)
			}
			if tc.wantError {
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
			want := append([]string{"test", "-v", "-count=1"}, args...)
			want = append(want, connections...)
			want = append(want, "1")
			if string(invocation) != strings.Join(want, "\n")+"\n" {
				t.Fatalf("invocation:\n%s\nwant:\n%s", invocation, strings.Join(want, "\n"))
			}
		})
	}
}
