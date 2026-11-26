package god_test

import (
	"context"
	"fmt"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/job"
	"github.com/witchcraze/party2re/internal/core/progression"
	"github.com/witchcraze/party2re/internal/god"
)

func TestGod_StatsWishClampsWithoutHealing(t *testing.T) {
	for _, tc := range []struct {
		name       string
		hpmp, stat int
		want       corecharacter.Stats
	}{
		{"below", 500, 100, corecharacter.Stats{MaxHP: 540, MaxMP: 540, HP: 100, MP: 50, Attack: 140, Defense: 140, Agility: 140}},
		{"just below cap", 958, 214, corecharacter.Stats{MaxHP: 998, MaxMP: 998, HP: 100, MP: 50, Attack: 254, Defense: 254, Agility: 254}},
		{"reach cap", 959, 215, corecharacter.Stats{MaxHP: 999, MaxMP: 999, HP: 100, MP: 50, Attack: 255, Defense: 255, Agility: 255}},
		{"cross cap", 980, 240, corecharacter.Stats{MaxHP: 999, MaxMP: 999, HP: 100, MP: 50, Attack: 255, Defense: 255, Agility: 255}},
		{"at cap", 999, 255, corecharacter.Stats{MaxHP: 999, MaxMP: 999, HP: 100, MP: 50, Attack: 255, Defense: 255, Agility: 255}},
		{"above cap", 1039, 295, corecharacter.Stats{MaxHP: 999, MaxMP: 999, HP: 100, MP: 50, Attack: 255, Defense: 255, Agility: 255}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newMockCharacterRepo()
			c := corecharacter.Character{ID: "c", Level: 99, GodVisit: corecharacter.GodVisitHeaven,
				Stats: corecharacter.Stats{MaxHP: tc.hpmp, MaxMP: tc.hpmp, HP: 100, MP: 50, Attack: tc.stat, Defense: tc.stat, Agility: tc.stat}}
			repo.characters[c.ID] = c
			svc, err := god.NewService(repo)
			if err != nil {
				t.Fatal(err)
			}
			res, err := svc.GrantWish(context.Background(), c.ID, god.WishStats)
			if err != nil {
				t.Fatal(err)
			}
			stored, err := repo.FindByID(context.Background(), c.ID)
			if err != nil {
				t.Fatal(err)
			}
			if stored != res.Character || stored.Stats != tc.want || stored.GodVisit != "" {
				t.Fatalf("stored=%+v result=%+v, want stats=%+v and ended visit", stored, res.Character, tc.want)
			}
		})
	}
}

func TestGod_RestoreLevelLimitNormalizesBeforeSaving(t *testing.T) {
	for _, level := range []int{50, 99, 100, 120, 150} {
		t.Run(fmt.Sprint(level), func(t *testing.T) {
			repo := newMockCharacterRepo()
			c := corecharacter.Character{ID: "c", Level: level, OverLevel: true, Experience: 200000, SP: 123,
				GodVisit: corecharacter.GodVisitHeaven,
				Stats:    corecharacter.Stats{MaxHP: 1998, MaxMP: 1500, HP: 1998, MP: 30, Attack: 510, Defense: 400, Agility: 200}}
			repo.characters[c.ID] = c
			svc, err := god.NewService(repo)
			if err != nil {
				t.Fatal(err)
			}
			res, err := svc.GrantWish(context.Background(), c.ID, god.WishRestoreLevelLimit)
			if err != nil {
				t.Fatal(err)
			}
			stored, err := repo.FindByID(context.Background(), c.ID)
			if err != nil {
				t.Fatal(err)
			}
			want := c
			want.OverLevel, want.Level, want.GodVisit = false, min(level, 99), ""
			want.Stats = corecharacter.Stats{MaxHP: 999, MaxMP: 999, HP: 999, MP: 30, Attack: 255, Defense: 255, Agility: 200}
			if stored != want || res.Character != want {
				t.Fatalf("stored=%+v result=%+v, want %+v", stored, res.Character, want)
			}
			gained, err := progression.ApplyExperience(&stored, 1)
			if err != nil || gained != 99-want.Level || stored.Level != 99 || stored.Experience != c.Experience+1 {
				t.Fatalf("experience after restore: character=%+v gained=%d err=%v", stored, gained, err)
			}
		})
	}
}

func TestGod_HeavenWishOverLevelCeilings(t *testing.T) {
	for _, level := range []int{99, 100, 120} {
		t.Run(fmt.Sprint(level), func(t *testing.T) {
			repo := newMockCharacterRepo()
			c := corecharacter.Character{ID: "c", Level: level, OverLevel: true, GodVisit: corecharacter.GodVisitHeaven,
				Stats: corecharacter.Stats{MaxHP: 2038, MaxMP: 2038, HP: 2038, MP: 10, Attack: 550, Defense: 550, Agility: 550}}
			repo.characters[c.ID] = c
			svc, err := god.NewService(repo)
			if err != nil {
				t.Fatal(err)
			}
			if res, err := svc.GrantWish(context.Background(), c.ID, god.WishStats); err != nil || !res.Refused {
				t.Fatalf("stat wish %+v err=%v", res, err)
			}
			if repo.characters[c.ID] != c {
				t.Fatal("refused wish changed character")
			}
			res, err := svc.GrantWish(context.Background(), c.ID, god.WishSP)
			if err != nil {
				t.Fatal(err)
			}
			want := corecharacter.Stats{MaxHP: 999, MaxMP: 999, HP: 999, MP: 10, Attack: 255, Defense: 255, Agility: 255}
			if level > 99 {
				want = corecharacter.Stats{MaxHP: 1998, MaxMP: 1998, HP: 1998, MP: 10, Attack: 510, Defense: 510, Agility: 510}
			}
			if res.Character.Stats != want || !res.Character.OverLevel || res.Character.Level != level || repo.characters[c.ID] != res.Character {
				t.Fatalf("result=%+v, want stats=%+v with level and OverLevel preserved", res.Character, want)
			}
		})
	}
}

type zeroGrowthRandom struct{}

func (zeroGrowthRandom) Intn(int) (int, error) { return 0, nil }

func TestGod_LimitBreakGrowsBeyondNormalStatCapAtLevel100(t *testing.T) {
	repo := newMockCharacterRepo()
	c := corecharacter.Character{ID: "c", Level: 99, Experience: 98000, GodVisit: corecharacter.GodVisitHeaven,
		Stats: corecharacter.Stats{MaxHP: 999, HP: 100, MaxMP: 999, MP: 50, Attack: 255, Defense: 255, Agility: 255}}
	repo.characters[c.ID] = c
	svc, err := god.NewService(repo)
	if err != nil {
		t.Fatal(err)
	}
	res, err := svc.GrantWish(context.Background(), c.ID, god.WishLimitBreakLevel)
	if err != nil {
		t.Fatal(err)
	}
	want := c
	want.OverLevel, want.GodVisit = true, ""
	if res.Character != want || repo.characters[c.ID] != want {
		t.Fatalf("result=%+v, want %+v", res.Character, want)
	}
	stored, err := repo.FindByID(context.Background(), c.ID)
	if err != nil {
		t.Fatal(err)
	}
	result, err := progression.ApplyExperienceWithJobFull(&stored, 10, job.Definition{ID: "job-01"}, zeroGrowthRandom{}, progression.ApplyExperienceOptions{})
	if err != nil || result.LevelsGained != 1 || stored.Level != 100 || stored.Stats.MaxHP != 1000 || stored.Stats.HP != 100 || stored.Stats.MP != 50 {
		t.Fatalf("growth after limit break: character=%+v result=%+v err=%v", stored, result, err)
	}
}
