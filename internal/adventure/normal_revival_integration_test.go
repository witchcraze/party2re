package adventure_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/adventure"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/skill"
	"github.com/witchcraze/party2re/internal/id"
)

type revivalRunCombat struct {
	*normalCombat
	f *normalFixture
}

func (c *revivalRunCombat) ExecuteRunAction(ctx context.Context, r *adventure.NormalRun, cmd adventure.RunCommand) (adventure.RunActionEffects, error) {
	if cmd.Action != "job-66-revive" {
		return c.normalCombat.ExecuteRunAction(ctx, r, cmd)
	}
	fx := adventure.RunActionEffects{RoomActivity: true, ActorActivity: true, ReleaseStock: true}
	requester, err := c.f.chars.FindByID(ctx, cmd.ActorID)
	if err != nil {
		return fx, err
	}
	targets := corebattle.TargetContext{ActorID: cmd.ActorID}
	var teamID string
	for _, a := range r.Battle.Actors {
		if a.ID == cmd.ActorID {
			teamID = a.TeamID
			break
		}
	}
	for _, a := range r.Battle.Actors {
		if a.TeamID == teamID {
			targets.PartyIDs = append(targets.PartyIDs, a.ID)
		} else if a.HP > 0 {
			targets.EnemyIDs = append(targets.EnemyIDs, a.ID)
		}
	}
	revival := &corebattle.RevivalContext{TargetContext: targets}
	damage := corebattle.DamageContext{EffectContext: corebattle.EffectContext{TargetContext: targets, RequesterLevel: &requester.Level}}
	opCtx := adventure.NormalOperationContext{
		Requester: &requester,
		Callback:  corebattle.CallbackContext{TargetContext: targets, Damage: &damage},
	}
	opCtx.Definitions.RegisterPlayerCallbacks(skill.PlayerCallbackContexts{Revival: revival}, len(targets.PartyIDs))
	parsed, err := r.ParseNormalOperation(cmd, opCtx)
	if err != nil {
		return fx, err
	}
	if _, err := opCtx.Definitions.RevivalAction(&r.Battle, revival, parsed.First.EntryID, parsed.First.TargetID, c.f.rng); err != nil {
		return fx, err
	}
	defeat := &corebattle.DefeatContext{EquipmentResetContext: corebattle.EquipmentResetContext{TargetContext: targets, RequesterLevel: &requester.Level}}
	result, err := r.Battle.ApplyPlayerOperation(&corebattle.PlayerOperation{
		First:           parsed.First.PlayerAction,
		Tail:            corebattle.PlayerTailContext{DamageContext: damage, DefeatContext: defeat},
		ResponseEnabled: true,
	}, c.f.rng)
	if err != nil {
		return fx, err
	}
	if !result.Accepted {
		return fx, adventure.ErrRunUnavailable
	}
	if result.ResponseReached {
		if npc := r.NextNPC(c.f.clock.Now()); npc != "" {
			if err := r.RecordNPCResponse(npc, c.f.clock.Now()); err != nil {
				return fx, err
			}
		}
	}
	return fx, nil
}

func runActorByID(r *adventure.NormalRun, id string) *corebattle.LifetimeActor {
	for i := range r.Battle.Actors {
		if r.Battle.Actors[i].ID == id {
			return &r.Battle.Actors[i]
		}
	}
	return nil
}

func TestNormalRunFallenSelfRevivalAndReceiptIntegration(t *testing.T) {
	const job66Icon = "assets/images/placeholders/battle/forms.svg#job-66-"
	for _, tc := range []struct {
		name               string
		namedHP, iconHP    int
		amulet             bool
		wantErr            error
		wantHP, wantHelper int
		wantStatus         string
		wantBan            int
	}{
		{name: "missing helper", namedHP: -1, iconHP: -1, amulet: true},
		{name: "fallen named helper", namedHP: 0, iconHP: -1, amulet: true},
		{name: "fallen named blocks living appearance fallback", namedHP: 0, iconHP: 11, amulet: true, wantHelper: 11},
		{name: "fallen appearance fallback", namedHP: -1, iconHP: 0, amulet: true},
		{name: "living appearance fallback", namedHP: -1, iconHP: 11, amulet: true, wantHP: 50, wantHelper: 5, wantStatus: corebattle.StatusDofuu, wantBan: 2},
		{name: "living named helper", namedHP: 10, iconHP: -1, amulet: true, wantHP: 50, wantHelper: 5, wantStatus: corebattle.StatusDofuu, wantBan: 2},
		{name: "missing listing amulet", namedHP: 10, iconHP: -1, amulet: false, wantErr: adventure.ErrRunUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newNormalFixture(t, 2)
			f.cfg.Combat = &revivalRunCombat{normalCombat: f.combat, f: f}
			f.restart(t)
			_, lobby := f.waitingParty(t, adventure.CrawlModeNormal)
			leader, ally := f.actors[0], f.actors[1]
			entry, err := f.svc.EnterParty(t.Context(), leader.PlayerID, leader.ID, lobby)
			if err != nil {
				t.Fatal(err)
			}
			runID := entry.Command.RunID
			enemyID, namedID := "enemy-1", "calling-helper"
			var version int
			if err := f.store.WithRunForUpdate(t.Context(), runID, false, func(ctx context.Context) error {
				r, err := f.store.FindRun(ctx, runID)
				if err != nil {
					return err
				}
				r.Floor, r.Enemies = 1, []string{enemyID}
				r.Battle.SkillBans = map[string]int{"other": 2}
				self := runActorByID(r, leader.ID)
				self.HP, self.MaxHP = 0, 100
				self.Jobs = corebattle.BattleJobs{JobID: "job-66", SP: 150}
				self.CMPNeedCharge = 3
				if tc.amulet {
					self.ItemDefinitionIDs = []string{"item-158"}
				}
				if tc.iconHP >= 0 {
					fallback := runActorByID(r, ally.ID)
					fallback.Icon, fallback.HP = job66Icon, tc.iconHP
				}
				if tc.namedHP >= 0 {
					helper := self.Clone()
					helper.ID, helper.NPC, helper.RosterIdentity, helper.HP = namedID, true, "calling-66", tc.namedHP
					r.Battle.Actors = append(r.Battle.Actors, helper)
				}
				r.Battle.Actors = append(r.Battle.Actors, corebattle.LifetimeActor{
					Participant: corebattle.Participant{ID: enemyID, TeamID: "enemies", HP: 100, MaxHP: 100},
					NPC:         true,
				})
				r.Version++
				version = r.Version
				return f.store.SaveRun(ctx, *r, version-1)
			}); err != nil {
				t.Fatal(err)
			}

			f.clock.add(3 * time.Second)
			cmd := adventure.RunCommand{
				PlayerID: leader.PlayerID, ActorID: leader.ID, RunID: runID,
				ID: id.New(), Version: version, Action: "job-66-revive",
			}
			receipt, err := f.svc.Command(t.Context(), cmd)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Command err=%v want %v", err, tc.wantErr)
			}
			if tc.wantErr != nil {
				gotReceipt, rerr := f.svc.GetRunReceipt(t.Context(), leader.PlayerID, leader.ID, runID, cmd.ID)
				if rerr != nil || gotReceipt != nil {
					t.Fatalf("rejected command stored receipt: %+v %v", gotReceipt, rerr)
				}
				return
			}
			f.restart(t)
			replay, err := f.svc.Command(t.Context(), cmd)
			if err != nil || !reflect.DeepEqual(replay, receipt) {
				t.Fatalf("replay mismatch: %+v vs %+v (%v)", replay, receipt, err)
			}
			saved, err := f.store.FindRun(t.Context(), runID)
			if err != nil || saved == nil {
				t.Fatal(saved, err)
			}
			self := runActorByID(saved, leader.ID)
			if self.HP != tc.wantHP || self.Status != tc.wantStatus || self.CMPCharge != 1 ||
				saved.Battle.SkillBans["other"] != 1 || saved.Battle.SkillBans["job-66-revive"] != tc.wantBan ||
				saved.LastActions[enemyID].IsZero() {
				t.Fatalf("unexpected run state: self=%+v bans=%v last=%v", self, saved.Battle.SkillBans, saved.LastActions)
			}
			if tc.iconHP >= 0 {
				if fallback := runActorByID(saved, ally.ID); fallback.HP != tc.wantHelper {
					t.Fatalf("appearance helper HP=%d want %d", fallback.HP, tc.wantHelper)
				}
			} else if tc.namedHP > 0 {
				if named := runActorByID(saved, namedID); named == nil || named.HP != tc.wantHelper {
					t.Fatalf("named helper=%+v want HP=%d", named, tc.wantHelper)
				}
			}
			if tc.wantBan > 0 {
				f.clock.add(3 * time.Second)
				again := adventure.RunCommand{
					PlayerID: leader.PlayerID, ActorID: leader.ID, RunID: runID,
					ID: id.New(), Version: saved.Version, Action: "job-66-revive",
				}
				if _, err := f.svc.Command(t.Context(), again); !errors.Is(err, adventure.ErrRunUnavailable) {
					t.Fatalf("banned submission allowed: %v", err)
				}
			}
		})
	}
}
