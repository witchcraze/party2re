package god_test

import (
	"context"
	"errors"
	"testing"

	"github.com/witchcraze/party2re/internal/casino"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/god"
	"github.com/witchcraze/party2re/internal/guild"
)

type mockCasinoRepo struct {
	coins map[string]int64
}

func (m *mockCasinoRepo) AdjustCoins(_ context.Context, charID string, delta int64) (casino.Account, error) {
	if m.coins == nil {
		m.coins = make(map[string]int64)
	}
	m.coins[charID] += delta
	return casino.Account{CharacterID: charID, Coins: m.coins[charID]}, nil
}

type mockLotteryRepo struct {
	tickets map[string]int
}

func (m *mockLotteryRepo) AddRaffleTickets(_ context.Context, charID string, count int) (int, error) {
	if m.tickets == nil {
		m.tickets = make(map[string]int)
	}
	m.tickets[charID] += count
	return m.tickets[charID], nil
}

type mockGuildRepo struct {
	guilds  map[string]guild.Guild
	members map[string]guild.Member
}

func (m *mockGuildRepo) GetGuildByCharacter(_ context.Context, charID string) (guild.Guild, guild.Member, error) {
	mem, ok := m.members[charID]
	if !ok {
		return guild.Guild{}, guild.Member{}, guild.ErrCharacterNotInGuild
	}
	g, ok := m.guilds[mem.GuildID]
	if !ok {
		return guild.Guild{}, guild.Member{}, guild.ErrGuildNotFound
	}
	return g, mem, nil
}

func (m *mockGuildRepo) AddPoints(_ context.Context, guildID string, points int64) error {
	g, ok := m.guilds[guildID]
	if !ok {
		return guild.ErrGuildNotFound
	}
	g.Points += points
	m.guilds[guildID] = g
	return nil
}

func (m *mockGuildRepo) UpdateBgimg(_ context.Context, guildID string, bgimg string) error {
	g, ok := m.guilds[guildID]
	if !ok {
		return guild.ErrGuildNotFound
	}
	g.Bgimg = bgimg
	m.guilds[guildID] = g
	return nil
}

type mockHomeMemberRepo struct {
	members map[string][]god.HomeMember
}

func (m *mockHomeMemberRepo) FindByCharacterID(_ context.Context, charID string) ([]god.HomeMember, error) {
	return m.members[charID], nil
}

func (m *mockHomeMemberRepo) AddMember(_ context.Context, member god.HomeMember) error {
	if m.members == nil {
		m.members = make(map[string][]god.HomeMember)
	}
	m.members[member.CharacterID] = append(m.members[member.CharacterID], member)
	return nil
}

type mockProfileRepo struct {
	avatars map[string]string
}

func (m *mockProfileRepo) UpdateAvatar(_ context.Context, charID string, avatarURL string) error {
	if m.avatars == nil {
		m.avatars = make(map[string]string)
	}
	m.avatars[charID] = avatarURL
	return nil
}

type mockHomeRepo struct {
	bgimgs map[string]string
}

func (m *mockHomeRepo) UpdateBgimg(_ context.Context, charID string, bgimg string) error {
	if m.bgimgs == nil {
		m.bgimgs = make(map[string]string)
	}
	m.bgimgs[charID] = bgimg
	return nil
}

func setupHeavenService() (*god.Service, *mockCharacterRepo, *mockDepotRepo, *mockCasinoRepo, *mockLotteryRepo, *mockGuildRepo, *mockHomeMemberRepo, *mockProfileRepo, *mockHomeRepo) {
	charRepo := newMockCharacterRepo()
	depotRepo := newMockDepotRepo()
	casinoRepo := &mockCasinoRepo{coins: make(map[string]int64)}
	lotteryRepo := &mockLotteryRepo{tickets: make(map[string]int)}
	guildRepo := &mockGuildRepo{
		guilds:  make(map[string]guild.Guild),
		members: make(map[string]guild.Member),
	}
	homeMemberRepo := &mockHomeMemberRepo{members: make(map[string][]god.HomeMember)}
	profileRepo := &mockProfileRepo{avatars: make(map[string]string)}
	homeRepo := &mockHomeRepo{bgimgs: make(map[string]string)}

	svc, _ := god.NewService(
		charRepo,
		god.WithDepotRepository(depotRepo),
		god.WithCasinoRepository(casinoRepo),
		god.WithLotteryRepository(lotteryRepo),
		god.WithGuildRepository(guildRepo),
		god.WithHomeMemberRepository(homeMemberRepo),
		god.WithProfileRepository(profileRepo),
		god.WithHomeRepository(homeRepo),
	)

	return svc, charRepo, depotRepo, casinoRepo, lotteryRepo, guildRepo, homeMemberRepo, profileRepo, homeRepo
}

func TestGod_HeavenWishes_AllCatalog(t *testing.T) {
	ctx := context.Background()

	t.Run("wish_stats adds 40 to all stats and rejects if OverLevel", func(t *testing.T) {
		svc, charRepo, _, _, _, _, _, _, _ := setupHeavenService()
		char := corecharacter.Character{
			ID: "char-stats", Name: "Hero",
			Stats: corecharacter.Stats{
				MaxHP: 100, HP: 50, MaxMP: 50, MP: 20, Attack: 30, Defense: 25, Agility: 20,
			},
		}
		charRepo.characters[char.ID] = char

		res, err := svc.GrantWish(ctx, char.ID, god.WishStats, god.RealmHeaven)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Character.Stats.MaxHP != 140 || res.Character.Stats.HP != 90 {
			t.Errorf("expected HP 90/140, got %d/%d", res.Character.Stats.HP, res.Character.Stats.MaxHP)
		}
		if res.Character.Stats.Attack != 70 || res.Character.Stats.Defense != 65 || res.Character.Stats.Agility != 60 {
			t.Errorf("unexpected stats: %+v", res.Character.Stats)
		}
		if res.NextLocation != "home" {
			t.Errorf("expected NextLocation home, got %s", res.NextLocation)
		}

		// Reject if OverLevel
		char.OverLevel = true
		charRepo.characters[char.ID] = char
		_, err = svc.GrantWish(ctx, char.ID, god.WishStats, god.RealmHeaven)
		if !errors.Is(err, god.ErrWishRequirement) {
			t.Errorf("expected ErrWishRequirement for OverLevel, got %v", err)
		}
	})

	t.Run("wish_sp adds 50 SP", func(t *testing.T) {
		svc, charRepo, _, _, _, _, _, _, _ := setupHeavenService()
		char := corecharacter.Character{ID: "char-sp", Name: "Mage", SP: 10}
		charRepo.characters[char.ID] = char

		res, err := svc.GrantWish(ctx, char.ID, god.WishSP, god.RealmHeaven)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Character.SP != 60 {
			t.Errorf("expected SP 60, got %d", res.Character.SP)
		}
		if res.NextLocation != "home" {
			t.Errorf("expected NextLocation home, got %s", res.NextLocation)
		}
	})

	t.Run("wish_money adds 100000 G", func(t *testing.T) {
		svc, charRepo, _, _, _, _, _, _, _ := setupHeavenService()
		char := corecharacter.Character{ID: "char-money", Name: "Merchant", Money: 500}
		charRepo.characters[char.ID] = char

		res, err := svc.GrantWish(ctx, char.ID, god.WishMoney, god.RealmHeaven)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Character.Money != 100500 {
			t.Errorf("expected Money 100500, got %d", res.Character.Money)
		}
		if res.NextLocation != "home" {
			t.Errorf("expected NextLocation home, got %s", res.NextLocation)
		}
	})

	t.Run("wish_casino_coins adds 50000 coins", func(t *testing.T) {
		svc, charRepo, _, casinoRepo, _, _, _, _, _ := setupHeavenService()
		char := corecharacter.Character{ID: "char-casino", Name: "Gambler"}
		charRepo.characters[char.ID] = char

		res, err := svc.GrantWish(ctx, char.ID, god.WishCasinoCoins, god.RealmHeaven)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if casinoRepo.coins[char.ID] != 50000 {
			t.Errorf("expected 50000 casino coins, got %d", casinoRepo.coins[char.ID])
		}
		if res.NextLocation != "home" {
			t.Errorf("expected NextLocation home, got %s", res.NextLocation)
		}
	})

	t.Run("wish_small_medals adds 20 medals", func(t *testing.T) {
		svc, charRepo, _, _, _, _, _, _, _ := setupHeavenService()
		char := corecharacter.Character{ID: "char-medals", Name: "Collector", SmallMedals: 5}
		charRepo.characters[char.ID] = char

		res, err := svc.GrantWish(ctx, char.ID, god.WishSmallMedals, god.RealmHeaven)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Character.SmallMedals != 25 {
			t.Errorf("expected SmallMedals 25, got %d", res.Character.SmallMedals)
		}
		if res.NextLocation != "home" {
			t.Errorf("expected NextLocation home, got %s", res.NextLocation)
		}
	})

	t.Run("wish_lottery_tickets adds 1000 tickets", func(t *testing.T) {
		svc, charRepo, _, _, lotteryRepo, _, _, _, _ := setupHeavenService()
		char := corecharacter.Character{ID: "char-lottery", Name: "Lucky"}
		charRepo.characters[char.ID] = char

		res, err := svc.GrantWish(ctx, char.ID, god.WishLotteryTickets, god.RealmHeaven)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if lotteryRepo.tickets[char.ID] != 1000 {
			t.Errorf("expected 1000 raffle tickets, got %d", lotteryRepo.tickets[char.ID])
		}
		if res.NextLocation != "home" {
			t.Errorf("expected NextLocation home, got %s", res.NextLocation)
		}
	})

	t.Run("wish_guild_rank and wish_guild_gorgeous require guild membership", func(t *testing.T) {
		svc, charRepo, _, _, _, guildRepo, _, _, _ := setupHeavenService()
		char := corecharacter.Character{ID: "char-guild", Name: "GuildMaster"}
		charRepo.characters[char.ID] = char

		// Without guild: fails
		_, err := svc.GrantWish(ctx, char.ID, god.WishGuildRank, god.RealmHeaven)
		if !errors.Is(err, god.ErrWishRequirement) {
			t.Errorf("expected ErrWishRequirement without guild, got %v", err)
		}

		// Join guild
		guildRepo.guilds["guild-1"] = guild.Guild{ID: "guild-1", Name: "Test Guild", Points: 200}
		guildRepo.members[char.ID] = guild.Member{GuildID: "guild-1", CharacterID: char.ID}

		// Rank up wish
		res, err := svc.GrantWish(ctx, char.ID, god.WishGuildRank, god.RealmHeaven)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if guildRepo.guilds["guild-1"].Points != 1200 {
			t.Errorf("expected 1200 guild points, got %d", guildRepo.guilds["guild-1"].Points)
		}
		if res.NextLocation != "home" {
			t.Errorf("expected NextLocation home, got %s", res.NextLocation)
		}

		// Gorgeous wish
		res, err = svc.GrantWish(ctx, char.ID, god.WishGuildGorgeous, god.RealmHeaven)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if guildRepo.guilds["guild-1"].Bgimg != "god.gif" {
			t.Errorf("expected god.gif guild bgimg, got %s", guildRepo.guilds["guild-1"].Bgimg)
		}
	})

	t.Run("wish_refresh recovers full HP and MP and reduces fatigue by 150%", func(t *testing.T) {
		svc, charRepo, _, _, _, _, _, _, _ := setupHeavenService()
		char := corecharacter.Character{
			ID: "char-refresh", Name: "Tired",
			Tired: 80,
			Stats: corecharacter.Stats{MaxHP: 300, HP: 10, MaxMP: 150, MP: 0},
		}
		charRepo.characters[char.ID] = char

		res, err := svc.GrantWish(ctx, char.ID, god.WishRefresh, god.RealmHeaven)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Character.Stats.HP != 300 || res.Character.Stats.MP != 150 {
			t.Errorf("expected full HP/MP (300/150), got %d/%d", res.Character.Stats.HP, res.Character.Stats.MP)
		}
		if res.Character.Tired != -70 {
			t.Errorf("expected Tired -70 (80 - 150), got %d", res.Character.Tired)
		}
		expectedMsg := "疲労度が 150% 回復し、HPとMPが完全に回復しました！"
		if res.Message != expectedMsg {
			t.Errorf("expected Message %q, got %q", expectedMsg, res.Message)
		}
		if res.NextLocation != "home" {
			t.Errorf("expected NextLocation home, got %s", res.NextLocation)
		}
	})

	t.Run("wish_all_orbs gives all 6 standard orbs", func(t *testing.T) {
		svc, charRepo, _, _, _, _, _, _, _ := setupHeavenService()
		char := corecharacter.Character{ID: "char-orbs", Name: "OrbHunter", Orb: ""}
		charRepo.characters[char.ID] = char

		res, err := svc.GrantWish(ctx, char.ID, god.WishAllOrbs, god.RealmHeaven)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !res.Character.HasAllOrbs() {
			t.Errorf("expected character to have all orbs, got %s", res.Character.Orb)
		}
		if res.NextLocation != "home" {
			t.Errorf("expected NextLocation home, got %s", res.NextLocation)
		}
	})

	t.Run("wish_celestial_dragon changes job to job-70 and rejects if already job-70", func(t *testing.T) {
		svc, charRepo, _, _, _, _, _, _, _ := setupHeavenService()
		char := corecharacter.Character{
			ID: "char-dragon", Name: "Warrior", JobID: "job-01",
			Stats: corecharacter.Stats{MaxHP: 200, HP: 200, Attack: 80, Defense: 60, Agility: 40},
		}
		charRepo.characters[char.ID] = char

		res, err := svc.GrantWish(ctx, char.ID, god.WishCelestialDragon, god.RealmHeaven)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Character.JobID != "job-70" {
			t.Errorf("expected job-70, got %s", res.Character.JobID)
		}
		if res.NextLocation != "home" {
			t.Errorf("expected NextLocation home, got %s", res.NextLocation)
		}

		// Reject if already job-70
		_, err = svc.GrantWish(ctx, char.ID, god.WishCelestialDragon, god.RealmHeaven)
		if !errors.Is(err, god.ErrWishRequirement) {
			t.Errorf("expected ErrWishRequirement for existing job-70, got %v", err)
		}
	})

	t.Run("wish_god_of_new_world updates avatar and home bgimg", func(t *testing.T) {
		svc, charRepo, _, _, _, _, _, profileRepo, homeRepo := setupHeavenService()
		char := corecharacter.Character{ID: "char-god-new-world", Name: "Light"}
		charRepo.characters[char.ID] = char

		res, err := svc.GrantWish(ctx, char.ID, god.WishGodOfNewWorld, god.RealmHeaven)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if profileRepo.avatars[char.ID] != "chr/052.gif" {
			t.Errorf("expected avatar chr/052.gif, got %s", profileRepo.avatars[char.ID])
		}
		if homeRepo.bgimgs[char.ID] != "god.gif" {
			t.Errorf("expected home bgimg god.gif, got %s", homeRepo.bgimgs[char.ID])
		}
		if res.NextLocation != "home" {
			t.Errorf("expected NextLocation home, got %s", res.NextLocation)
		}
	})

	t.Run("wish_ortega, wish_cat, wish_secret_maid add home members", func(t *testing.T) {
		svc, charRepo, _, _, _, _, homeMemberRepo, _, _ := setupHeavenService()
		char := corecharacter.Character{ID: "char-family", Name: "Hero"}
		charRepo.characters[char.ID] = char

		// Ortega
		res, err := svc.GrantWish(ctx, char.ID, god.WishOrtega, god.RealmHeaven)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(homeMemberRepo.members[char.ID]) != 1 || homeMemberRepo.members[char.ID][0].Name != "オルテガ" {
			t.Errorf("expected オルテガ home member, got %+v", homeMemberRepo.members[char.ID])
		}
		if res.NextLocation != "home" {
			t.Errorf("expected NextLocation home, got %s", res.NextLocation)
		}

		// Cat
		_, err = svc.GrantWish(ctx, char.ID, god.WishCat, god.RealmHeaven)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(homeMemberRepo.members[char.ID]) != 2 {
			t.Fatalf("expected 2 home members, got %d", len(homeMemberRepo.members[char.ID]))
		}

		// Secret Maid
		_, err = svc.GrantWish(ctx, char.ID, god.WishSecretMaid, god.RealmHeaven)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(homeMemberRepo.members[char.ID]) != 3 || homeMemberRepo.members[char.ID][2].Name != "メイド" {
			t.Errorf("expected メイド home member, got %+v", homeMemberRepo.members[char.ID])
		}

		// Repeat maid wish should fail
		_, err = svc.GrantWish(ctx, char.ID, god.WishSecretMaid, god.RealmHeaven)
		if !errors.Is(err, god.ErrWishRequirement) {
			t.Errorf("expected ErrWishRequirement for duplicate maid, got %v", err)
		}
	})

	t.Run("wish_erotic_book and wish_alchemy_recipe deliver items to depot", func(t *testing.T) {
		svc, charRepo, depotRepo, _, _, _, _, _, _ := setupHeavenService()
		char := corecharacter.Character{ID: "char-items", Name: "Scholar"}
		charRepo.characters[char.ID] = char

		// Erotic book -> depot
		res, err := svc.GrantWish(ctx, char.ID, god.WishEroticBook, god.RealmHeaven)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		dep := depotRepo.depots[char.ID]
		if len(dep.Items) != 1 || dep.Items[0].DefinitionID != "item-058" {
			t.Errorf("expected item-058 in depot, got %+v", dep.Items)
		}
		if res.NextLocation != "home" {
			t.Errorf("expected NextLocation home, got %s", res.NextLocation)
		}

		// Alchemy recipe -> depot
		_, err = svc.GrantWish(ctx, char.ID, god.WishAlchemyRecipe, god.RealmHeaven)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		dep = depotRepo.depots[char.ID]
		if len(dep.Items) != 2 {
			t.Fatalf("expected 2 items in depot, got %d", len(dep.Items))
		}
		if dep.Items[1].DefinitionID != "item-128" && dep.Items[1].DefinitionID != "item-129" {
			t.Errorf("expected item-128 or item-129 in depot, got %s", dep.Items[1].DefinitionID)
		}
	})

	t.Run("wish_lover gives joke rejection and does not teleport", func(t *testing.T) {
		svc, charRepo, _, _, _, _, _, _, _ := setupHeavenService()
		char := corecharacter.Character{ID: "char-lover", Name: "Lonely"}
		charRepo.characters[char.ID] = char

		res, err := svc.GrantWish(ctx, char.ID, god.WishLover, god.RealmHeaven)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.NextLocation != "" {
			t.Errorf("expected empty NextLocation, got %s", res.NextLocation)
		}
		if res.NPCSpeech != "それは無理な願いだ…。アドバイスとしては積極的にアピールするのだ…" {
			t.Errorf("unexpected NPC speech: %s", res.NPCSpeech)
		}
	})
}
