package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/contest"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/home"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/logging"
	"github.com/witchcraze/party2re/internal/testutil"
)

var contestPrizeTables = []string{
	"players", "characters", "inventory_items", "equipment_slots", "character_depots", "depot_items",
	"character_pending_remittances", "character_delivery_notices", "guilds", "guild_members",
	"character_memories", "character_photos", "contest_rounds", "contest_entries", "contest_votes",
	"contest_legends", "news_articles",
}

type contestPrizeFixture struct {
	db      *sql.DB
	core    *coreServices
	econ    *econServices
	soc     *socServices
	misc    *miscServices
	repo    *database.ContestRepository
	placed  []corecharacter.Character // 1st..5th
	voter   corecharacter.Character
	unknown item.Instance // unrelated Depot row of the 1st place
}

// newContestPrizeFixture prepares an overdue round: 1st place votes for itself
// and has a full Depot holding a medal stack; 2nd place votes for 1st and is over
// capacity without one; an outside voter also votes for 1st; 4th votes for 2nd.
func newContestPrizeFixture(t *testing.T) contestPrizeFixture {
	t.Helper()
	// Settlement selects rounds globally, so it needs a dedicated database.
	db := database.CreateTestDatabase(t, contestPrizeTables...)
	ctx := context.Background()
	client := testutil.OpenValkey(t)
	core, err := newCoreServices(db, client)
	if err != nil {
		t.Fatal(err)
	}
	econ, err := newEconServices(db, core)
	if err != nil {
		t.Fatal(err)
	}
	soc, err := newSocServices(db, core, econ, client, logging.Nop())
	if err != nil {
		t.Fatal(err)
	}
	misc, err := newMiscServices(db, core, soc, econ, nil)
	if err != nil {
		t.Fatal(err)
	}
	repo, err := database.NewContestRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	f := contestPrizeFixture{db: db, core: core, econ: econ, soc: soc, misc: misc, repo: repo}
	now := time.Now().UTC()
	if err := repo.SaveRound(ctx, contest.ContestRound{Round: 1, Status: contest.StatusActive, StartTime: now.Add(-time.Hour), EndTime: now}); err != nil {
		t.Fatal(err)
	}
	for i := range contest.MinEntriesForContest {
		char, err := database.CreateTestCharacterWithFunds(ctx, db, "CP"+id.New()[:6], 1000)
		if err != nil {
			t.Fatal(err)
		}
		f.placed = append(f.placed, char)
		if err := repo.SaveEntry(ctx, contest.ContestEntry{ID: id.New(), Round: 1, CharacterID: char.ID, CharacterName: char.Name,
			Title: fmt.Sprintf("entry-%d", i), Votes: 10 - i, CreatedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	if f.voter, err = database.CreateTestCharacter(ctx, db, "CV"+id.New()[:6]); err != nil {
		t.Fatal(err)
	}
	entries, err := repo.ListEntriesByRound(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	entryOf := map[string]string{}
	for _, e := range entries {
		entryOf[e.CharacterID] = e.ID
	}
	for _, v := range []struct{ voter, target string }{
		{f.placed[0].ID, f.placed[0].ID}, {f.placed[1].ID, f.placed[0].ID}, {f.voter.ID, f.placed[0].ID}, {f.placed[3].ID, f.placed[1].ID},
	} {
		if err := repo.SaveVote(ctx, contest.ContestVote{ID: id.New(), Round: 1, EntryID: entryOf[v.target], VoterCharacterID: v.voter, CreatedAt: now}); err != nil {
			t.Fatal(err)
		}
	}

	fill := func(char corecharacter.Character, extra int, medals int) {
		dep, err := depot.NewDepotWithCapacity(char.ID, char.JobLevel, 0, char.OverDepot)
		if err != nil {
			t.Fatal(err)
		}
		if medals > 0 {
			dep.AppendDelivery(item.Instance{ID: id.New(), DefinitionID: contest.PrizeMedalItemID, Quantity: medals})
		}
		for len(dep.Items) < dep.Capacity+extra {
			dep.AppendDelivery(item.Instance{ID: id.New(), DefinitionID: "weapon-01", Quantity: 1})
		}
		if err := econ.depotRepo.Save(ctx, dep); err != nil {
			t.Fatal(err)
		}
	}
	fill(f.placed[0], 0, 4)
	fill(f.placed[1], 2, 0)
	dep, err := econ.depotRepo.FindByCharacterID(ctx, f.placed[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	f.unknown = dep.Items[len(dep.Items)-1]
	return f
}

func (f contestPrizeFixture) medals(t *testing.T, characterID string) (stacks []int, slots int) {
	t.Helper()
	dep, err := f.econ.depotRepo.FindByCharacterID(context.Background(), characterID)
	if errors.Is(err, depot.ErrNotFound) {
		return nil, 0
	}
	if err != nil {
		t.Fatal(err)
	}
	for _, inst := range dep.Items {
		if inst.DefinitionID == contest.PrizeMedalItemID {
			stacks = append(stacks, inst.Quantity)
		}
	}
	return stacks, len(dep.Items)
}

func (f contestPrizeFixture) notices(t *testing.T, characterID string) []string {
	t.Helper()
	notices, err := f.soc.home.ListDeliveryNotices(context.Background(), characterID, true)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, n := range notices {
		out = append(out, n.NoticeType+"|"+n.Message)
	}
	slices.Sort(out)
	return out
}

func (f contestPrizeFixture) held(t *testing.T, characterID string) []depot.Remittance {
	t.Helper()
	held, err := f.econ.depotRepo.ListRemittances(context.Background(), characterID)
	if err != nil {
		t.Fatal(err)
	}
	for i := range held {
		held[i].ID = ""
	}
	return held
}

func TestContestPrizeProductionWiringIntegration(t *testing.T) {
	f := newContestPrizeFixture(t)
	ctx := context.Background()
	_, firstSlots := f.medals(t, f.placed[0].ID)
	_, secondSlots := f.medals(t, f.placed[1].ID)

	res, err := f.misc.contest.SettleContest(ctx, true)
	if err != nil || !res.PrizesDistributed || res.VotersRewarded != 3 {
		t.Fatalf("settle: %+v, %v", res, err)
	}

	gold := []int{15000, 7000, 3000}
	wantMedals := [][]int{{4 + 10}, {6}, {2}, nil, nil}
	for i, char := range f.placed {
		var wantHeld []depot.Remittance
		if i < 3 {
			wantHeld = []depot.Remittance{{CharacterID: char.ID, Amount: gold[i], Source: depot.RemittanceContestPrize,
				SenderName: fmt.Sprintf("第1回フォトコンテスト%d位", i+1)}}
		}
		if got := f.held(t, char.ID); !slices.Equal(got, wantHeld) {
			t.Errorf("place %d held = %+v, want %+v", i+1, got, wantHeld)
		}
		if got, _ := f.medals(t, char.ID); !slices.Equal(got, wantMedals[i]) {
			t.Errorf("place %d medals = %v, want %v", i+1, got, wantMedals[i])
		}
		after, err := f.core.charRepo.FindByID(ctx, char.ID)
		if err != nil || after.Money != char.Money || after.SmallMedals != char.SmallMedals {
			t.Errorf("place %d wallet/counter changed: %+v, %v", i+1, after, err)
		}
	}
	// The full Depot merges into its stack; the over-capacity Depot gains a slot.
	if _, slots := f.medals(t, f.placed[0].ID); slots != firstSlots {
		t.Errorf("1st place slots = %d, want %d", slots, firstSlots)
	}
	if _, slots := f.medals(t, f.placed[1].ID); slots != secondSlots+1 {
		t.Errorf("2nd place slots = %d, want %d", slots, secondSlots+1)
	}
	dep, err := f.econ.depotRepo.FindByCharacterID(ctx, f.placed[0].ID)
	if err != nil || !slices.ContainsFunc(dep.Items, func(i item.Instance) bool { return i == f.unknown }) {
		t.Errorf("unrelated Depot row lost: %v", err)
	}
	if got, _ := f.medals(t, f.voter.ID); !slices.Equal(got, []int{1}) {
		t.Errorf("voter medals = %v", got)
	}
	if got := f.held(t, f.voter.ID); len(got) != 0 {
		t.Errorf("voter held gold = %+v", got)
	}

	money := depot.NoticeMoneyTransfer + "|お金が預かり所に届いています"
	medal := depot.NoticeItemTransfer + "|預かり所にフォトコンテストから小さなメダルが届いています"
	for who, want := range map[string][]string{
		f.placed[0].ID: {medal, money}, f.placed[1].ID: {medal, money}, f.placed[2].ID: {money},
		f.placed[3].ID: nil, f.voter.ID: {medal},
	} {
		if got := f.notices(t, who); !slices.Equal(got, want) {
			t.Errorf("%s notices = %q, want %q", who, got, want)
		}
	}

	// Collection at the wallet cap keeps the remainder held.
	if _, err := f.db.ExecContext(ctx, "UPDATE characters SET money = ? WHERE id = ?", corecharacter.MaxMoney-5000, f.placed[0].ID); err != nil {
		t.Fatal(err)
	}
	_, collected, err := f.econ.depot.WithdrawItem(ctx, f.placed[0].ID, "")
	if err != nil || len(collected) != 1 || collected[0].Amount != 5000 || collected[0].Source != depot.RemittanceContestPrize {
		t.Fatalf("collect: %+v, %v", collected, err)
	}
	if got := f.held(t, f.placed[0].ID); len(got) != 1 || got[0].Amount != 10000 {
		t.Fatalf("remaining held = %+v", got)
	}

	// A replay finds no settleable round and creates nothing more.
	if res, err := f.misc.contest.SettleContest(ctx, true); err != nil || res.PrizesDistributed {
		t.Fatalf("replay: %+v, %v", res, err)
	}
	if got, _ := f.medals(t, f.voter.ID); !slices.Equal(got, []int{1}) {
		t.Errorf("voter medals after replay = %v", got)
	}
}

type failingContestNotice struct {
	*home.Service
	fail bool
}

func (f *failingContestNotice) AddDeliveryNotice(ctx context.Context, actor, kind, message string) error {
	if err := f.Service.AddDeliveryNotice(ctx, actor, kind, message); err != nil {
		return err
	}
	if f.fail {
		return errNoticeAfterWrite
	}
	return nil
}

func TestContestPrizeRollbackAndRetryIntegration(t *testing.T) {
	f := newContestPrizeFixture(t)
	ctx := context.Background()
	notice := &failingContestNotice{Service: f.soc.home, fail: true}
	svc, err := contest.NewService(f.core.charRepo, f.repo, contest.WithTransactionProvider(f.core.txProvider),
		contest.WithPrizeDelivery(contestPrizeDelivery{f.econ.depotRepo, f.econ.depotRepo, notice, f.core.itemCatalog}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SettleContest(ctx, true); !errors.Is(err, errNoticeAfterWrite) {
		t.Fatalf("injected failure: %v", err)
	}
	for _, char := range append(f.placed, f.voter) {
		if held, notices := f.held(t, char.ID), f.notices(t, char.ID); len(held) != 0 || len(notices) != 0 {
			t.Errorf("%s kept held=%+v notices=%q after rollback", char.ID, held, notices)
		}
	}
	if got, _ := f.medals(t, f.placed[0].ID); !slices.Equal(got, []int{4}) {
		t.Errorf("1st place medals after rollback = %v", got)
	}
	if round, err := f.repo.GetRoundByNumber(ctx, 1); err != nil || round.Status != contest.StatusActive {
		t.Fatalf("round after rollback: %+v, %v", round, err)
	}

	notice.fail = false
	if res, err := svc.SettleContest(ctx, true); err != nil || !res.PrizesDistributed {
		t.Fatalf("retry: %+v, %v", res, err)
	}
	if got := f.held(t, f.placed[0].ID); len(got) != 1 || got[0].Amount != 15000 {
		t.Errorf("held after retry = %+v", got)
	}
	if got, _ := f.medals(t, f.placed[0].ID); !slices.Equal(got, []int{14}) {
		t.Errorf("1st place medals after retry = %v", got)
	}
}

// Competing settlements and collections conserve gold (wallet + held) and medals.
func TestContestPrizeConcurrentSettlementAndCollectionIntegration(t *testing.T) {
	f := newContestPrizeFixture(t)
	ctx := context.Background()
	winner := f.placed[0].ID
	settle := func() error {
		_, err := f.misc.contest.SettleContest(ctx, true)
		return err
	}
	collect := func() error {
		_, _, err := f.econ.depot.WithdrawItem(ctx, winner, "")
		if errors.Is(err, depot.ErrInvalidItemInstanceID) {
			return nil // ran before settlement: nothing held to collect
		}
		return err
	}
	for _, err := range testutil.RunRace(settle, settle, collect, collect) {
		if err != nil {
			t.Fatal(err)
		}
	}
	char, err := f.core.charRepo.FindByID(ctx, winner)
	if err != nil {
		t.Fatal(err)
	}
	total := char.Money
	for _, r := range f.held(t, winner) {
		total += r.Amount
	}
	if total != 1000+15000 {
		t.Errorf("winner wallet+held = %d, want %d", total, 1000+15000)
	}
	if got, _ := f.medals(t, winner); !slices.Equal(got, []int{14}) {
		t.Errorf("winner medals = %v, want [14]", got)
	}
	if got, _ := f.medals(t, f.voter.ID); !slices.Equal(got, []int{1}) {
		t.Errorf("voter medals = %v, want [1]", got)
	}
}
