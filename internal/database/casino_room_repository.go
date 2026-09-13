package database

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/witchcraze/party2re/internal/casino"
)

type CasinoRoomRepository struct {
	db *sql.DB
}

func NewCasinoRoomRepository(db *sql.DB) (*CasinoRoomRepository, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}
	return &CasinoRoomRepository{db: db}, nil
}

func (r *CasinoRoomRepository) CreateRoom(ctx context.Context, room casino.Room, leader casino.RoomMember) error {
	exec := ExecutorFromContext(ctx, r.db)

	_, err := exec.ExecContext(ctx, `
		INSERT INTO casino_rooms (
			id, name, game_type, leader_character_id, speed, max_players, rate,
			password_hash, has_password, allow_spectators, status, round,
			current_bet, max_bet, pot, winner_character_id, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		room.ID, room.Name, string(room.GameType), room.LeaderCharacterID, int(room.Speed),
		room.MaxPlayers, room.Rate, room.PasswordHash, room.HasPassword, room.AllowSpectators,
		string(room.Status), room.Round, room.CurrentBet, room.MaxBet, room.Pot,
		room.WinnerCharacterID, room.CreatedAt, room.UpdatedAt,
	)
	if err != nil {
		return err
	}

	_, err = exec.ExecContext(ctx, `
		INSERT INTO casino_members (
			room_id, character_id, is_spectator, action, card, joined_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`,
		leader.RoomID, leader.CharacterID, leader.IsSpectator, leader.Action,
		leader.Card, leader.JoinedAt, leader.UpdatedAt,
	)
	return err
}

const casinoRoomColumns = `
	id, name, game_type, leader_character_id, speed, max_players, rate,
	password_hash, has_password, allow_spectators, status, round,
	current_bet, max_bet, pot, winner_character_id, created_at, updated_at
`

func scanRoom(row interface {
	Scan(dest ...any) error
}) (*casino.Room, error) {
	var room casino.Room
	var gameType, status string
	var speed int
	var winner *string

	err := row.Scan(
		&room.ID, &room.Name, &gameType, &room.LeaderCharacterID, &speed,
		&room.MaxPlayers, &room.Rate, &room.PasswordHash, &room.HasPassword,
		&room.AllowSpectators, &status, &room.Round, &room.CurrentBet,
		&room.MaxBet, &room.Pot, &winner, &room.CreatedAt, &room.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, casino.ErrRoomNotFound
	}
	if err != nil {
		return nil, err
	}

	room.GameType = casino.GameType(gameType)
	room.Status = casino.RoomStatus(status)
	room.Speed = casino.RoomSpeed(speed)
	room.WinnerCharacterID = winner
	return &room, nil
}

func (r *CasinoRoomRepository) GetRoom(ctx context.Context, roomID string) (*casino.Room, error) {
	row := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT `+casinoRoomColumns+`
		FROM casino_rooms
		WHERE id = ?
	`, roomID)
	return scanRoom(row)
}

func (r *CasinoRoomRepository) GetRoomByName(ctx context.Context, name string) (*casino.Room, error) {
	row := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT `+casinoRoomColumns+`
		FROM casino_rooms
		WHERE name = ?
	`, name)
	return scanRoom(row)
}

func (r *CasinoRoomRepository) GetRoomForUpdate(ctx context.Context, roomID string) (*casino.Room, error) {
	row := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT `+casinoRoomColumns+`
		FROM casino_rooms
		WHERE id = ?
		FOR UPDATE
	`, roomID)
	return scanRoom(row)
}

func (r *CasinoRoomRepository) ListActiveRooms(ctx context.Context) ([]casino.RoomDetail, error) {
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, `
		SELECT `+casinoRoomColumns+`
		FROM casino_rooms
		WHERE status != 'disbanded'
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var details []casino.RoomDetail
	for rows.Next() {
		room, err := scanRoom(rows)
		if err != nil {
			return nil, err
		}
		members, err := r.ListMembers(ctx, room.ID)
		if err != nil {
			return nil, err
		}
		details = append(details, casino.RoomDetail{
			Room:    *room,
			Members: members,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return details, nil
}

func (r *CasinoRoomRepository) UpdateRoom(ctx context.Context, room casino.Room) error {
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		UPDATE casino_rooms
		SET name = ?,
		    leader_character_id = ?,
		    speed = ?,
		    max_players = ?,
		    rate = ?,
		    password_hash = ?,
		    has_password = ?,
		    allow_spectators = ?,
		    status = ?,
		    round = ?,
		    current_bet = ?,
		    max_bet = ?,
		    pot = ?,
		    winner_character_id = ?,
		    updated_at = ?
		WHERE id = ?
	`,
		room.Name, room.LeaderCharacterID, int(room.Speed), room.MaxPlayers, room.Rate,
		room.PasswordHash, room.HasPassword, room.AllowSpectators, string(room.Status),
		room.Round, room.CurrentBet, room.MaxBet, room.Pot, room.WinnerCharacterID,
		time.Now().UTC(), room.ID,
	)
	return err
}

func (r *CasinoRoomRepository) AddMember(ctx context.Context, member casino.RoomMember) error {
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		INSERT INTO casino_members (
			room_id, character_id, is_spectator, action, card, joined_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`,
		member.RoomID, member.CharacterID, member.IsSpectator, member.Action,
		member.Card, member.JoinedAt, member.UpdatedAt,
	)
	return err
}

func scanMember(row interface {
	Scan(dest ...any) error
}) (*casino.RoomMember, error) {
	var m casino.RoomMember
	err := row.Scan(
		&m.RoomID, &m.CharacterID, &m.IsSpectator, &m.Action,
		&m.Card, &m.JoinedAt, &m.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, casino.ErrMemberNotFound
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *CasinoRoomRepository) GetMember(ctx context.Context, roomID string, characterID string) (*casino.RoomMember, error) {
	row := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT room_id, character_id, is_spectator, action, card, joined_at, updated_at
		FROM casino_members
		WHERE room_id = ? AND character_id = ?
	`, roomID, characterID)
	return scanMember(row)
}

func (r *CasinoRoomRepository) GetMemberForUpdate(ctx context.Context, roomID string, characterID string) (*casino.RoomMember, error) {
	row := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT room_id, character_id, is_spectator, action, card, joined_at, updated_at
		FROM casino_members
		WHERE room_id = ? AND character_id = ?
		FOR UPDATE
	`, roomID, characterID)
	return scanMember(row)
}

func (r *CasinoRoomRepository) ListMembers(ctx context.Context, roomID string) ([]casino.RoomMember, error) {
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, `
		SELECT cm.room_id, cm.character_id, cm.is_spectator, cm.action, cm.card,
		       cm.joined_at, cm.updated_at, c.name
		FROM casino_members cm
		JOIN characters c ON cm.character_id = c.id
		WHERE cm.room_id = ?
		ORDER BY cm.joined_at ASC
	`, roomID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []casino.RoomMember
	for rows.Next() {
		var m casino.RoomMember
		var charName string
		if err := rows.Scan(
			&m.RoomID, &m.CharacterID, &m.IsSpectator, &m.Action,
			&m.Card, &m.JoinedAt, &m.UpdatedAt, &charName,
		); err != nil {
			return nil, err
		}
		m.CharacterName = charName
		members = append(members, m)
	}
	return members, rows.Err()
}

func (r *CasinoRoomRepository) ListMembersForUpdate(ctx context.Context, roomID string) ([]casino.RoomMember, error) {
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, `
		SELECT room_id, character_id, is_spectator, action, card, joined_at, updated_at
		FROM casino_members
		WHERE room_id = ?
		ORDER BY joined_at ASC
		FOR UPDATE
	`, roomID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []casino.RoomMember
	for rows.Next() {
		m, err := scanMember(rows)
		if err != nil {
			return nil, err
		}
		members = append(members, *m)
	}
	return members, rows.Err()
}

func (r *CasinoRoomRepository) UpdateMember(ctx context.Context, member casino.RoomMember) error {
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		UPDATE casino_members
		SET is_spectator = ?,
		    action = ?,
		    card = ?,
		    updated_at = ?
		WHERE room_id = ? AND character_id = ?
	`, member.IsSpectator, member.Action, member.Card, time.Now().UTC(), member.RoomID, member.CharacterID)
	return err
}

func (r *CasinoRoomRepository) RemoveMember(ctx context.Context, roomID string, characterID string) error {
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		DELETE FROM casino_members
		WHERE room_id = ? AND character_id = ?
	`, roomID, characterID)
	return err
}

func (r *CasinoRoomRepository) DeleteRoom(ctx context.Context, roomID string) error {
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		DELETE FROM casino_rooms
		WHERE id = ?
	`, roomID)
	return err
}
