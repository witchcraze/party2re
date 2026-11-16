package architecture_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPresentationFieldClassification(t *testing.T) {
	for _, tc := range []struct {
		name, fields string
		whitelist    map[string]string
		want         string
	}{
		{name: "new message", fields: "Message string `json:\"message\"`", want: "sample.Result.Message"},
		{name: "optional message", fields: "Text string `json:\"message,omitempty\"`", want: "sample.Result.Text"},
		{name: "untagged message", fields: "Message string", want: "sample.Result.Message"},
		{name: "nested anonymous message", fields: "Nested struct { Message string }", want: "sample.(anonymous).Message"},
		{name: "authored data", fields: "//lint:ignore presentation player-authored text\nMessage string"},
		{name: "persisted data inline", fields: "Message string //lint:ignore presentation persisted notice body"},
		{name: "missing reason", fields: "//lint:ignore presentation\nMessage string", want: "sample.Result.Message"},
		{name: "empty reason", fields: "//lint:ignore presentation   \nMessage string", want: "sample.Result.Message"},
		{name: "different directive", fields: "//lint:ignore error-swallow persisted data\nMessage string", want: "sample.Result.Message"},
		{name: "directive on other field", fields: "//lint:ignore presentation player-authored name\nName string\nMessage string", want: "sample.Result.Message"},
		{name: "legacy field", fields: "Message string", whitelist: map[string]string{"sample.Result.Message": "#2293"}},
		{name: "stale legacy field", fields: "Count int", whitelist: map[string]string{"sample.Result.Message": "#2293"}, want: "remove it"},
		{name: "converted legacy field", fields: "//lint:ignore presentation stored text\nMessage string", whitelist: map[string]string{"sample.Result.Message": "#2293"}, want: "remove it"},
		{name: "dialogue still prohibited", fields: "Dialogue string", want: "sample.Result.Dialogue"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			for _, dir := range []string{"sample", "api/http", "testutil", "architecture"} {
				path := filepath.Join(root, "internal", dir)
				if err := os.MkdirAll(path, 0o755); err != nil {
					t.Fatal(err)
				}
				source := "package sample\ntype Result struct {\n" + tc.fields + "\n}\n"
				if err := os.WriteFile(filepath.Join(path, "model.go"), []byte(source), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(path, "model_test.go"), []byte("package sample\ntype TestOnly struct { Message string }"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			violations, err := presentationFieldViolations(root, tc.whitelist)
			if err != nil {
				t.Fatal(err)
			}
			if tc.want == "" {
				if len(violations) != 0 {
					t.Fatalf("unexpected violations: %v", violations)
				}
			} else if len(violations) != 1 || !strings.Contains(violations[0], tc.want) {
				t.Fatalf("want one violation containing %q, got %v", tc.want, violations)
			}
		})
	}
}
