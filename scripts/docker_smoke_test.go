package scripts_test

import (
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type dockerSmokeFixture struct {
	root         string
	binDir       string
	dockerRecord string
	curlRecord   string
	env          []string
}

func setupDockerSmokeFixture(t *testing.T) *dockerSmokeFixture {
	t.Helper()
	root := t.TempDir()
	scriptsDir := filepath.Join(root, "scripts")
	if err := os.MkdirAll(scriptsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	smokeScript, err := os.ReadFile("docker_smoke.sh")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(scriptsDir, "docker_smoke.sh"), smokeScript, 0o755); err != nil {
		t.Fatal(err)
	}

	migrateScript := "#!/usr/bin/env bash\nexit \"${MIGRATE_EXIT:-0}\"\n"
	if err := os.WriteFile(filepath.Join(scriptsDir, "migrate.sh"), []byte(migrateScript), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "compose.test.yaml"), []byte("name: party2re-test\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}

	logDir := t.TempDir()
	dockerRecord := filepath.Join(logDir, "docker.log")
	curlRecord := filepath.Join(logDir, "curl.log")

	mockDocker := `#!/usr/bin/env bash
if [ -n "${DOCKER_RECORD:-}" ]; then
    echo "docker $*" >> "$DOCKER_RECORD"
fi
case "${1:-}" in
    image)
        if [ "${2:-}" = "inspect" ]; then
            exit "${DOCKER_IMAGE_INSPECT_EXIT:-0}"
        fi
        ;;
    compose)
        if [ "${2:-}" = "ps" ]; then
            echo "mariadb-cid-123"
            exit 0
        fi
        exit "${DOCKER_COMPOSE_EXIT:-0}"
        ;;
    container)
        if [ "${2:-}" = "inspect" ]; then
            exit "${DOCKER_CONTAINER_EXISTS:-0}"
        fi
        ;;
    inspect)
        case "${3:-}" in
            *NetworkSettings.Networks*)
                echo "party2re-test_default"
                ;;
            *State.Running*)
                echo "${DOCKER_CONTAINER_RUNNING:-true}"
                ;;
        esac
        exit 0
        ;;
    run)
        exit "${DOCKER_RUN_EXIT:-0}"
        ;;
    port)
        echo "${DOCKER_PORT_ADDR:-127.0.0.1:49152}"
        exit 0
        ;;
    logs)
        echo "${DOCKER_CONTAINER_LOGS:-mock container log line}"
        exit 0
        ;;
    rm)
        exit 0
        ;;
esac
exit 0
`
	if err := os.WriteFile(filepath.Join(binDir, "docker"), []byte(mockDocker), 0o755); err != nil {
		t.Fatal(err)
	}

	mockCurl := `#!/usr/bin/env bash
if [ -n "${CURL_RECORD:-}" ]; then
    echo "curl $*" >> "$CURL_RECORD"
fi
out_file=""
method="GET"
url=""
while [ "$#" -gt 0 ]; do
    case "$1" in
        -o)
            out_file="$2"
            shift 2
            ;;
        -X)
            method="$2"
            shift 2
            ;;
        -w|-H|-d|--connect-timeout|--max-time)
            shift 2
            ;;
        -sS|-fsS)
            shift
            ;;
        *)
            url="$1"
            shift
            ;;
    esac
done
path="${url#http://127.0.0.1:49152}"
write_resp() {
    local code="$1" body="$2"
    if [ -n "$out_file" ]; then
        printf '%s' "$body" > "$out_file"
    fi
    printf '%s' "$code"
}
case "$method $path" in
    "GET /health")
        write_resp "200" '{"status":"ok"}'
        ;;
    "POST /players")
        write_resp "201" '{"id":"player-1"}'
        ;;
    "POST /sessions")
        write_resp "201" '{"id":"session-token-1","player_id":"player-1"}'
        ;;
    "POST /characters")
        write_resp "201" '{"id":"char-1","player_id":"player-1"}'
        ;;
    "GET /api/v1/characters/char-1/context")
        if [ "${CURL_CONTEXT_STATUS:-200}" != "200" ]; then
            write_resp "${CURL_CONTEXT_STATUS}" '{"error":"context failed"}'
            exit 0
        fi
        write_resp "200" '{"character":{"id":"char-1"},"scene":{"location_id":"town"}}'
        ;;
    "DELETE /players/me")
        write_resp "200" '{"deleted":true}'
        ;;
    *)
        write_resp "404" '{"error":"not found"}'
        ;;
esac
`
	if err := os.WriteFile(filepath.Join(binDir, "curl"), []byte(mockCurl), 0o755); err != nil {
		t.Fatal(err)
	}

	env := []string{
		"PATH=" + binDir + ":" + os.Getenv("PATH"),
		"DOCKER_RECORD=" + dockerRecord,
		"CURL_RECORD=" + curlRecord,
		"PARTY2_SMOKE_HEALTH_TIMEOUT_SECONDS=2",
	}

	return &dockerSmokeFixture{
		root:         root,
		binDir:       binDir,
		dockerRecord: dockerRecord,
		curlRecord:   curlRecord,
		env:          env,
	}
}

func TestDockerSmoke_HappyPath(t *testing.T) {
	fix := setupDockerSmokeFixture(t)
	out, code := runCmd(t, fix.root, fix.env, "bash", "./scripts/docker_smoke.sh", "party2re:ci-smoke")
	if code != 0 {
		t.Fatalf("docker_smoke.sh failed with code %d:\n%s", code, out)
	}
	if !strings.Contains(out, "Production image smoke test passed.") {
		t.Fatalf("expected pass banner, got:\n%s", out)
	}

	dockerLogBytes, err := os.ReadFile(fix.dockerRecord)
	if err != nil {
		t.Fatal(err)
	}
	dockerLog := string(dockerLogBytes)
	for _, want := range []string{
		"docker image inspect party2re:ci-smoke",
		"docker compose up -d --wait mariadb valkey",
		"docker run -d",
		"-e PARTY2_DB_DSN=party2:party2@tcp(mariadb:3306)/party2?parseTime=true",
		"-e PARTY2_VALKEY_ADDR=valkey:6379",
		"-e PARTY2_MAX_PLAYERS=0",
		"party2re:ci-smoke",
		"docker rm -f party2-smoke-",
	} {
		if !strings.Contains(dockerLog, want) {
			t.Fatalf("docker log missing %q:\n%s", want, dockerLog)
		}
	}

	curlLogBytes, err := os.ReadFile(fix.curlRecord)
	if err != nil {
		t.Fatal(err)
	}
	curlLines := strings.Split(strings.TrimSpace(string(curlLogBytes)), "\n")
	for _, want := range []struct {
		method string
		url    string
	}{
		{method: "POST", url: "http://127.0.0.1:49152/players"},
		{method: "POST", url: "http://127.0.0.1:49152/sessions"},
		{method: "POST", url: "http://127.0.0.1:49152/characters"},
		{method: "GET", url: "http://127.0.0.1:49152/api/v1/characters/char-1/context"},
		{method: "DELETE", url: "http://127.0.0.1:49152/players/me"},
	} {
		found := false
		for _, line := range curlLines {
			if strings.Contains(line, "-X "+want.method+" ") && strings.HasSuffix(line, " "+want.url) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("curl log missing %s %s:\n%s", want.method, want.url, string(curlLogBytes))
		}
	}
	for _, line := range curlLines {
		if !strings.Contains(line, "--max-time ") || !strings.Contains(line, "--connect-timeout ") {
			t.Fatalf("curl invocation missing timeout flags: %s", line)
		}
	}
}

func TestDockerSmoke_EarlyExitPrintsContainerLogsAndCleansUp(t *testing.T) {
	fix := setupDockerSmokeFixture(t)
	env := append([]string{}, fix.env...)
	env = append(env,
		"DOCKER_CONTAINER_RUNNING=false",
		"DOCKER_CONTAINER_LOGS=fatal: bad entrypoint or startup failure",
	)
	out, code := runCmd(t, fix.root, env, "bash", "./scripts/docker_smoke.sh", "party2re:broken")
	if code == 0 {
		t.Fatalf("expected non-zero exit when container exits early:\n%s", out)
	}
	if !strings.Contains(out, "fatal: bad entrypoint or startup failure") {
		t.Fatalf("expected container logs in failure output, got:\n%s", out)
	}
	dockerLogBytes, err := os.ReadFile(fix.dockerRecord)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(dockerLogBytes), "docker rm -f party2-smoke-") {
		t.Fatalf("expected container cleanup on failure:\n%s", dockerLogBytes)
	}
}

func TestDockerSmoke_HTTPFailurePrintsContainerLogsAndCleansUp(t *testing.T) {
	fix := setupDockerSmokeFixture(t)
	env := append([]string{}, fix.env...)
	env = append(env,
		"CURL_CONTEXT_STATUS=500",
		"DOCKER_CONTAINER_LOGS=error: simulated context handler failure",
	)
	out, code := runCmd(t, fix.root, env, "bash", "./scripts/docker_smoke.sh", "party2re:ci-smoke")
	if code == 0 {
		t.Fatalf("expected non-zero exit when GET context fails:\n%s", out)
	}
	if !strings.Contains(out, "expected HTTP 200, got 500") {
		t.Fatalf("expected HTTP failure message, got:\n%s", out)
	}
	if !strings.Contains(out, "error: simulated context handler failure") {
		t.Fatalf("expected container logs in failure output, got:\n%s", out)
	}
}

func TestDockerSmoke_HangingHealthTimesOutWithRealCurl(t *testing.T) {
	realCurl, err := exec.LookPath("curl")
	if err != nil {
		t.Skip("curl not installed")
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			wg.Add(1)
			go func(c net.Conn) {
				defer wg.Done()
				defer c.Close()
				<-done
			}(conn)
		}
	}()
	t.Cleanup(func() {
		close(done)
		_ = ln.Close()
		wg.Wait()
	})

	fix := setupDockerSmokeFixture(t)
	if err := os.Remove(filepath.Join(fix.binDir, "curl")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realCurl, filepath.Join(fix.binDir, "curl")); err != nil {
		t.Fatal(err)
	}

	env := append([]string{}, fix.env...)
	env = append(env,
		"DOCKER_PORT_ADDR="+ln.Addr().String(),
		"DOCKER_CONTAINER_LOGS=server hung before responding to /health",
		"PARTY2_SMOKE_HEALTH_TIMEOUT_SECONDS=2",
	)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "./scripts/docker_smoke.sh", "party2re:hung")
	cmd.Dir = fix.root
	cmd.Env = env
	outBytes, err := cmd.CombinedOutput()
	out := string(outBytes)

	if ctx.Err() != nil {
		t.Fatalf("docker_smoke.sh did not terminate within dead-man bound: %v\n%s", ctx.Err(), out)
	}
	if err == nil {
		t.Fatalf("expected non-zero exit when /health hangs:\n%s", out)
	}
	if !strings.Contains(out, "timed out after 2s waiting for /health") {
		t.Fatalf("expected timeout message in output, got:\n%s", out)
	}
	if !strings.Contains(out, "server hung before responding to /health") {
		t.Fatalf("expected container logs in output, got:\n%s", out)
	}
	dockerLogBytes, err := os.ReadFile(fix.dockerRecord)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(dockerLogBytes), "docker rm -f party2-smoke-") {
		t.Fatalf("expected container cleanup on timeout:\n%s", dockerLogBytes)
	}
}

func TestCIWorkflow_DockerSmokeRunsBuiltImage(t *testing.T) {
	ciBytes, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	ci := string(ciBytes)
	for _, want := range []string{
		"docker-smoke:",
		"docker build -f Dockerfile -t party2re:ci-smoke .",
		"./scripts/docker_smoke.sh party2re:ci-smoke",
	} {
		if !strings.Contains(ci, want) {
			t.Fatalf("ci.yml missing %q", want)
		}
	}
}
