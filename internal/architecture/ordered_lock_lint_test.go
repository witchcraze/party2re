package architecture_test

import (
	"go/parser"
	"go/token"
	"testing"
)

func TestLockHierarchyOrderedCallbacks(t *testing.T) {
	for _, tt := range []struct {
		name string
		code string
		want int
	}{
		{"valid", `economy.LockInOrder(ctx, ids, func(ctx context.Context, id string) error {
			_, err := s.characters.FindByIDForUpdate(ctx, id); return err
		}); s.inventories.FindByCharacterIDForUpdate(ctx, id)`, 0},
		{"previous rank", `s.inventories.FindByCharacterIDForUpdate(ctx, id)
		 economy.LockInOrder(ctx, ids, func(ctx context.Context, id string) error {
			_, err := s.characters.FindByIDForUpdate(ctx, id); return err
		})`, 1},
		{"following rank", `economy.LockInOrder(ctx, ids, func(ctx context.Context, id string) error {
			_, _, err := s.guilds.GetGuildForUpdate(ctx, id); return err
		}); s.characters.FindByIDForUpdate(ctx, id)`, 1},
		{"two callbacks", `economy.LockInOrder(ctx, ids, func(ctx context.Context, id string) error {
			_, _, err := s.guilds.GetGuildForUpdate(ctx, id); return err
		}); economy.LockInOrder(ctx, ids, func(ctx context.Context, id string) error {
			_, err := s.characters.FindByIDForUpdate(ctx, id); return err
		})`, 1},
		{"method value", `s.guilds.GetGuildForUpdate(ctx, id)
		 economy.LockInOrder(ctx, ids, s.repo.LockCharacterForUpdate)`, 1},
		{"quest callback", `economy.LockInOrder(ctx, ids, func(ctx context.Context, id string) error {
			_, err := s.quests.FindByIDForUpdate(ctx, id); return err
		}); s.characters.FindByIDForUpdate(ctx, id)`, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "service.go", "package helperquest; func action() {"+tt.code+"}", 0)
			if err != nil {
				t.Fatal(err)
			}
			violations := analyzeASTFileLocks(fset, file, "internal/helperquest/service.go")
			if len(violations) != tt.want {
				t.Fatalf("violations = %v, want %d", violations, tt.want)
			}
		})
	}
}
