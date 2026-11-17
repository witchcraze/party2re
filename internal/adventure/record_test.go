package adventure

import (
	"reflect"
	"testing"
)

func TestRecordUnlockThresholdsAndIdempotence(t *testing.T) {
	for _, tc := range []struct {
		total int
		want  []string
	}{
		{0, []string{}}, {49, []string{}}, {50, []string{"try"}}, {99, []string{"try"}},
		{100, []string{"try", "image"}}, {149, []string{"try", "image"}},
		{150, []string{"try", "calm", "image"}}, {299, []string{"try", "calm", "image"}},
		{300, []string{"try", "calm", "hard", "image"}}, {499, []string{"try", "calm", "hard", "image"}},
		{500, []string{"try", "calm", "hard", "image", "avatar"}},
		{1000, []string{"try", "calm", "hard", "image", "avatar"}},
	} {
		u := RecordUnlocks{}
		if got := u.unlock(tc.total); !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("total %d: %v, want %v", tc.total, got, tc.want)
		}
		saved := u
		if got := u.unlock(tc.total); len(got) != 0 || u != saved {
			t.Fatalf("total %d repeated: %v %+v", tc.total, got, u)
		}
		if got := u.unlock(0); len(got) != 0 || u != saved {
			t.Fatal("lower clear count revoked unlocks")
		}
	}
	u := RecordUnlocks{Try: true, Image: true}
	if got := u.unlock(500); !reflect.DeepEqual(got, []string{"calm", "hard", "avatar"}) {
		t.Fatalf("previous unlocks repeated: %v", got)
	}
}
