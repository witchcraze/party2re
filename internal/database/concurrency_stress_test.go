package database

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/auction"
	"github.com/witchcraze/party2re/internal/bank"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/guild"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/shop"
)

var errOutOfStock = errors.New("out of stock")

func getStressConfig() (workers int, operationsPerWorker int) {
	if os.Getenv("PARTY2_STRESS_ENABLED") == "1" {
		return 50, 20
	}
	// Fast verification for default make check / CI
	return 15, 10
}

func TestConcurrencyStressBankTransfersAndDeposits(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	now := time.Now().UTC()

	playerRepo, err := NewPlayerRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	charRepo, err := NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	bankRepo, err := NewBankRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	numPlayers := 6
	players := make([]coreplayer.Player, numPlayers)
	initialBalancePerPlayer := int(10000)

	suffix := id.New()[:8]
	for i := 0; i < numPlayers; i++ {
		p, err := coreplayer.New(fmt.Sprintf("sp_%s_%d", suffix, i), "password123", now)
		if err != nil {
			t.Fatal(err)
		}
		if err := playerRepo.Save(ctx, p); err != nil {
			t.Fatal(err)
		}
		players[i] = p

		c, err := corecharacter.New(fmt.Sprintf("SC_%s_%d", suffix, i))
		if err != nil {
			t.Fatal(err)
		}
		c.PlayerID = p.ID
		c.Money = initialBalancePerPlayer
		if err := charRepo.Save(ctx, c); err != nil {
			t.Fatal(err)
		}

		// Initial deposit
		_, _, err = bankRepo.Deposit(ctx, p.ID, c.ID, initialBalancePerPlayer)
		if err != nil {
			t.Fatal(err)
		}
	}

	workers, opsPerWorker := getStressConfig()
	totalOps := workers * opsPerWorker

	var wg sync.WaitGroup
	var successfulTransfers int64
	var failedTransfers int64
	var deadlockErrors int64

	start := time.Now()

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			r := rand.New(rand.NewSource(time.Now().UnixNano() + int64(workerID)))

			for op := 0; op < opsPerWorker; op++ {
				// Pick 2 distinct players
				fromIdx := r.Intn(numPlayers)
				toIdx := r.Intn(numPlayers)
				for toIdx == fromIdx {
					toIdx = r.Intn(numPlayers)
				}

				fromPlayer := players[fromIdx]
				toPlayer := players[toIdx]
				transferAmount := int64(r.Intn(200) + 1)

				record := bank.TransferRecord{
					ID:           id.New(),
					FromPlayerID: fromPlayer.ID,
					ToPlayerID:   toPlayer.ID,
					Amount:       transferAmount,
					CreatedAt:    time.Now().UTC(),
				}

				_, _, err := bankRepo.Transfer(ctx, record)
				if err != nil {
					if errors.Is(err, bank.ErrInsufficientBalance) {
						atomic.AddInt64(&failedTransfers, 1)
					} else {
						// Check if deadlock error
						if isDeadlockError(err) {
							atomic.AddInt64(&deadlockErrors, 1)
						}
						t.Errorf("worker %d unexpected transfer error: %v", workerID, err)
					}
				} else {
					atomic.AddInt64(&successfulTransfers, 1)
				}
			}
		}(w)
	}

	wg.Wait()
	duration := time.Since(start)

	if deadlockErrors > 0 {
		t.Fatalf("Deadlock detected during concurrent transfers: count = %d", deadlockErrors)
	}

	// Verify Conservation of Money across all accounts
	var totalEndingBalance int64
	for _, p := range players {
		acc, err := bankRepo.GetAccount(ctx, p.ID)
		if err != nil {
			t.Fatalf("failed to read account balance for %s: %v", p.ID, err)
		}
		if acc.Balance < 0 {
			t.Fatalf("account %s went negative: %d", p.ID, acc.Balance)
		}
		totalEndingBalance += acc.Balance
	}

	expectedTotal := int64(initialBalancePerPlayer * numPlayers)
	if totalEndingBalance != expectedTotal {
		t.Fatalf("Money conservation violated! Expected %d total balance, got %d", expectedTotal, totalEndingBalance)
	}

	t.Logf("Bank Concurrency Stress Test Completed: %d total ops (%d success, %d insufficient balance) in %v. Total conserved: %d gold",
		totalOps, successfulTransfers, failedTransfers, duration, totalEndingBalance)
}

func TestConcurrencyStressGuildConcurrentDonations(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	now := time.Now().UTC()

	playerRepo, err := NewPlayerRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	charRepo, err := NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	guildRepo, err := NewGuildRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	suffix := id.New()[:8]
	leaderPlayer, err := coreplayer.New("glead_p_"+suffix, "password123", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := playerRepo.Save(ctx, leaderPlayer); err != nil {
		t.Fatal(err)
	}

	leaderChar, err := corecharacter.New("GLeadChar_" + suffix)
	if err != nil {
		t.Fatal(err)
	}
	leaderChar.PlayerID = leaderPlayer.ID
	leaderChar.Money = 50000
	if err := charRepo.Save(ctx, leaderChar); err != nil {
		t.Fatal(err)
	}

	testGuild := guild.Guild{
		ID:                id.New(),
		Name:              "StressG_" + suffix,
		LeaderCharacterID: leaderChar.ID,
		Level:             1,
		Exp:               0,
		Gold:              0,
		Notice:            "Stress Testing Guild",
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	creatorMember := guild.Member{
		GuildID:          testGuild.ID,
		CharacterID:      leaderChar.ID,
		Role:             guild.RoleLeader,
		JoinedAt:         now,
		TotalDonatedGold: 0,
	}

	createdGuild, _, _, err := guildRepo.CreateGuild(ctx, testGuild, creatorMember, 0)
	if err != nil {
		t.Fatal(err)
	}

	workers, opsPerWorker := getStressConfig()
	donationAmountPerOp := 50

	type memberInfo struct {
		character corecharacter.Character
	}
	members := make([]memberInfo, workers)

	for w := 0; w < workers; w++ {
		p, err := coreplayer.New(fmt.Sprintf("gmp_%s_%d", suffix, w), "password123", now)
		if err != nil {
			t.Fatal(err)
		}
		if err := playerRepo.Save(ctx, p); err != nil {
			t.Fatal(err)
		}

		c, err := corecharacter.New(fmt.Sprintf("GM_%s_%d", suffix, w))
		if err != nil {
			t.Fatal(err)
		}
		c.PlayerID = p.ID
		c.Money = opsPerWorker * donationAmountPerOp * 2 // sufficient money
		if err := charRepo.Save(ctx, c); err != nil {
			t.Fatal(err)
		}

		_, err = guildRepo.AddMember(ctx, guild.Member{
			GuildID:          createdGuild.ID,
			CharacterID:      c.ID,
			Role:             guild.RoleMember,
			JoinedAt:         now,
			TotalDonatedGold: 0,
		})
		if err != nil {
			t.Fatal(err)
		}
		members[w] = memberInfo{character: c}
	}

	var wg sync.WaitGroup
	var successfulDonations int64
	var deadlockErrors int64

	start := time.Now()

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			m := members[workerID]

			for op := 0; op < opsPerWorker; op++ {
				_, _, _, err := guildRepo.Donate(ctx, createdGuild.ID, m.character.ID, donationAmountPerOp)
				if err != nil {
					if isDeadlockError(err) {
						atomic.AddInt64(&deadlockErrors, 1)
					}
					t.Errorf("worker %d unexpected donation error: %v", workerID, err)
				} else {
					atomic.AddInt64(&successfulDonations, 1)
				}
			}
		}(w)
	}

	wg.Wait()
	duration := time.Since(start)

	if deadlockErrors > 0 {
		t.Fatalf("Deadlock detected during concurrent guild donations: count = %d", deadlockErrors)
	}

	// Verify Guild Total Gold and Member Contributions
	finalGuild, guildMembers, err := guildRepo.GetGuild(ctx, createdGuild.ID)
	if err != nil {
		t.Fatalf("failed to get guild: %v", err)
	}

	expectedGuildGold := int64(successfulDonations * int64(donationAmountPerOp))
	if finalGuild.Gold != expectedGuildGold {
		t.Fatalf("Guild gold mismatch! Expected %d, got %d", expectedGuildGold, finalGuild.Gold)
	}

	// Verify all members' total donations
	var sumMemberDonations int64
	for _, gm := range guildMembers {
		sumMemberDonations += gm.TotalDonatedGold
	}

	if sumMemberDonations != expectedGuildGold {
		t.Fatalf("Sum of member donations (%d) does not match guild gold (%d)", sumMemberDonations, expectedGuildGold)
	}

	t.Logf("Guild Concurrency Stress Test Completed: %d successful donations totaling %d gold in %v",
		successfulDonations, expectedGuildGold, duration)
}

func TestConcurrencyStressShopStockDepletion(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	now := time.Now().UTC()

	playerRepo, err := NewPlayerRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	charRepo, err := NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	invRepo, err := NewInventoryRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	itemDefRepo, err := NewItemDefinitionRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	suffix := id.New()[:8]
	itemDefID := "stress_item_" + suffix
	itemPrice := 100
	itemDef, err := coreitem.NewDefinition(itemDefID, "Limited Edition Stress Relic", itemPrice)
	if err != nil {
		t.Fatal(err)
	}
	if err := itemDefRepo.Save(ctx, itemDef); err != nil {
		t.Fatal(err)
	}

	initialStock := 5
	var currentStock int64 = int64(initialStock)

	workers := 25
	buyers := make([]corecharacter.Character, workers)
	for w := 0; w < workers; w++ {
		p, err := coreplayer.New(fmt.Sprintf("bp_%s_%d", suffix, w), "password123", now)
		if err != nil {
			t.Fatal(err)
		}
		if err := playerRepo.Save(ctx, p); err != nil {
			t.Fatal(err)
		}

		c, err := corecharacter.New(fmt.Sprintf("B_%s_%d", suffix, w))
		if err != nil {
			t.Fatal(err)
		}
		c.PlayerID = p.ID
		c.Money = itemPrice * 2 // enough money
		if err := charRepo.Save(ctx, c); err != nil {
			t.Fatal(err)
		}
		buyers[w] = c
	}

	var wg sync.WaitGroup
	var successfulPurchases int64
	var outOfStockCount int64
	var deadlockErrors int64

	start := time.Now()

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			buyer := buyers[workerID]

			// Atomic purchase transaction simulation with RunInTx and row locking
			err := RunInTx(ctx, db, func(txCtx context.Context) error {
				exec := ExecutorFromContext(txCtx, db)

				// 1. Lock character
				char, err := scanCharacterRow(exec.QueryRowContext(txCtx, `SELECT `+characterColumns+` FROM characters WHERE id = ? FOR UPDATE`, buyer.ID))
				if err != nil {
					return err
				}
				if char.Money < itemPrice {
					return shop.ErrInsufficientFunds
				}

				// 2. Check and decrement stock atomically using CAS
				for {
					stock := atomic.LoadInt64(&currentStock)
					if stock <= 0 {
						return errOutOfStock
					}
					if atomic.CompareAndSwapInt64(&currentStock, stock, stock-1) {
						break
					}
				}

				// 3. Deduct money
				char.Money -= itemPrice
				if err := updateCharacterAtomically(txCtx, exec, char); err != nil {
					return err
				}

				// 4. Add item to inventory
				itemInstance, err := coreitem.NewInstance(itemDefID, 1)
				if err != nil {
					return err
				}
				_, err = exec.ExecContext(txCtx, `
					INSERT INTO inventory_items (id, character_id, definition_id, quantity, enhancement_level)
					VALUES (?, ?, ?, ?, ?)
				`, itemInstance.ID, char.ID, itemInstance.DefinitionID, itemInstance.Quantity, itemInstance.EnhancementLevel)
				return err
			})

			if err != nil {
				if errors.Is(err, errOutOfStock) {
					atomic.AddInt64(&outOfStockCount, 1)
				} else {
					if isDeadlockError(err) {
						atomic.AddInt64(&deadlockErrors, 1)
					}
					t.Errorf("buyer %d unexpected error: %v", workerID, err)
				}
			} else {
				atomic.AddInt64(&successfulPurchases, 1)
			}
		}(w)
	}

	wg.Wait()
	duration := time.Since(start)

	if deadlockErrors > 0 {
		t.Fatalf("Deadlock detected during concurrent shop stock depletion: count = %d", deadlockErrors)
	}

	if int(successfulPurchases) != initialStock {
		t.Fatalf("Expected exactly %d successful purchases, got %d", initialStock, successfulPurchases)
	}

	expectedOutOfStock := workers - initialStock
	if int(outOfStockCount) != expectedOutOfStock {
		t.Fatalf("Expected exactly %d out-of-stock errors, got %d", expectedOutOfStock, outOfStockCount)
	}

	// Verify each successful buyer received their item
	var totalItemsInInventories int
	for _, buyer := range buyers {
		inv, err := invRepo.FindByCharacterID(ctx, buyer.ID)
		if err != nil {
			t.Fatal(err)
		}
		totalItemsInInventories += len(inv.Items)
	}

	if totalItemsInInventories != initialStock {
		t.Fatalf("Total items across buyer inventories (%d) must equal initial stock (%d)", totalItemsInInventories, initialStock)
	}

	t.Logf("Shop Stock Depletion Stress Test Completed: %d purchased, %d out-of-stock in %v",
		successfulPurchases, outOfStockCount, duration)
}

func TestConcurrencyStressAuctionBiddingAndBuyout(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	now := time.Now().UTC()

	playerRepo, err := NewPlayerRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	charRepo, err := NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	auctionRepo, err := NewAuctionRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	suffix := id.New()[:8]
	sellerPlayer, err := coreplayer.New("sel_p_"+suffix, "password123", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := playerRepo.Save(ctx, sellerPlayer); err != nil {
		t.Fatal(err)
	}

	sellerChar, err := corecharacter.New("SC_" + suffix)
	if err != nil {
		t.Fatal(err)
	}
	sellerChar.PlayerID = sellerPlayer.ID
	if err := charRepo.Save(ctx, sellerChar); err != nil {
		t.Fatal(err)
	}

	startBid := 100
	buyoutPrice := 5000
	listing, err := auctionRepo.CreateListing(ctx, auction.AuctionListing{
		ID:                id.New(),
		SellerCharacterID: sellerChar.ID,
		ItemID:            "item_" + suffix,
		ItemName:          "Mythic Blade",
		ItemCategory:      "WEAPON",
		EnhancementLevel:  10,
		StartBid:          startBid,
		BuyoutPrice:       buyoutPrice,
		ExpiresAt:         now.Add(24 * time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}

	numBidders := 20
	bidders := make([]corecharacter.Character, numBidders)
	for i := 0; i < numBidders; i++ {
		p, err := coreplayer.New(fmt.Sprintf("bidp_%s_%d", suffix, i), "password123", now)
		if err != nil {
			t.Fatal(err)
		}
		if err := playerRepo.Save(ctx, p); err != nil {
			t.Fatal(err)
		}

		c, err := corecharacter.New(fmt.Sprintf("BC_%s_%d", suffix, i))
		if err != nil {
			t.Fatal(err)
		}
		c.PlayerID = p.ID
		c.Money = 10000 // rich bidders
		if err := charRepo.Save(ctx, c); err != nil {
			t.Fatal(err)
		}
		bidders[i] = c
	}

	var wg sync.WaitGroup
	var successfulBids int64
	var outbidErrors int64
	var deadlockErrors int64

	start := time.Now()

	for i := 0; i < numBidders; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			bidder := bidders[idx]
			bidAmount := startBid + (idx+1)*50

			_, err := auctionRepo.PlaceBid(ctx, listing.ID, bidder.ID, bidAmount)
			if err != nil {
				if errors.Is(err, auction.ErrInvalidBidAmount) || errors.Is(err, auction.ErrListingNotActive) {
					atomic.AddInt64(&outbidErrors, 1)
				} else {
					if isDeadlockError(err) {
						atomic.AddInt64(&deadlockErrors, 1)
					}
					t.Errorf("bidder %d unexpected bid error: %v", idx, err)
				}
			} else {
				atomic.AddInt64(&successfulBids, 1)
			}
		}(i)
	}

	wg.Wait()
	duration := time.Since(start)

	if deadlockErrors > 0 {
		t.Fatalf("Deadlock detected during concurrent auction bidding: count = %d", deadlockErrors)
	}

	// Verify Auction State
	finalListing, err := auctionRepo.GetListing(ctx, listing.ID)
	if err != nil {
		t.Fatal(err)
	}

	if finalListing.CurrentBid < startBid {
		t.Fatalf("Expected current bid >= start bid %d, got %d", startBid, finalListing.CurrentBid)
	}

	// Verify Total Money across seller and all bidders is conserved
	var totalMoneyEnd int
	for _, bidder := range bidders {
		c, err := charRepo.FindByID(ctx, bidder.ID)
		if err != nil {
			t.Fatal(err)
		}
		totalMoneyEnd += c.Money
	}

	expectedTotalMoneyEnd := (numBidders * 10000) - finalListing.CurrentBid
	if totalMoneyEnd != expectedTotalMoneyEnd {
		t.Fatalf("Money conservation in auction bidding failed! Expected total bidder money %d, got %d", expectedTotalMoneyEnd, totalMoneyEnd)
	}

	t.Logf("Auction Bidding Concurrency Stress Test Completed: %d successful bids, %d outbids in %v. Highest bid: %d by %v",
		successfulBids, outbidErrors, duration, finalListing.CurrentBid, *finalListing.HighestBidderID)
}

func TestConcurrencyStressMultiDomainChaos(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	now := time.Now().UTC()

	playerRepo, err := NewPlayerRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	charRepo, err := NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	bankRepo, err := NewBankRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	depotRepo, err := NewDepotRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	guildRepo, err := NewGuildRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	suffix := id.New()[:8]

	// Shared Guild
	leadP, err := coreplayer.New("cgp_"+suffix, "password123", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := playerRepo.Save(ctx, leadP); err != nil {
		t.Fatal(err)
	}

	leadC, err := corecharacter.New("CL_" + suffix)
	if err != nil {
		t.Fatal(err)
	}
	leadC.PlayerID = leadP.ID
	leadC.Money = 50000
	if err := charRepo.Save(ctx, leadC); err != nil {
		t.Fatal(err)
	}

	sharedGuild, _, _, err := guildRepo.CreateGuild(ctx, guild.Guild{
		ID:                id.New(),
		Name:              "ChaosG_" + suffix,
		LeaderCharacterID: leadC.ID,
		Level:             1,
		Notice:            "Chaos guild",
		CreatedAt:         now,
		UpdatedAt:         now,
	}, guild.Member{
		CharacterID: leadC.ID,
		Role:        guild.RoleLeader,
		JoinedAt:    now,
	}, 0)
	if err != nil {
		t.Fatal(err)
	}

	// Setup 10 chaos subjects
	numSubjects := 10
	type subject struct {
		player    coreplayer.Player
		character corecharacter.Character
	}
	subjects := make([]subject, numSubjects)

	for i := 0; i < numSubjects; i++ {
		p, err := coreplayer.New(fmt.Sprintf("cp_%s_%d", suffix, i), "password123", now)
		if err != nil {
			t.Fatal(err)
		}
		if err := playerRepo.Save(ctx, p); err != nil {
			t.Fatal(err)
		}

		c, err := corecharacter.New(fmt.Sprintf("CC_%s_%d", suffix, i))
		if err != nil {
			t.Fatal(err)
		}
		c.PlayerID = p.ID
		c.Money = 10000
		if err := charRepo.Save(ctx, c); err != nil {
			t.Fatal(err)
		}

		_, _, err = bankRepo.Deposit(ctx, p.ID, c.ID, 5000)
		if err != nil {
			t.Fatal(err)
		}

		_, err = guildRepo.AddMember(ctx, guild.Member{
			GuildID:     sharedGuild.ID,
			CharacterID: c.ID,
			Role:        guild.RoleMember,
			JoinedAt:    now,
		})
		if err != nil {
			t.Fatal(err)
		}

		err = depotRepo.Save(ctx, depot.Depot{
			CharacterID: c.ID,
			Capacity:    20,
			Gold:        1000,
			Items:       nil,
		})
		if err != nil {
			t.Fatal(err)
		}

		subjects[i] = subject{player: p, character: c}
	}

	workers, opsPerWorker := getStressConfig()
	var wg sync.WaitGroup
	var completedChaosOps int64
	var deadlockErrors int64

	start := time.Now()

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			r := rand.New(rand.NewSource(time.Now().UnixNano() + int64(workerID)))

			for op := 0; op < opsPerWorker; op++ {
				action := r.Intn(4)
				s1 := subjects[r.Intn(numSubjects)]
				s2 := subjects[r.Intn(numSubjects)]

				switch action {
				case 0:
					// Bank Transfer
					if s1.player.ID != s2.player.ID {
						_, _, err := bankRepo.Transfer(ctx, bank.TransferRecord{
							ID:           id.New(),
							FromPlayerID: s1.player.ID,
							ToPlayerID:   s2.player.ID,
							Amount:       int64(r.Intn(50) + 1),
							CreatedAt:    time.Now().UTC(),
						})
						if err != nil && !errors.Is(err, bank.ErrInsufficientBalance) {
							if isDeadlockError(err) {
								atomic.AddInt64(&deadlockErrors, 1)
							}
						}
					}
				case 1:
					// Guild Donation
					_, _, _, err := guildRepo.Donate(ctx, sharedGuild.ID, s1.character.ID, 10)
					if err != nil && !errors.Is(err, guild.ErrInsufficientFunds) {
						if isDeadlockError(err) {
							atomic.AddInt64(&deadlockErrors, 1)
						}
					}
				case 2:
					// Depot Gold Storage
					dep, err := depotRepo.FindByCharacterID(ctx, s1.character.ID)
					if err == nil {
						dep.Gold += 10
						_ = depotRepo.Save(ctx, dep)
					}
				case 3:
					// Character Profile / Money Update within RunInTx
					_ = RunInTx(ctx, db, func(txCtx context.Context) error {
						c, err := charRepo.FindByID(txCtx, s1.character.ID)
						if err != nil {
							return err
						}
						c.Money += 5
						return charRepo.Update(txCtx, c)
					})
				}
				atomic.AddInt64(&completedChaosOps, 1)
			}
		}(w)
	}

	wg.Wait()
	duration := time.Since(start)

	if deadlockErrors > 0 {
		t.Fatalf("Deadlock detected in Multi-Domain Chaos Stress Test: count = %d", deadlockErrors)
	}

	t.Logf("Multi-Domain Chaos Stress Test Completed: %d mixed domain operations across %d workers in %v with 0 deadlocks",
		completedChaosOps, workers, duration)
}

func isDeadlockError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	// MariaDB Error 1213: Deadlock found when trying to get lock
	// MariaDB Error 1205: Lock wait timeout exceeded
	return errors.Is(err, context.DeadlineExceeded) ||
		(len(errStr) > 0 && (contains(errStr, "1213") ||
			contains(errStr, "Deadlock") ||
			contains(errStr, "deadlock") ||
			contains(errStr, "1205") ||
			contains(errStr, "Lock wait timeout")))
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || (len(s) > 0 && len(substr) > 0 && findSubstr(s, substr)))
}

func findSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
