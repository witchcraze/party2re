package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/witchcraze/party2re/internal/character"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

type CharacterRepository struct {
	db *sql.DB
}

func NewCharacterRepository(db *sql.DB) (*CharacterRepository, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}
	return &CharacterRepository{db: db}, nil
}

func (r *CharacterRepository) Save(ctx context.Context, value corecharacter.Character) error {
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		INSERT INTO characters
			(id, player_id, name, job_id, gender, max_hp, max_mp, hp, mp, attack, defense, agility, money, level, experience, rebirth_count, small_medals, help_count, over_level, over_depot, over_monster, over_future, over_flea, over_store)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, value.ID, value.PlayerID, value.Name, value.JobID, value.Gender, value.Stats.MaxHP, value.Stats.MaxMP,
		value.Stats.HP, value.Stats.MP, value.Stats.Attack, value.Stats.Defense, value.Stats.Agility,
		value.Money, value.Level, value.Experience, value.RebirthCount, value.SmallMedals, value.HelpCount,
		value.OverLevel, value.OverDepot, value.OverMonster, value.OverFuture, value.OverFlea, value.OverStore)
	return err
}

func (r *CharacterRepository) FindByID(ctx context.Context, id string) (corecharacter.Character, error) {
	return r.findByIDWithQuery(ctx, id, `
		SELECT `+characterColumns+`
		FROM characters
		WHERE id = ?
	`)
}

func (r *CharacterRepository) FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error) {
	return r.findByIDWithQuery(ctx, id, `
		SELECT `+characterColumns+`
		FROM characters
		WHERE id = ? FOR UPDATE
	`)
}

func (r *CharacterRepository) FindByName(ctx context.Context, name string) (corecharacter.Character, error) {
	return scanCharacterRow(ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT `+characterColumns+`
		FROM characters
		WHERE name = ?
	`, name))
}

func (r *CharacterRepository) FindByNameForUpdate(ctx context.Context, name string) (corecharacter.Character, error) {
	return scanCharacterRow(ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT `+characterColumns+`
		FROM characters
		WHERE name = ? FOR UPDATE
	`, name))
}

func (r *CharacterRepository) findByIDWithQuery(ctx context.Context, id string, query string) (corecharacter.Character, error) {
	return scanCharacterRow(ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, query, id))
}

func (r *CharacterRepository) FindByPlayerID(ctx context.Context, playerID string) ([]corecharacter.Character, error) {
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, `
		SELECT `+characterColumns+`
		FROM characters
		WHERE player_id = ?
		ORDER BY created_at ASC
	`, playerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanCharacterRows(rows)
}

func (r *CharacterRepository) Update(ctx context.Context, value corecharacter.Character) error {
	return updateCharacter(ctx, ExecutorFromContext(ctx, r.db), value)
}

func (r *CharacterRepository) GetProfile(ctx context.Context, characterID string) (character.Profile, error) {
	var (
		charID    string
		comment   string
		avatarURL string
		rawBio    sql.NullString
		updatedAt time.Time
	)
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT character_id, comment, avatar_url, bio_data, updated_at
		FROM character_profiles
		WHERE character_id = ?
	`, characterID).Scan(&charID, &comment, &avatarURL, &rawBio, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return character.Profile{
			CharacterID: characterID,
			BioData:     make(map[string]string),
			UpdatedAt:   time.Now().UTC(),
		}, nil
	}
	if err != nil {
		return character.Profile{}, err
	}

	bio := make(map[string]string)
	if rawBio.Valid && rawBio.String != "" {
		_ = json.Unmarshal([]byte(rawBio.String), &bio)
	}

	return character.Profile{
		CharacterID: charID,
		Comment:     comment,
		AvatarURL:   avatarURL,
		BioData:     bio,
		UpdatedAt:   updatedAt,
	}, nil
}

func (r *CharacterRepository) SaveProfile(ctx context.Context, profile character.Profile) error {
	var bioJSON []byte
	var err error
	if len(profile.BioData) > 0 {
		bioJSON, err = json.Marshal(profile.BioData)
		if err != nil {
			return err
		}
	}

	_, err = ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		INSERT INTO character_profiles (character_id, comment, avatar_url, bio_data, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			comment = VALUES(comment),
			avatar_url = VALUES(avatar_url),
			bio_data = VALUES(bio_data),
			updated_at = VALUES(updated_at)
	`, profile.CharacterID, profile.Comment, profile.AvatarURL, bioJSON, profile.UpdatedAt)
	return err
}

func (r *CharacterRepository) Delete(ctx context.Context, id string) error {
	exec := ExecutorFromContext(ctx, r.db)

	queries := []string{
		`DELETE FROM casino_accounts WHERE character_id = ?`,
		`DELETE FROM casino_poker_sessions WHERE character_id = ?`,
		`DELETE FROM character_lottery WHERE character_id = ?`,
		`DELETE FROM lottery_tickets WHERE character_id = ?`,
		`DELETE FROM farm_plots WHERE character_id = ?`,
		`DELETE FROM character_blessings WHERE character_id = ?`,
		`DELETE FROM banquet_toasts WHERE character_id = ?`,
		`DELETE FROM blackmarket_character_points WHERE character_id = ?`,
		`DELETE FROM blackmarket_character_purchases WHERE character_id = ?`,
		`DELETE FROM tavern_deliveries WHERE character_id = ?`,
		`DELETE FROM tavern_character_status WHERE character_id = ?`,
		`DELETE FROM park_posts WHERE character_id = ?`,
		`DELETE FROM rescue_records WHERE character_id = ?`,
		`DELETE FROM contest_votes WHERE voter_character_id = ?`,
		`DELETE FROM contest_entries WHERE character_id = ?`,
		`DELETE FROM character_photos WHERE character_id = ?`,
		`DELETE FROM character_deliveries WHERE character_id = ?`,
		`DELETE FROM delivery_parcels WHERE sender_character_id = ? OR recipient_character_id = ?`,
		`DELETE FROM fleamarket_listings WHERE seller_character_id = ?`,
		`DELETE FROM auction_listings WHERE seller_character_id = ?`,
		`DELETE FROM character_letters WHERE sender_character_id = ? OR recipient_character_id = ?`,
		`DELETE FROM character_companion_phrases WHERE character_id = ?`,
		`DELETE FROM character_delivery_notices WHERE character_id = ?`,
		`DELETE FROM character_homes WHERE character_id = ?`,
		`DELETE FROM character_boss_records WHERE character_id = ?`,
		`DELETE FROM boss_challenge_history WHERE character_id = ?`,
		`DELETE FROM character_dungeon_records WHERE character_id = ?`,
		`DELETE FROM dungeon_active_expeditions WHERE character_id = ?`,
		`DELETE FROM dungeon_expedition_history WHERE character_id = ?`,
		`DELETE FROM character_challenge_records WHERE character_id = ?`,
		`DELETE FROM challenge_sessions WHERE character_id = ?`,
		`DELETE FROM arena_ratings WHERE character_id = ?`,
		`DELETE FROM arena_matches WHERE attacker_id = ? OR defender_id = ?`,
		`DELETE FROM character_monsters WHERE character_id = ?`,
		`DELETE FROM character_monster_book WHERE character_id = ?`,
		`DELETE FROM character_item_collection WHERE character_id = ?`,
		`DELETE FROM character_medals WHERE character_id = ?`,
		`DELETE FROM character_achievements WHERE character_id = ?`,
		`DELETE FROM guild_members WHERE character_id = ?`,
		`UPDATE guilds SET leader_character_id = NULL WHERE leader_character_id = ?`,
		`DELETE FROM activities WHERE character_id = ?`,
		`DELETE FROM adventures WHERE character_id = ?`,
		`DELETE FROM character_custom_skills WHERE character_id = ?`,
		`DELETE FROM equipment_slots WHERE character_id = ?`,
		`DELETE FROM inventory_items WHERE character_id = ?`,
		`DELETE FROM depot_items WHERE character_id = ?`,
		`DELETE FROM character_depots WHERE character_id = ?`,
		`DELETE FROM character_job_masteries WHERE character_id = ?`,
		`DELETE FROM character_job_history WHERE character_id = ?`,
		`DELETE FROM character_jobs WHERE character_id = ?`,
		`DELETE FROM character_profiles WHERE character_id = ?`,
		`DELETE FROM characters WHERE id = ?`,
	}

	for _, q := range queries {
		if strings.Count(q, "?") == 2 {
			if _, err := exec.ExecContext(ctx, q, id, id); err != nil {
				return err
			}
		} else {
			if _, err := exec.ExecContext(ctx, q, id); err != nil {
				return err
			}
		}
	}

	return nil
}
