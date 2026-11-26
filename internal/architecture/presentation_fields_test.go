package architecture_test

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPresentationFieldClassification(t *testing.T) {
	for _, tc := range []struct {
		name, fields, want string
	}{
		{name: "new message", fields: "Message string `json:\"message\"`", want: "sample.Result.Message"},
		{name: "optional message", fields: "Text string `json:\"message,omitempty\"`", want: "sample.Result.Text"},
		{name: "untagged message", fields: "Message string", want: "sample.Result.Message"},
		{name: "embedded message", fields: "Message `json:\"message\"`", want: "sample.Result.Message"},
		{name: "embedded pointer", fields: "*Message", want: "sample.Result.Message"},
		{name: "embedded qualified type", fields: "*external.Text `json:\"message,omitempty\"`", want: "sample.Result.Text"},
		{name: "embedded generic type", fields: "external.Text[string] `json:\"message\"`", want: "sample.Result.Text"},
		{name: "embedded generic pointer", fields: "*external.Text[string, int] `json:\"message\"`", want: "sample.Result.Text"},
		{name: "embedded data", fields: "//lint:ignore presentation persisted notice body\nMessage `json:\"message\"`"},
		{name: "nested anonymous message", fields: "Nested struct { Message string }", want: "sample.(anonymous).Message"},
		{name: "authored data", fields: "//lint:ignore presentation player-authored text\nMessage string"},
		{name: "persisted data inline", fields: "Message string //lint:ignore presentation persisted notice body"},
		{name: "missing reason", fields: "//lint:ignore presentation\nMessage string", want: "sample.Result.Message"},
		{name: "empty reason", fields: "//lint:ignore presentation   \nMessage string", want: "sample.Result.Message"},
		{name: "different directive", fields: "//lint:ignore error-swallow persisted data\nMessage string", want: "sample.Result.Message"},
		{name: "directive on other field", fields: "//lint:ignore presentation player-authored name\nName string\nMessage string", want: "sample.Result.Message"},
		{name: "dialogue still prohibited", fields: "Dialogue string", want: "sample.Result.Dialogue"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			for _, dir := range []string{"sample", "api/http", "testutil", "architecture"} {
				path := filepath.Join(root, "internal", dir)
				if err := os.MkdirAll(path, 0o755); err != nil {
					t.Fatal(err)
				}
				source := "package sample\ntype Message string\ntype Result struct {\n" + tc.fields + "\n}\n"
				if err := os.WriteFile(filepath.Join(path, "model.go"), []byte(source), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(path, "model_test.go"), []byte("package sample\ntype TestOnly struct { Message string }"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			violations, err := presentationFieldViolations(root)
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

func TestEmbeddedMessageJSONAndAST(t *testing.T) {
	type Message string
	result := struct {
		Message `json:"message"`
	}{Message: "hello"}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `{"message":"hello"}` {
		t.Fatalf("embedded message JSON = %s", encoded)
	}
	source := "package sample\ntype Message string\ntype Result struct { Message `json:\"message\"` }"
	file, err := parser.ParseFile(token.NewFileSet(), "model.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	resultType := file.Decls[1].(*ast.GenDecl).Specs[0].(*ast.TypeSpec).Type.(*ast.StructType)
	field := resultType.Fields.List[0]
	if len(field.Names) != 0 || field.Type.(*ast.Ident).Name != "Message" {
		t.Fatalf("unexpected embedded field AST: %+v", field)
	}
}
