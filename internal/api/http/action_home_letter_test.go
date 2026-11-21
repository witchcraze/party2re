package http

import (
	"context"
	"slices"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/home"
	"github.com/witchcraze/party2re/internal/playercontext"
	"github.com/witchcraze/party2re/internal/validation"
)

func (f *homeSceneFixture) SendLetter(_ context.Context, actor, recipient, content string) (home.Letter, error) {
	return home.Letter{ID: "sent", RecipientCharacterID: recipient, Content: content}, f.record("send " + actor + " " + recipient + " " + content)
}
func (f *homeSceneFixture) ReadLetter(_ context.Context, letterID, actor string) error {
	return f.record("read " + actor + " " + letterID)
}
func (f *homeSceneFixture) DeleteLetter(_ context.Context, letterID, actor string) error {
	return f.record("delete " + actor + " " + letterID)
}

type letterNames map[string]string

func (n letterNames) FindByName(_ context.Context, name string) (corecharacter.Character, error) {
	if id, ok := n[name]; ok {
		return corecharacter.Character{ID: id, Name: name}, nil
	}
	return corecharacter.Character{}, corecharacter.ErrNotFound
}

func TestHomeLetterCommandsAreOfferedOnlyInOwnedMailboxScenes(t *testing.T) {
	f, store, router := homeSceneRouter(t, WithCharacterNames(letterNames{}))
	offered := func() []string {
		t.Helper()
		status, response := navigationGET(t, router)
		if status != 200 {
			t.Fatalf("observe %s: %d", store.selection.Destination, status)
		}
		var ids []string
		for _, a := range response.AvailableActions {
			if slices.Contains([]string{"home_letter_send", "home_letter_read", "home_letter_delete"}, a.Action) {
				ids = append(ids, a.Action)
			}
		}
		return ids
	}
	for _, tc := range []struct {
		destination string
		want        []string
	}{
		{"home", []string{"home_letter_send"}},
		{"home_inbox", []string{"home_letter_read", "home_letter_delete"}},
		{"home_outbox", []string{"home_letter_delete"}},
	} {
		store.selection = playercontext.Selection{Destination: tc.destination}
		if got := offered(); !slices.Equal(got, tc.want) {
			t.Fatalf("%s offers %v, want %v", tc.destination, got, tc.want)
		}
	}
	// The owner's Home observes the unread count; a visitor's does not.
	store.selection = playercontext.Selection{Destination: "home"}
	if _, response := navigationGET(t, router); decodeSceneData[HomeSceneData](t, response).View.Private.UnreadLetterCount != 1 {
		t.Fatal("owned Home lacks the unread letter count")
	}
	store.selection = playercontext.Selection{Destination: "home", Subject: playercontext.Subject{Kind: "home", ID: "other"}}
	if got := offered(); len(got) != 0 {
		t.Fatalf("visitor offered %v", got)
	}
	if len(f.living) != 0 || f.executions != 0 {
		t.Fatalf("observation mutated: %v", f.living)
	}
}

func TestHomeLetterCommandsActForTheActor(t *testing.T) {
	f, store, router := homeSceneRouter(t, WithCharacterNames(letterNames{"Mage": "mage"}))
	for _, tc := range []struct{ destination, action, params, call string }{
		{"home", "home_letter_send", `{"recipient_name":"Mage","content":"やあ"}`, "send hero mage やあ"},
		{"home_inbox", "home_letter_read", `{"letter_id":"letter-2"}`, "read hero letter-2"},
		{"home_inbox", "home_letter_delete", `{"letter_id":"letter-2"}`, "delete hero letter-2"},
		{"home_outbox", "home_letter_delete", `{"letter_id":"letter-1"}`, "delete hero letter-1"},
	} {
		store.selection = playercontext.Selection{Destination: tc.destination}
		f.living = nil
		status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"`+tc.action+`","params":`+tc.params+`}`, nil)
		if status != 200 || !slices.Equal(f.living, []string{tc.call}) {
			t.Fatalf("%s in %s: %d %s %v", tc.action, tc.destination, status, raw, f.living)
		}
	}

	// Commands are scene-scoped: reading is not admitted from Home.
	store.selection = playercontext.Selection{Destination: "home"}
	f.living = nil
	if status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"home_letter_read","params":{"letter_id":"letter-2"}}`, nil); status == 200 || len(f.living) != 0 {
		t.Fatalf("read outside the inbox: %d %s", status, raw)
	}
	for _, params := range []string{`{}`, `{"letter_id":"x","character_id":"other"}`, `{"letter_id":1}`} {
		store.selection = playercontext.Selection{Destination: "home_inbox"}
		status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"home_letter_read","params":`+params+`}`, nil)
		if status != 400 || len(f.living) != 0 {
			t.Fatalf("schema %s: %d %s", params, status, raw)
		}
		assertGatewayError(t, raw, "INVALID_ACTION_PARAMS")
	}
}

func TestHomeLetterRejections(t *testing.T) {
	f, store, router := homeSceneRouter(t, WithCharacterNames(letterNames{"Mage": "mage"}))
	store.selection = playercontext.Selection{Destination: "home"}
	status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"home_letter_send","params":{"recipient_name":"Nobody","content":"x"}}`, nil)
	if status != 404 || len(f.living) != 0 {
		t.Fatalf("unknown recipient: %d %s %v", status, raw, f.living)
	}
	assertGatewayError(t, raw, "RECIPIENT_NOT_FOUND")

	for _, tc := range []struct {
		destination, action, params string
		err                         error
		status                      int
		code                        string
	}{
		{"home", "home_letter_send", `{"recipient_name":"Mage","content":"x"}`, home.ErrLetterThrottled, 429, "HOME_LETTER_THROTTLED"},
		{"home", "home_letter_send", `{"recipient_name":"Mage","content":"x"}`, home.ErrEmptyContent, 400, "HOME_INVALID_LETTER"},
		{"home", "home_letter_send", `{"recipient_name":"Mage","content":"x"}`, validation.ErrControlCharacter, 400, "HOME_INVALID_LETTER"},
		// Another character's letter is reported like a missing one.
		{"home_inbox", "home_letter_read", `{"letter_id":"theirs"}`, home.ErrForbidden, 404, "HOME_LETTER_NOT_FOUND"},
		{"home_outbox", "home_letter_delete", `{"letter_id":"gone"}`, home.ErrLetterNotFound, 404, "HOME_LETTER_NOT_FOUND"},
	} {
		store.selection = playercontext.Selection{Destination: tc.destination}
		f.livingErr = tc.err
		status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"`+tc.action+`","params":`+tc.params+`}`, nil)
		if status != tc.status {
			t.Fatalf("%s %v: %d %s", tc.action, tc.err, status, raw)
		}
		assertGatewayError(t, raw, tc.code)
	}
}
