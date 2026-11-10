package party

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/id"
)

func hashPassword(pass string) string {
	pass = strings.TrimSpace(pass)
	if pass == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(pass))
	return hex.EncodeToString(sum[:])
}

// CreateParty creates a new party with the creator as leader.
func (s *Service) CreateParty(ctx context.Context, leaderCharID string, req CreatePartyRequest) (PartyDetail, error) {
	if err := ValidatePartyName(req.Name); err != nil {
		return PartyDetail{}, err
	}
	mode, err := ValidatePartyMode(req.Mode)
	if err != nil {
		return PartyDetail{}, err
	}

	stage, err := s.stages.FindByID(req.StageID)
	if err != nil {
		return PartyDetail{}, ErrStageNotFound
	}

	speed := req.Speed
	if speed != 3 && speed != 18 && speed != 25 {
		speed = DefaultSpeed
	}

	maxMembers := req.MaxMembers
	if maxMembers <= 0 {
		maxMembers = MaxPartyMembers
	}
	if maxMembers < MinPartyMembers || maxMembers > MaxPartyMembers {
		return PartyDetail{}, ErrInvalidMaxMembers
	}

	minLevel := req.MinLevel
	if minLevel <= 0 {
		minLevel = 1
	}
	maxLevel := req.MaxLevel
	if maxLevel <= 0 {
		maxLevel = 999
	}
	minHP := req.MinHP
	if minHP < 0 {
		minHP = 0
	}

	var createdParty Party
	var createdMember Member
	var lockToken string
	var lockAcquired bool
	var committed bool

	defer func() {
		if !committed && lockAcquired && s.timer != nil {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			//lint:ignore error-swallow compensatory unlock on aborted transaction
			_, _ = s.timer.ReleaseLockWithToken(cleanupCtx, timer.CategoryDungeonOnce, leaderCharID, lockToken)
		}
	}()

	err = s.runInTx(ctx, func(txCtx context.Context) error {
		// Check if leader already belongs to an active party
		_, _, err := s.repo.GetActivePartyByCharacter(txCtx, leaderCharID)
		if err == nil {
			return ErrAlreadyInParty
		}

		leaderChar, err := s.charRepo.FindByIDForUpdate(txCtx, leaderCharID)
		if err != nil {
			return err
		}
		if s.activityCheck != nil {
			if err := s.activityCheck(txCtx, leaderCharID); err != nil {
				return err
			}
		}
		if leaderChar.Stats.HP <= 0 {
			return ErrCharacterUnconscious
		}
		if leaderChar.Tired >= 100 {
			return ErrCharacterExhausted
		}
		if err := s.stages.CanAccessStage(leaderChar, stage.ID); err != nil {
			return err
		}
		if stage.ID == "stage-26" && s.timer != nil {
			lockToken = id.New()
			acquired, err := s.timer.TryLockWithToken(txCtx, timer.CategoryDungeonOnce, leaderChar.ID, lockToken, 0)
			if err != nil {
				return err
			}
			if !acquired {
				return fmt.Errorf("%w: %sはすでに本日の限定ダンジョン探索を終えています。自宅で睡眠をとると再度挑戦できます", ErrDailyOnceAlreadyUsed, leaderChar.Name)
			}
			lockAcquired = true
		}
		if req.NeedJoin != "" {
			if err := ValidateNeedJoin(req.NeedJoin, leaderChar); err != nil {
				return err
			}
		}

		now := time.Now().UTC()
		partyID := id.New()
		passHash := hashPassword(req.Password)

		chosenSeason := ""
		if len(stage.Seasons) > 0 {
			chosenSeason = stage.SelectSeason(s.rng).Season
		}

		createdParty = Party{
			ID:                partyID,
			LeaderCharacterID: leaderChar.ID,
			Name:              strings.TrimSpace(req.Name),
			PasswordHash:      passHash,
			StageID:           stage.ID,
			Season:            chosenSeason,
			Mode:              mode,
			Speed:             speed,
			MaxMembers:        maxMembers,
			MinLevel:          minLevel,
			MaxLevel:          maxLevel,
			MinHP:             minHP,
			NeedJoin:          strings.TrimSpace(req.NeedJoin),
			Status:            StatusRecruiting,
			CreatedAt:         now,
			UpdatedAt:         now,
		}
		if err := s.repo.SaveParty(txCtx, createdParty); err != nil {
			return err
		}

		createdMember = Member{
			PartyID:       partyID,
			CharacterID:   leaderChar.ID,
			CharacterName: leaderChar.Name,
			JobID:         leaderChar.JobID,
			Level:         leaderChar.Level,
			HP:            leaderChar.Stats.HP,
			MaxHP:         leaderChar.Stats.MaxHP,
			IsLeader:      true,
			ReadyState:    true,
			JoinedAt:      now,
		}
		return s.repo.AddMember(txCtx, createdMember)
	})
	if err != nil {
		return PartyDetail{}, err
	}
	committed = true

	if s.news != nil {
		//lint:ignore error-swallow best-effort informational news after party creation commits
		_ = s.news.PublishNews(
			ctx,
			"party",
			fmt.Sprintf("パーティー『%s』が結成されました！", createdParty.Name),
			fmt.Sprintf("冒険場所：%s、募集人数：%d人", stage.Name, createdParty.MaxMembers),
			createdMember.CharacterName,
			time.Now().UTC(),
		)
	}

	return PartyDetail{
		Party:   createdParty,
		Members: []Member{createdMember},
	}, nil
}
