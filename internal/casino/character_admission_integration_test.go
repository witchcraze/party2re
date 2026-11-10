package casino_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	valkeygo "github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/casino"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/testutil"
)

func TestCharacterAdmission_StaleMappingRejoin(t *testing.T) {
	for _, contender := range []string{"join", "spectate", "create", "same-room-retry", "absent-mapping"} {
		t.Run(contender, func(t *testing.T) {
			raw := openIsolatedTestValkey(t, 0)
			t.Cleanup(raw.Close)
			otherRaw := openIsolatedTestValkey(t, 0)
			t.Cleanup(otherRaw.Close)
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			prefix := "admit1993-" + id.New()[:8]
			mover, leaderA, leaderB := prefix+"-m", prefix+"-a", prefix+"-b"
			actors := []string{mover, leaderA, leaderB}
			chars := &inMemoryCharRepo{chars: map[string]corecharacter.Character{}}
			accounts := newMockPrizeCasinoRepo()
			for _, actor := range actors {
				chars.chars[actor] = corecharacter.Character{ID: actor, Name: actor, PlayerID: "owner-" + actor}
				accounts.accounts[actor] = casino.Account{CharacterID: actor, Coins: 500}
			}
			fault := &faultValkeyClient{Client: raw}
			repo, err := casino.NewValkeyRoomRepository(fault, casino.WithValkeyCharacterRepository(chars))
			if err != nil {
				t.Fatal(err)
			}
			svc, err := casino.NewService(accounts, casino.WithDepartureRepository(casino.NewMemoryDepartureRepository()), casino.WithRoomRepository(repo), casino.WithCharacterRepository(chars))
			if err != nil {
				t.Fatal(err)
			}
			request := casino.CreateRoomRequest{Name: prefix, GameType: casino.GameTypeIndian, Speed: casino.SpeedFast, MaxPlayers: 4, Rate: 10, AllowSpectators: true}
			a, err := svc.CreateRoom(ctx, leaderA, request)
			if err != nil {
				t.Fatal(err)
			}
			request.Name += "B"
			b, err := svc.CreateRoom(ctx, leaderB, request)
			if err != nil {
				t.Fatal(err)
			}
			rooms := []string{a.Room.ID, b.Room.ID}
			defer func() { registerCasinoValkeyCleanup(t, raw, rooms, actors) }()
			if _, err := svc.JoinRoom(ctx, a.Room.ID, mover, "", 0); err != nil {
				t.Fatal(err)
			}
			fault.failCharDel.Store(true)
			if err := svc.LeaveRoom(ctx, a.Room.ID, mover); !errors.Is(err, casino.ErrSecondarySync) {
				t.Fatalf("leave=%v", err)
			}
			fault.failCharDel.Store(false)
			if _, err := repo.GetMember(ctx, a.Room.ID, mover); !errors.Is(err, casino.ErrMemberNotFound) {
				t.Fatalf("departed roster=%v", err)
			}
			if mapped, err := repo.GetCharacterRoom(ctx, mover); err != nil || mapped != a.Room.ID {
				t.Fatalf("stale mapping=%q, %v", mapped, err)
			}
			// Finish the durable departure, then seed a stale pointer for the admission race itself.
			if err := svc.LeaveRoom(ctx, a.Room.ID, mover); err != nil {
				t.Fatal(err)
			}
			if err := raw.Do(ctx, raw.B().Set().Key(casino.DefaultCharacterKeyPrefix+mover).Value(a.Room.ID).Ex(casino.DefaultLobbyTTL).Build()).Error(); err != nil {
				t.Fatal(err)
			}
			if contender == "absent-mapping" {
				if err := raw.Do(ctx, raw.B().Del().Key(casino.DefaultCharacterKeyPrefix+mover).Build()).Error(); err != nil {
					t.Fatal(err)
				}
			}

			paused, attempted := make(chan struct{}), make(chan struct{})
			var pausedOnce, attemptedOnce sync.Once
			wait := func(ch <-chan struct{}) error {
				select {
				case <-ch:
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			isClaim := func(cmd []string) bool {
				return len(cmd) == 7 && (cmd[0] == "EVAL" || cmd[0] == "EVALSHA") && cmd[3] == casino.DefaultCharacterKeyPrefix+mover
			}
			blocked := &hookValkeyClient{Client: raw, beforeDo: func(cmd []string) error {
				if isClaim(cmd) {
					pausedOnce.Do(func() { close(paused) })
					return wait(attempted)
				}
				return nil
			}}
			other := &hookValkeyClient{Client: otherRaw,
				beforeDo: func(cmd []string) error {
					if len(cmd) > 2 && cmd[0] == "SET" && strings.Contains(cmd[1], ":lock:") && (strings.HasSuffix(cmd[1], mover) || (contender == "same-room-retry" && cmd[1] == casino.DefaultRoomLockKeyPrefix+b.Room.ID)) {
						attemptedOnce.Do(func() { close(attempted) })
					}
					return nil
				},
				afterDo: func(cmd []string, err error) {
					// Without character serialization, let the old-room claim complete first.
					if isClaim(cmd) && err == nil {
						attemptedOnce.Do(func() { close(attempted) })
					}
				},
			}
			service := func(client *hookValkeyClient) *casino.Service {
				r, err := casino.NewValkeyRoomRepository(client, casino.WithValkeyCharacterRepository(chars))
				if err != nil {
					t.Fatal(err)
				}
				s, err := casino.NewService(accounts, casino.WithDepartureRepository(casino.NewMemoryDepartureRepository()), casino.WithRoomRepository(r), casino.WithCharacterRepository(chars))
				if err != nil {
					t.Fatal(err)
				}
				return s
			}
			blockedSvc, otherSvc := service(blocked), service(other)
			errs := testutil.RunRace(
				func() error {
					_, err := blockedSvc.JoinRoom(ctx, b.Room.ID, mover, "", 0)
					return err
				},
				func() error {
					if err := wait(paused); err != nil {
						return err
					}
					switch contender {
					case "spectate":
						_, err = otherSvc.SpectateRoom(ctx, a.Room.ID, mover, "")
					case "create":
						request.Name += "C"
						var created *casino.RoomDetail
						created, err = otherSvc.CreateRoom(ctx, mover, request)
						if created != nil {
							rooms = append(rooms, created.Room.ID)
						}
					case "same-room-retry":
						_, err = otherSvc.JoinRoom(ctx, b.Room.ID, mover, "", 0)
					default:
						_, err = otherSvc.JoinRoom(ctx, a.Room.ID, mover, "", 0)
					}
					return err
				},
			)
			wins := 0
			for _, err := range errs {
				if err == nil {
					wins++
				} else if !errors.Is(err, casino.ErrAlreadyInRoom) {
					t.Fatalf("admission=%v", errs)
				}
			}
			if wins != 1 {
				t.Fatalf("expected one admission, got %v", errs)
			}
			count := 0
			for _, roomID := range rooms {
				if _, err := repo.GetMember(ctx, roomID, mover); err == nil {
					count++
				} else if !errors.Is(err, casino.ErrMemberNotFound) {
					t.Fatal(err)
				}
			}
			mapped, err := repo.GetCharacterRoom(ctx, mover)
			if err != nil || mapped != b.Room.ID || count != 1 {
				t.Fatalf("rosters=%d mapping=%q err=%v", count, mapped, err)
			}
			view, err := svc.GetCharacterRoomView(ctx, "owner-"+mover, mover)
			if err != nil || view == nil || view.Room.ID != mapped {
				t.Fatalf("view=%+v err=%v", view, err)
			}
			if err := raw.Do(ctx, raw.B().Get().Key(casino.DefaultCharacterLockKeyPrefix+mover).Build()).Error(); !valkeygo.IsValkeyNil(err) {
				t.Fatalf("admission lock retained: %v", err)
			}
		})
	}
}

func TestCharacterAdmission_ConcurrentCreateAndCancellation(t *testing.T) {
	raw := openIsolatedTestValkey(t, 0)
	t.Cleanup(raw.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	prefix := "create1993-" + id.New()[:8]
	mover := prefix + "-m"
	accounts := newMockPrizeCasinoRepo()
	accounts.accounts[mover] = casino.Account{CharacterID: mover, Coins: 500}
	repo, err := casino.NewValkeyRoomRepository(raw)
	if err != nil {
		t.Fatal(err)
	}
	svc, err := casino.NewService(accounts, casino.WithRoomRepository(repo))
	if err != nil {
		t.Fatal(err)
	}
	var results [2]*casino.RoomDetail
	errs := testutil.RunRace(
		func() error {
			var err error
			results[0], err = svc.CreateRoom(ctx, mover, casino.CreateRoomRequest{Name: prefix + "A", GameType: casino.GameTypeIndian, Speed: casino.SpeedFast, MaxPlayers: 4, Rate: 10})
			return err
		},
		func() error {
			var err error
			results[1], err = svc.CreateRoom(ctx, mover, casino.CreateRoomRequest{Name: prefix + "B", GameType: casino.GameTypeIndian, Speed: casino.SpeedFast, MaxPlayers: 4, Rate: 10})
			return err
		},
	)
	var rooms []string
	for _, result := range results {
		if result != nil {
			rooms = append(rooms, result.Room.ID)
		}
	}
	registerCasinoValkeyCleanup(t, raw, rooms, []string{mover})
	if len(rooms) != 1 {
		t.Fatalf("create outcomes=%v, results=%+v", errs, results)
	}
	for _, err := range errs {
		if err != nil && !errors.Is(err, casino.ErrAlreadyInRoom) {
			t.Fatal(err)
		}
	}
	if mapped, err := repo.GetCharacterRoom(ctx, mover); err != nil || mapped != rooms[0] {
		t.Fatalf("mapping=%q, %v", mapped, err)
	}

	waiter := prefix + "-waiter"
	lockKey := casino.DefaultCharacterLockKeyPrefix + waiter
	if err := raw.Do(ctx, raw.B().Set().Key(lockKey).Value("foreign-token").Ex(casino.DefaultRoomLockTTL).Build()).Error(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := raw.Do(context.Background(), raw.B().Del().Key(lockKey).Build()).Error(); err != nil {
			t.Error(err)
		}
	})
	cancelCtx, cancelWait := context.WithCancel(ctx)
	client := &hookValkeyClient{Client: raw, beforeDo: func(cmd []string) error {
		if len(cmd) > 2 && cmd[0] == "SET" && cmd[1] == lockKey {
			cancelWait()
			return context.Canceled
		}
		return nil
	}}
	waitRepo, err := casino.NewValkeyRoomRepository(client)
	if err != nil {
		t.Fatal(err)
	}
	member := casino.RoomMember{RoomID: rooms[0], CharacterID: waiter}
	if err := waitRepo.AddMember(cancelCtx, member); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled admission=%v", err)
	}
	if _, err := repo.GetMember(ctx, rooms[0], waiter); !errors.Is(err, casino.ErrMemberNotFound) {
		t.Fatalf("canceled roster=%v", err)
	}
	if token, err := raw.Do(ctx, raw.B().Get().Key(lockKey).Build()).ToString(); err != nil || token != "foreign-token" {
		t.Fatalf("foreign lock=%q, %v", token, err)
	}
	if err := raw.Do(ctx, raw.B().Del().Key(lockKey).Build()).Error(); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddMember(ctx, member); err != nil {
		t.Fatalf("retry after cancellation=%v", err)
	}
	registerCasinoValkeyCleanup(t, raw, nil, []string{waiter})
}
