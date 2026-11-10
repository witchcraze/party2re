package economy

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"
)

func TestLockInOrder(t *testing.T) {
	for _, ids := range [][]string{nil, {}, {"b", "a", "b", "", " a", "a"}} {
		original := slices.Clone(ids)
		var got []string
		ctx := context.WithValue(context.Background(), struct{}{}, "transaction")
		err := LockInOrder(ctx, ids, func(lockCtx context.Context, id string) error {
			if lockCtx != ctx {
				t.Fatal("lost transaction context")
			}
			got = append(got, id)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		var want []string
		if len(ids) != 0 {
			want = []string{"", " a", "a", "b"}
		}
		if !slices.Equal(got, want) || !reflect.DeepEqual(ids, original) {
			t.Fatalf("locks = %q, want %q; input = %q, originally %q", got, want, ids, original)
		}
	}
}

func TestLockInOrderNotFoundPolicy(t *testing.T) {
	notFound := errors.New("not found")
	for _, skipMissing := range []bool{false, true} {
		var got []string
		err := LockInOrder(context.Background(), []string{"c", "b", "a"}, func(_ context.Context, id string) error {
			got = append(got, id)
			var err error
			if id == "b" {
				err = notFound
			}
			if skipMissing && errors.Is(err, notFound) {
				return nil
			}
			return err
		})
		want := []string{"a", "b"}
		if skipMissing {
			want = append(want, "c")
			if err != nil {
				t.Fatal(err)
			}
		} else if !errors.Is(err, notFound) {
			t.Fatalf("error = %v, want not found", err)
		}
		if !slices.Equal(got, want) {
			t.Fatalf("locks = %q, want %q", got, want)
		}
	}
}

func TestLockInOrderCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var got []string
	err := LockInOrder(ctx, []string{"b", "a"}, func(_ context.Context, id string) error {
		got = append(got, id)
		cancel()
		return nil
	})
	if !errors.Is(err, context.Canceled) || !slices.Equal(got, []string{"a"}) {
		t.Fatalf("locks = %q, error = %v", got, err)
	}
}
