package http

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/witchcraze/party2re/internal/adventure"
	"github.com/witchcraze/party2re/internal/playercontext"
)

func TestAdventureRecordEntryOnlyAffectsOwner(t *testing.T) {
	for _, owner := range []string{"", "hero", "other"} {
		t.Run("subject="+owner, func(t *testing.T) {
			f, store, act, get := homeProfileRouter(t)
			if owner != "" {
				store.selection.Subject = playercontext.Subject{Kind: "home", ID: owner}
			}
			f.recordEntryResult = adventure.RecordEntryResult{TotalClears: 100, Unlocks: adventure.RecordUnlocks{Try: true, Image: true}, NewlyUnlocked: []string{"try", "image"}}
			status, raw := act("scene_enter", `{"destination":"home_adventure_record"}`)
			if status != 200 {
				t.Fatalf("entry: %d %s", status, raw)
			}
			var result sceneEnterResult
			if err := json.Unmarshal(raw["result"], &result); err != nil {
				t.Fatal(err)
			}
			wantCalls := 1
			if owner == "other" {
				wantCalls = 0
				if result.AdventureRecord != nil {
					t.Fatal("visitor received private unlocks")
				}
			} else if !reflect.DeepEqual(result.AdventureRecord, &f.recordEntryResult) {
				t.Fatalf("lost unlock messages: %+v", result.AdventureRecord)
			}
			if f.recordEntries != wantCalls {
				t.Fatalf("entry calls: %d", f.recordEntries)
			}
			if status, _ := get(); status != 200 {
				t.Fatal("record GET failed")
			}
			if status, _ := act("scene_back", `{}`); status != 200 {
				t.Fatal("back failed")
			}
			// Restoring a saved scene must never perform entry effects.
			store.selection.Destination = "home_adventure_record"
			if status, _ := get(); status != 200 || f.recordEntries != wantCalls {
				t.Fatal("GET/back replayed unlocks")
			}
		})
	}
}

func TestAdventureRecordEntryFailureAndRecovery(t *testing.T) {
	for _, failure := range []string{"selection", "unlock", "ownership", "refresh"} {
		t.Run(failure, func(t *testing.T) {
			f, store, act, get := homeProfileRouter(t)
			switch failure {
			case "selection":
				store.saveErr = errors.New("selection failure")
			case "unlock":
				f.recordEntryErr = errors.New("private storage failure")
			case "ownership":
				f.recordEntryErr = adventure.ErrRecordForbidden
			case "refresh":
				f.recordEntryResult = adventure.RecordEntryResult{NewlyUnlocked: []string{"try"}}
				f.afterRecordEntry = func() { f.recordFail = "hero" }
			}
			status, raw := act("scene_enter", `{"destination":"home_adventure_record"}`)
			wantCalls := 1
			if failure == "selection" {
				wantCalls = 0
			}
			if f.recordEntries != wantCalls {
				t.Fatalf("entry calls: %d", f.recordEntries)
			}
			if failure == "refresh" {
				if status != 200 || string(raw["success"]) != "true" || string(raw["context"]) != "null" {
					t.Fatalf("committed outcome lost: %d %s", status, raw)
				}
				var result sceneEnterResult
				if err := json.Unmarshal(raw["result"], &result); err != nil || result.AdventureRecord == nil || len(result.AdventureRecord.NewlyUnlocked) != 1 {
					t.Fatalf("unlock result lost: %s %v", raw["result"], err)
				}
			} else if failure == "ownership" {
				if status != 403 {
					t.Fatalf("ownership failure: %d %s", status, raw)
				}
			} else if status != 500 {
				t.Fatalf("failure hidden: %d %s", status, raw)
			}
			f.recordFail, f.recordEntryErr = "", nil
			if status, _ := get(); status != 200 || f.recordEntries != wantCalls {
				t.Fatal("GET recovery reran entry")
			}
		})
	}
}
