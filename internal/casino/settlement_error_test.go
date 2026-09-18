package casino_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/witchcraze/party2re/internal/casino"
)

type trackingTxProvider struct {
	rollbacks int
	commits   int
}

func (p *trackingTxProvider) RunInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	err := fn(ctx)
	if err != nil {
		p.rollbacks++
		return err
	}
	p.commits++
	return nil
}

type errorInjectingRoomRepo struct {
	casino.RoomRepository
	failUpdateMember error
	failRemoveMember error
}

func (r *errorInjectingRoomRepo) UpdateMember(ctx context.Context, member casino.RoomMember) error {
	if r.failUpdateMember != nil {
		return r.failUpdateMember
	}
	return r.RoomRepository.UpdateMember(ctx, member)
}

func (r *errorInjectingRoomRepo) RemoveMember(ctx context.Context, roomID string, characterID string) error {
	if r.failRemoveMember != nil {
		return r.failRemoveMember
	}
	return r.RoomRepository.RemoveMember(ctx, roomID, characterID)
}

type errorInjectingCasinoRepo struct {
	*mockPrizeCasinoRepo
	failGetAccount error
}

func (r *errorInjectingCasinoRepo) GetAccount(ctx context.Context, characterID string) (casino.Account, error) {
	if r.failGetAccount != nil {
		return casino.Account{}, r.failGetAccount
	}
	return r.mockPrizeCasinoRepo.GetAccount(ctx, characterID)
}

func TestMultiplayerHighLow_Settlement_UpdateMemberErrorRollback(t *testing.T) {
	ctx := context.Background()
	rawCasinoRepo := newMockPrizeCasinoRepo()
	casinoRepo := &errorInjectingCasinoRepo{mockPrizeCasinoRepo: rawCasinoRepo}
	memRoomRepo := newMockMemoryRoomRepo()
	roomRepo := &errorInjectingRoomRepo{RoomRepository: memRoomRepo}
	txProvider := &trackingTxProvider{}

	svc, err := casino.NewService(
		casinoRepo,
		casino.WithRoomRepository(roomRepo),
		casino.WithTransactionProvider(txProvider),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	p1 := "hl-err-p1"
	p2 := "hl-err-p2"
	casinoRepo.accounts[p1] = casino.Account{CharacterID: p1, Coins: 500}
	casinoRepo.accounts[p2] = casino.Account{CharacterID: p2, Coins: 500}

	detail, err := svc.CreateRoom(ctx, p1, casino.CreateRoomRequest{
		Name:       "HLErrorRoom",
		GameType:   casino.GameTypeHighLow,
		Speed:      casino.SpeedFast,
		MaxPlayers: 2,
		Rate:       10,
	})
	if err != nil {
		t.Fatalf("CreateRoom failed: %v", err)
	}
	roomID := detail.Room.ID

	if _, err := svc.JoinRoom(ctx, roomID, p2, "", 0); err != nil {
		t.Fatalf("JoinRoom failed: %v", err)
	}
	if _, err := svc.StartHighLow(ctx, roomID, p1); err != nil {
		t.Fatalf("StartHighLow failed: %v", err)
	}

	// P1 plays High
	if _, err := svc.PlayHighLowAction(ctx, roomID, p1, casino.HighLowActionHigh); err != nil {
		t.Fatalf("P1 action failed: %v", err)
	}

	// Inject UpdateMember error right before P2 triggers showdown
	expectedErr := errors.New("injected update member error")
	roomRepo.failUpdateMember = expectedErr

	_, err = svc.PlayHighLowAction(ctx, roomID, p2, casino.HighLowActionHigh)
	if err == nil {
		t.Fatal("expected error on showdown UpdateMember failure, got nil")
	}
	if !strings.Contains(err.Error(), "injected update member error") {
		t.Errorf("expected error containing injected message, got: %v", err)
	}
	if txProvider.rollbacks == 0 {
		t.Error("expected transaction rollback on showdown error, got 0 rollbacks")
	}
}

func TestMultiplayerHighLow_Settlement_RemoveMemberErrorRollback(t *testing.T) {
	ctx := context.Background()
	rawCasinoRepo := newMockPrizeCasinoRepo()
	casinoRepo := &errorInjectingCasinoRepo{mockPrizeCasinoRepo: rawCasinoRepo}
	memRoomRepo := newMockMemoryRoomRepo()
	roomRepo := &errorInjectingRoomRepo{RoomRepository: memRoomRepo}
	txProvider := &trackingTxProvider{}

	svc, err := casino.NewService(
		casinoRepo,
		casino.WithRoomRepository(roomRepo),
		casino.WithTransactionProvider(txProvider),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	p1 := "hl-elim-p1"
	p2 := "hl-elim-p2"
	casinoRepo.accounts[p1] = casino.Account{CharacterID: p1, Coins: 500}
	casinoRepo.accounts[p2] = casino.Account{CharacterID: p2, Coins: 10} // Will have 0 coins after betting 10

	detail, err := svc.CreateRoom(ctx, p1, casino.CreateRoomRequest{
		Name:       "HLElimRoom",
		GameType:   casino.GameTypeHighLow,
		Speed:      casino.SpeedFast,
		MaxPlayers: 2,
		Rate:       10,
	})
	if err != nil {
		t.Fatalf("CreateRoom failed: %v", err)
	}
	roomID := detail.Room.ID

	if _, err := svc.JoinRoom(ctx, roomID, p2, "", 0); err != nil {
		t.Fatalf("JoinRoom failed: %v", err)
	}
	if _, err := svc.StartHighLow(ctx, roomID, p1); err != nil {
		t.Fatalf("StartHighLow failed: %v", err)
	}

	// Inject cards: P1 has 9 (Q), P2 has 3 (4). P1 wins showdown!
	m1, _ := memRoomRepo.GetMember(ctx, roomID, p1)
	m1.Card = 9
	_ = memRoomRepo.UpdateMember(ctx, *m1)

	m2, _ := memRoomRepo.GetMember(ctx, roomID, p2)
	m2.Card = 3
	_ = memRoomRepo.UpdateMember(ctx, *m2)

	if _, err := svc.PlayHighLowAction(ctx, roomID, p1, casino.HighLowActionHigh); err != nil {
		t.Fatalf("P1 action failed: %v", err)
	}

	// Inject RemoveMember failure for eliminated member P2
	expectedErr := errors.New("injected remove member error")
	roomRepo.failRemoveMember = expectedErr

	_, err = svc.PlayHighLowAction(ctx, roomID, p2, casino.HighLowActionHigh)
	if err == nil {
		t.Fatal("expected error on showdown RemoveMember failure, got nil")
	}
	if !strings.Contains(err.Error(), "injected remove member error") {
		t.Errorf("expected error containing injected message, got: %v", err)
	}
	if txProvider.rollbacks == 0 {
		t.Error("expected transaction rollback on showdown RemoveMember error, got 0 rollbacks")
	}
}

func TestMultiplayerHighLow_Settlement_GetAccountErrorRollback(t *testing.T) {
	ctx := context.Background()
	rawCasinoRepo := newMockPrizeCasinoRepo()
	casinoRepo := &errorInjectingCasinoRepo{mockPrizeCasinoRepo: rawCasinoRepo}
	memRoomRepo := newMockMemoryRoomRepo()
	roomRepo := &errorInjectingRoomRepo{RoomRepository: memRoomRepo}
	txProvider := &trackingTxProvider{}

	svc, err := casino.NewService(
		casinoRepo,
		casino.WithRoomRepository(roomRepo),
		casino.WithTransactionProvider(txProvider),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	p1 := "hl-acc-p1"
	p2 := "hl-acc-p2"
	casinoRepo.accounts[p1] = casino.Account{CharacterID: p1, Coins: 500}
	casinoRepo.accounts[p2] = casino.Account{CharacterID: p2, Coins: 500}

	detail, err := svc.CreateRoom(ctx, p1, casino.CreateRoomRequest{
		Name:       "HLAccErrRoom",
		GameType:   casino.GameTypeHighLow,
		Speed:      casino.SpeedFast,
		MaxPlayers: 2,
		Rate:       10,
	})
	if err != nil {
		t.Fatalf("CreateRoom failed: %v", err)
	}
	roomID := detail.Room.ID

	if _, err := svc.JoinRoom(ctx, roomID, p2, "", 0); err != nil {
		t.Fatalf("JoinRoom failed: %v", err)
	}
	if _, err := svc.StartHighLow(ctx, roomID, p1); err != nil {
		t.Fatalf("StartHighLow failed: %v", err)
	}

	if _, err := svc.PlayHighLowAction(ctx, roomID, p1, casino.HighLowActionHigh); err != nil {
		t.Fatalf("P1 action failed: %v", err)
	}

	// Inject GetAccount error during showdown coin check
	expectedErr := errors.New("injected get account error")
	casinoRepo.failGetAccount = expectedErr

	_, err = svc.PlayHighLowAction(ctx, roomID, p2, casino.HighLowActionHigh)
	if err == nil {
		t.Fatal("expected error on showdown GetAccount failure, got nil")
	}
	if !strings.Contains(err.Error(), "injected get account error") {
		t.Errorf("expected error containing injected message, got: %v", err)
	}
	if txProvider.rollbacks == 0 {
		t.Error("expected transaction rollback on showdown GetAccount error, got 0 rollbacks")
	}
}

func TestMultiplayerDoppel_Settlement_UpdateMemberErrorRollback(t *testing.T) {
	ctx := context.Background()
	rawCasinoRepo := newMockPrizeCasinoRepo()
	casinoRepo := &errorInjectingCasinoRepo{mockPrizeCasinoRepo: rawCasinoRepo}
	memRoomRepo := newMockMemoryRoomRepo()
	roomRepo := &errorInjectingRoomRepo{RoomRepository: memRoomRepo}
	txProvider := &trackingTxProvider{}

	svc, err := casino.NewService(
		casinoRepo,
		casino.WithRoomRepository(roomRepo),
		casino.WithTransactionProvider(txProvider),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	p1 := "dp-err-p1"
	p2 := "dp-err-p2"
	casinoRepo.accounts[p1] = casino.Account{CharacterID: p1, Coins: 500}
	casinoRepo.accounts[p2] = casino.Account{CharacterID: p2, Coins: 500}

	detail, err := svc.CreateRoom(ctx, p1, casino.CreateRoomRequest{
		Name:       "DpErrorRoom",
		GameType:   casino.GameTypeDoppel,
		Speed:      casino.SpeedFast,
		MaxPlayers: 2,
		Rate:       10,
	})
	if err != nil {
		t.Fatalf("CreateRoom failed: %v", err)
	}
	roomID := detail.Room.ID

	if _, err := svc.JoinRoom(ctx, roomID, p2, "", 0); err != nil {
		t.Fatalf("JoinRoom failed: %v", err)
	}
	if _, err := svc.StartDoppel(ctx, roomID, p1); err != nil {
		t.Fatalf("StartDoppel failed: %v", err)
	}

	if _, err := svc.PlayDoppelAction(ctx, roomID, p1, 0); err != nil {
		t.Fatalf("P1 action failed: %v", err)
	}

	expectedErr := errors.New("injected doppel update member error")
	roomRepo.failUpdateMember = expectedErr

	_, err = svc.PlayDoppelAction(ctx, roomID, p2, 1)
	if err == nil {
		t.Fatal("expected error on showdown UpdateMember failure, got nil")
	}
	if !strings.Contains(err.Error(), "injected doppel update member error") {
		t.Errorf("expected error containing injected message, got: %v", err)
	}
	if txProvider.rollbacks == 0 {
		t.Error("expected transaction rollback on showdown error, got 0 rollbacks")
	}
}

func TestMultiplayerDoppel_Settlement_RemoveMemberErrorRollback(t *testing.T) {
	ctx := context.Background()
	rawCasinoRepo := newMockPrizeCasinoRepo()
	casinoRepo := &errorInjectingCasinoRepo{mockPrizeCasinoRepo: rawCasinoRepo}
	memRoomRepo := newMockMemoryRoomRepo()
	roomRepo := &errorInjectingRoomRepo{RoomRepository: memRoomRepo}
	txProvider := &trackingTxProvider{}

	svc, err := casino.NewService(
		casinoRepo,
		casino.WithRoomRepository(roomRepo),
		casino.WithTransactionProvider(txProvider),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	p1 := "dp-elim-p1"
	p2 := "dp-elim-p2"
	casinoRepo.accounts[p1] = casino.Account{CharacterID: p1, Coins: 500}
	casinoRepo.accounts[p2] = casino.Account{CharacterID: p2, Coins: 10} // Will have 0 coins after betting 10

	detail, err := svc.CreateRoom(ctx, p1, casino.CreateRoomRequest{
		Name:       "DpElimRoom",
		GameType:   casino.GameTypeDoppel,
		Speed:      casino.SpeedFast,
		MaxPlayers: 2,
		Rate:       10,
	})
	if err != nil {
		t.Fatalf("CreateRoom failed: %v", err)
	}
	roomID := detail.Room.ID

	if _, err := svc.JoinRoom(ctx, roomID, p2, "", 0); err != nil {
		t.Fatalf("JoinRoom failed: %v", err)
	}
	if _, err := svc.StartDoppel(ctx, roomID, p1); err != nil {
		t.Fatalf("StartDoppel failed: %v", err)
	}

	// P1 selects 0, P2 selects 1 -> child does not match leader mark, leader wins pot, P2 has 0 coins left
	if _, err := svc.PlayDoppelAction(ctx, roomID, p1, 0); err != nil {
		t.Fatalf("P1 action failed: %v", err)
	}

	expectedErr := errors.New("injected doppel remove member error")
	roomRepo.failRemoveMember = expectedErr

	_, err = svc.PlayDoppelAction(ctx, roomID, p2, 1)
	if err == nil {
		t.Fatal("expected error on showdown RemoveMember failure, got nil")
	}
	if !strings.Contains(err.Error(), "injected doppel remove member error") {
		t.Errorf("expected error containing injected message, got: %v", err)
	}
	if txProvider.rollbacks == 0 {
		t.Error("expected transaction rollback on showdown RemoveMember error, got 0 rollbacks")
	}
}

func TestMultiplayerDoppel_Settlement_GetAccountErrorRollback(t *testing.T) {
	ctx := context.Background()
	rawCasinoRepo := newMockPrizeCasinoRepo()
	casinoRepo := &errorInjectingCasinoRepo{mockPrizeCasinoRepo: rawCasinoRepo}
	memRoomRepo := newMockMemoryRoomRepo()
	roomRepo := &errorInjectingRoomRepo{RoomRepository: memRoomRepo}
	txProvider := &trackingTxProvider{}

	svc, err := casino.NewService(
		casinoRepo,
		casino.WithRoomRepository(roomRepo),
		casino.WithTransactionProvider(txProvider),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	p1 := "dp-acc-p1"
	p2 := "dp-acc-p2"
	casinoRepo.accounts[p1] = casino.Account{CharacterID: p1, Coins: 500}
	casinoRepo.accounts[p2] = casino.Account{CharacterID: p2, Coins: 500}

	detail, err := svc.CreateRoom(ctx, p1, casino.CreateRoomRequest{
		Name:       "DpAccErrRoom",
		GameType:   casino.GameTypeDoppel,
		Speed:      casino.SpeedFast,
		MaxPlayers: 2,
		Rate:       10,
	})
	if err != nil {
		t.Fatalf("CreateRoom failed: %v", err)
	}
	roomID := detail.Room.ID

	if _, err := svc.JoinRoom(ctx, roomID, p2, "", 0); err != nil {
		t.Fatalf("JoinRoom failed: %v", err)
	}
	if _, err := svc.StartDoppel(ctx, roomID, p1); err != nil {
		t.Fatalf("StartDoppel failed: %v", err)
	}

	if _, err := svc.PlayDoppelAction(ctx, roomID, p1, 0); err != nil {
		t.Fatalf("P1 action failed: %v", err)
	}

	expectedErr := errors.New("injected doppel get account error")
	casinoRepo.failGetAccount = expectedErr

	_, err = svc.PlayDoppelAction(ctx, roomID, p2, 1)
	if err == nil {
		t.Fatal("expected error on showdown GetAccount failure, got nil")
	}
	if !strings.Contains(err.Error(), "injected doppel get account error") {
		t.Errorf("expected error containing injected message, got: %v", err)
	}
	if txProvider.rollbacks == 0 {
		t.Error("expected transaction rollback on showdown GetAccount error, got 0 rollbacks")
	}
}

func TestMultiplayerIndianPoker_Settlement_UpdateMemberErrorRollback(t *testing.T) {
	ctx := context.Background()
	rawCasinoRepo := newMockPrizeCasinoRepo()
	casinoRepo := &errorInjectingCasinoRepo{mockPrizeCasinoRepo: rawCasinoRepo}
	memRoomRepo := newMockMemoryRoomRepo()
	roomRepo := &errorInjectingRoomRepo{RoomRepository: memRoomRepo}
	txProvider := &trackingTxProvider{}

	svc, err := casino.NewService(
		casinoRepo,
		casino.WithRoomRepository(roomRepo),
		casino.WithTransactionProvider(txProvider),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	p1 := "ip-err-p1"
	p2 := "ip-err-p2"
	casinoRepo.accounts[p1] = casino.Account{CharacterID: p1, Coins: 500}
	casinoRepo.accounts[p2] = casino.Account{CharacterID: p2, Coins: 500}

	detail, err := svc.CreateRoom(ctx, p1, casino.CreateRoomRequest{
		Name:       "IPErrorRoom",
		GameType:   casino.GameTypeIndian,
		Speed:      casino.SpeedFast,
		MaxPlayers: 2,
		Rate:       10,
	})
	if err != nil {
		t.Fatalf("CreateRoom failed: %v", err)
	}
	roomID := detail.Room.ID

	if _, err := svc.JoinRoom(ctx, roomID, p2, "", 0); err != nil {
		t.Fatalf("JoinRoom failed: %v", err)
	}
	if _, err := svc.StartIndianPoker(ctx, roomID, p1); err != nil {
		t.Fatalf("StartIndianPoker failed: %v", err)
	}

	if _, err := svc.PlayIndianPokerAction(ctx, roomID, p1, casino.ActionCall); err != nil {
		t.Fatalf("P1 action failed: %v", err)
	}

	expectedErr := errors.New("injected indian poker update member error")
	roomRepo.failUpdateMember = expectedErr

	_, err = svc.PlayIndianPokerAction(ctx, roomID, p2, casino.ActionShowdown)
	if err == nil {
		t.Fatal("expected error on showdown UpdateMember failure, got nil")
	}
	if !strings.Contains(err.Error(), "injected indian poker update member error") {
		t.Errorf("expected error containing injected message, got: %v", err)
	}
	if txProvider.rollbacks == 0 {
		t.Error("expected transaction rollback on showdown error, got 0 rollbacks")
	}
}

func TestMultiplayerIndianPoker_Settlement_RemoveMemberErrorRollback(t *testing.T) {
	ctx := context.Background()
	rawCasinoRepo := newMockPrizeCasinoRepo()
	casinoRepo := &errorInjectingCasinoRepo{mockPrizeCasinoRepo: rawCasinoRepo}
	memRoomRepo := newMockMemoryRoomRepo()
	roomRepo := &errorInjectingRoomRepo{RoomRepository: memRoomRepo}
	txProvider := &trackingTxProvider{}

	svc, err := casino.NewService(
		casinoRepo,
		casino.WithRoomRepository(roomRepo),
		casino.WithTransactionProvider(txProvider),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	p1 := "ip-elim-p1"
	p2 := "ip-elim-p2"
	casinoRepo.accounts[p1] = casino.Account{CharacterID: p1, Coins: 500}
	casinoRepo.accounts[p2] = casino.Account{CharacterID: p2, Coins: 10} // Will have 0 coins after betting 10

	detail, err := svc.CreateRoom(ctx, p1, casino.CreateRoomRequest{
		Name:       "IPElimRoom",
		GameType:   casino.GameTypeIndian,
		Speed:      casino.SpeedFast,
		MaxPlayers: 2,
		Rate:       10,
	})
	if err != nil {
		t.Fatalf("CreateRoom failed: %v", err)
	}
	roomID := detail.Room.ID

	if _, err := svc.JoinRoom(ctx, roomID, p2, "", 0); err != nil {
		t.Fatalf("JoinRoom failed: %v", err)
	}
	if _, err := svc.StartIndianPoker(ctx, roomID, p1); err != nil {
		t.Fatalf("StartIndianPoker failed: %v", err)
	}

	// P1 has 10 (Card 9), P2 has 2 (Card 1). P1 wins showdown!
	m1, _ := memRoomRepo.GetMember(ctx, roomID, p1)
	m1.Card = 9
	_ = memRoomRepo.UpdateMember(ctx, *m1)

	m2, _ := memRoomRepo.GetMember(ctx, roomID, p2)
	m2.Card = 1
	_ = memRoomRepo.UpdateMember(ctx, *m2)

	if _, err := svc.PlayIndianPokerAction(ctx, roomID, p1, casino.ActionCall); err != nil {
		t.Fatalf("P1 action failed: %v", err)
	}

	expectedErr := errors.New("injected indian poker remove member error")
	roomRepo.failRemoveMember = expectedErr

	_, err = svc.PlayIndianPokerAction(ctx, roomID, p2, casino.ActionShowdown)
	if err == nil {
		t.Fatal("expected error on showdown RemoveMember failure, got nil")
	}
	if !strings.Contains(err.Error(), "injected indian poker remove member error") {
		t.Errorf("expected error containing injected message, got: %v", err)
	}
	if txProvider.rollbacks == 0 {
		t.Error("expected transaction rollback on showdown RemoveMember error, got 0 rollbacks")
	}
}

func TestMultiplayerIndianPoker_Settlement_GetAccountErrorRollback(t *testing.T) {
	ctx := context.Background()
	rawCasinoRepo := newMockPrizeCasinoRepo()
	casinoRepo := &errorInjectingCasinoRepo{mockPrizeCasinoRepo: rawCasinoRepo}
	memRoomRepo := newMockMemoryRoomRepo()
	roomRepo := &errorInjectingRoomRepo{RoomRepository: memRoomRepo}
	txProvider := &trackingTxProvider{}

	svc, err := casino.NewService(
		casinoRepo,
		casino.WithRoomRepository(roomRepo),
		casino.WithTransactionProvider(txProvider),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	p1 := "ip-acc-p1"
	p2 := "ip-acc-p2"
	casinoRepo.accounts[p1] = casino.Account{CharacterID: p1, Coins: 500}
	casinoRepo.accounts[p2] = casino.Account{CharacterID: p2, Coins: 500}

	detail, err := svc.CreateRoom(ctx, p1, casino.CreateRoomRequest{
		Name:       "IPAccErrRoom",
		GameType:   casino.GameTypeIndian,
		Speed:      casino.SpeedFast,
		MaxPlayers: 2,
		Rate:       10,
	})
	if err != nil {
		t.Fatalf("CreateRoom failed: %v", err)
	}
	roomID := detail.Room.ID

	if _, err := svc.JoinRoom(ctx, roomID, p2, "", 0); err != nil {
		t.Fatalf("JoinRoom failed: %v", err)
	}
	if _, err := svc.StartIndianPoker(ctx, roomID, p1); err != nil {
		t.Fatalf("StartIndianPoker failed: %v", err)
	}

	if _, err := svc.PlayIndianPokerAction(ctx, roomID, p1, casino.ActionCall); err != nil {
		t.Fatalf("P1 action failed: %v", err)
	}

	expectedErr := errors.New("injected indian poker get account error")
	casinoRepo.failGetAccount = expectedErr

	_, err = svc.PlayIndianPokerAction(ctx, roomID, p2, casino.ActionShowdown)
	if err == nil {
		t.Fatal("expected error on showdown GetAccount failure, got nil")
	}
	if !strings.Contains(err.Error(), "injected indian poker get account error") {
		t.Errorf("expected error containing injected message, got: %v", err)
	}
	if txProvider.rollbacks == 0 {
		t.Error("expected transaction rollback on showdown GetAccount error, got 0 rollbacks")
	}
}
