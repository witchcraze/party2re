package party_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/valkey-io/valkey-go"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/party"
)

func liveLobbyRepository(t *testing.T, partyIDs, actors []string) (valkey.Client, *party.ValkeyRepository, string) {
	t.Helper()
	client, closeClient := getLiveValkeyClient(t)
	t.Cleanup(closeClient)
	prefix := "party2:test:party-lobby:" + id.New() + ":"
	t.Cleanup(func() {
		ctx := context.Background()
		keys := []string{prefix + "lobbies"}
		for _, id := range partyIDs {
			keys = append(keys, prefix+"lobby:"+id)
		}
		for _, actor := range actors {
			keys = append(keys, prefix+"character:"+actor)
		}
		for _, key := range keys {
			if err := client.Do(ctx, client.B().Del().Key(key).Build()).Error(); err != nil {
				t.Error(err)
			}
		}
	})
	repo := party.NewValkeyRepository(client, party.WithLobbyKeyPrefix(prefix+"lobby:"),
		party.WithCharacterKeyPrefix(prefix+"character:"), party.WithLobbiesIndexKey(prefix+"lobbies"))
	return client, repo, prefix
}

func TestLiveLobbyRenewalKeepsAllLookups(t *testing.T) {
	client, repo, prefix := liveLobbyRepository(t, []string{"original", "other"}, []string{"leader", "ally", "peer"})
	ctx := t.Context()
	p := sampleTestParty("original", "leader", "Original")
	seedIdleLobby(t, repo, p)
	other := sampleTestParty("other", "peer", "Other")
	seedIdleLobby(t, repo, other)

	originalExpiry := time.Now().Add(2 * time.Second)
	lobbyKey := prefix + "lobby:" + p.ID
	actorKey := func(actor string) string { return prefix + "character:" + actor }
	for _, key := range []string{lobbyKey, actorKey("leader")} {
		if err := client.Do(ctx, client.B().Pexpireat().Key(key).MillisecondsTimestamp(originalExpiry.UnixMilli()).Build()).Error(); err != nil {
			t.Fatal(err)
		}
	}
	ally := party.Member{PartyID: p.ID, CharacterID: "ally", JoinedAt: p.CreatedAt.Add(time.Second)}
	if err := repo.AddMember(ctx, ally); err != nil {
		t.Fatal(err)
	}

	expiry := func(key string) int64 {
		t.Helper()
		n, err := client.Do(ctx, client.B().Pexpiretime().Key(key).Build()).AsInt64()
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	if got := expiry(lobbyKey); got <= originalExpiry.UnixMilli() ||
		got != expiry(actorKey("leader")) || got != expiry(actorKey("ally")) {
		t.Fatalf("lobby and members did not renew together, lobby deadline %d", got)
	}
	timer := time.NewTimer(time.Until(originalExpiry))
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	for _, actor := range []string{"leader", "ally"} {
		got, _, err := repo.GetActivePartyByCharacter(ctx, actor)
		if err != nil || got.ID != p.ID {
			t.Fatalf("%s lost membership after initial expiry: %v, %v", actor, got.ID, err)
		}
	}

	chars := &liveTestCharRepo{chars: map[string]corecharacter.Character{
		"leader": {ID: "leader", Stats: corecharacter.Stats{HP: 50, MaxHP: 50}},
	}}
	service, err := party.NewService(repo, chars, nil, liveTestStageProvider{}, liveTestMonsterProvider{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.JoinParty(ctx, other.ID, "leader", ""); !errors.Is(err, party.ErrAlreadyInParty) {
		t.Fatalf("join another lobby = %v", err)
	}
	if _, err := service.CreateParty(ctx, "leader", party.CreatePartyRequest{Name: "Duplicate", StageID: "stage-1", Speed: 3}); !errors.Is(err, party.ErrAlreadyInParty) {
		t.Fatalf("create another lobby = %v", err)
	}
	if err := repo.RemoveMember(ctx, p.ID, "leader"); err != nil {
		t.Fatal(err)
	}
	handover, member, err := repo.GetActivePartyByCharacter(ctx, "ally")
	if err != nil || handover.LeaderCharacterID != "ally" || !member.IsLeader {
		t.Fatalf("leader handover = %+v, %+v, %v", handover, member, err)
	}
	if expiry(lobbyKey) != expiry(actorKey("ally")) {
		t.Fatal("handover did not renew the remaining mapping")
	}
	handover.Speed = 18
	if err := repo.UpdateParty(ctx, handover); err != nil {
		t.Fatal(err)
	}
	if expiry(lobbyKey) != expiry(actorKey("ally")) {
		t.Fatal("configuration change did not renew the remaining mapping")
	}

	// A later lobby owns the mapping while the old roster still refers to it.
	if err := client.Do(ctx, client.B().Set().Key(actorKey("ally")).Value(other.ID).ExSeconds(600).Build()).Error(); err != nil {
		t.Fatal(err)
	}
	if err := repo.RemoveMember(ctx, p.ID, "ally"); err != nil {
		t.Fatal(err)
	}
	if err := repo.DeleteParty(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	if got, err := client.Do(ctx, client.B().Get().Key(actorKey("ally")).Build()).ToString(); err != nil || got != other.ID {
		t.Fatalf("cleanup removed the other lobby's mapping: %q, %v", got, err)
	}
}

func TestLiveLobbyIndexFailureLeavesNoPublication(t *testing.T) {
	client, repo, prefix := liveLobbyRepository(t, []string{"new", "other"}, []string{"leader", "peer"})
	ctx := t.Context()
	p := sampleTestParty("new", "leader", "New")
	leader := party.Member{PartyID: p.ID, CharacterID: p.LeaderCharacterID, IsLeader: true, JoinedAt: p.CreatedAt}
	indexKey := prefix + "lobbies"
	if err := client.Do(ctx, client.B().Set().Key(indexKey).Value("wrong type").Build()).Error(); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveParty(ctx, p, leader); err == nil {
		t.Fatal("publication succeeded with an unwritable index")
	}
	for _, key := range []string{prefix + "lobby:" + p.ID, prefix + "character:" + leader.CharacterID} {
		if result := client.Do(ctx, client.B().Get().Key(key).Build()); !valkey.IsValkeyNil(result.Error()) {
			t.Fatalf("failed publication left %s: %v", key, result.Error())
		}
	}
	if err := client.Do(ctx, client.B().Del().Key(indexKey).Build()).Error(); err != nil {
		t.Fatal(err)
	}
	other := sampleTestParty("other", "peer", "Other")
	seedIdleLobby(t, repo, other)
	if err := repo.SaveParty(ctx, p, leader); err != nil {
		t.Fatal(err)
	}
	if parties, total, err := repo.ListParties(ctx, party.StatusRecruiting, 10, 0); err != nil || total != 2 || len(parties) != total {
		t.Fatalf("recruiting list after retry = %+v, %d, %v", parties, total, err)
	}
	for actor, want := range map[string]string{leader.CharacterID: p.ID, other.LeaderCharacterID: other.ID} {
		if got, _, err := repo.GetActivePartyByCharacter(ctx, actor); err != nil || got.ID != want {
			t.Fatalf("mapping for %s = %s, %v", actor, got.ID, err)
		}
	}
}
