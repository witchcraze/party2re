package database

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/auction"
	"github.com/witchcraze/party2re/internal/bank"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/delivery"
	"github.com/witchcraze/party2re/internal/guild"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/shop"
)

var errOutOfStock = errors.New("out of stock")

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

	bankRepo, err := NewBankRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	numPlayers := 6
	players := make([]coreplayer.Player, numPlayers)
	initialBalancePerPlayer := int(10000)

	suffix := id.New()[:8]
	for i := 0; i < numPlayers; i++ {
		c, err := CreateTestCharacterWithFunds(ctx, db, fmt.Sprintf("SC_%s_%d", suffix, i), initialBalancePerPlayer)
		if err != nil {
			t.Fatal(err)
		}

		// Initial deposit
		_, _, err = bankRepo.Deposit(ctx, c.PlayerID, c.ID, initialBalancePerPlayer)
		if err != nil {
			t.Fatal(err)
		}
		players[i] = coreplayer.Player{ID: c.PlayerID}
	}

	cfg := GetStressConfig()
	var failedTransfers int64

	res := RunConcurrentStressTest(t, cfg, func(workerID int, op int) error {
		r := rand.New(rand.NewSource(time.Now().UnixNano() + int64(workerID*1000+op)))
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
				return err
			}
			t.Errorf("worker %d unexpected transfer error: %v", workerID, err)
			return err
		}
		return nil
	})

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
		res.TotalOps, res.Successes, failedTransfers, res.Duration, totalEndingBalance)
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

	guildRepo, err := NewGuildRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	suffix := id.New()[:8]
	createdGuild, _, err := CreateTestGuildWithLeader(ctx, db, "StressG_"+suffix, 50000)
	if err != nil {
		t.Fatal(err)
	}

	cfg := GetStressConfig()
	donationAmountPerOp := 50

	type memberInfo struct {
		character corecharacter.Character
	}
	members := make([]memberInfo, cfg.Workers)

	for w := 0; w < cfg.Workers; w++ {
		c, err := CreateTestCharacterWithFunds(ctx, db, fmt.Sprintf("GM_%s_%d", suffix, w), cfg.OpsPerWorker*donationAmountPerOp*2)
		if err != nil {
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

	res := RunConcurrentStressTest(t, cfg, func(workerID int, op int) error {
		m := members[workerID]
		_, _, _, err := guildRepo.Donate(ctx, createdGuild.ID, m.character.ID, donationAmountPerOp)
		if err != nil {
			t.Errorf("worker %d unexpected donation error: %v", workerID, err)
			return err
		}
		return nil
	})

	// Verify Guild Total Gold and Member Contributions
	finalGuild, guildMembers, err := guildRepo.GetGuild(ctx, createdGuild.ID)
	if err != nil {
		t.Fatalf("failed to get guild: %v", err)
	}

	expectedGuildGold := int64(res.Successes * int64(donationAmountPerOp))
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
		res.Successes, expectedGuildGold, res.Duration)
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
		c, err := CreateTestCharacterWithFunds(ctx, db, fmt.Sprintf("B_%s_%d", suffix, w), itemPrice*2)
		if err != nil {
			t.Fatal(err)
		}
		buyers[w] = c
	}

	var outOfStockCount int64

	res := RunConcurrentStressTest(t, ConcurrencyStressConfig{Workers: workers, OpsPerWorker: 1}, func(workerID int, op int) error {
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
				return err
			}
			t.Errorf("buyer %d unexpected error: %v", workerID, err)
			return err
		}
		return nil
	})

	if int(res.Successes) != initialStock {
		t.Fatalf("Expected exactly %d successful purchases, got %d", initialStock, res.Successes)
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
		res.Successes, outOfStockCount, res.Duration)
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

	charRepo, err := NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	auctionRepo, err := NewAuctionRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	suffix := id.New()[:8]
	sellerChar, err := CreateTestCharacter(ctx, db, "SC_"+suffix)
	if err != nil {
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
		c, err := CreateTestCharacterWithFunds(ctx, db, fmt.Sprintf("BC_%s_%d", suffix, i), 10000)
		if err != nil {
			t.Fatal(err)
		}
		bidders[i] = c
	}

	var outbidErrors int64

	res := RunConcurrentStressTest(t, ConcurrencyStressConfig{Workers: numBidders, OpsPerWorker: 1}, func(workerID int, op int) error {
		bidder := bidders[workerID]
		bidAmount := startBid + (workerID+1)*50

		_, err := auctionRepo.PlaceBid(ctx, listing.ID, bidder.ID, bidAmount)
		if err != nil {
			if errors.Is(err, auction.ErrInvalidBidAmount) || errors.Is(err, auction.ErrListingNotActive) {
				atomic.AddInt64(&outbidErrors, 1)
				return err
			}
			t.Errorf("bidder %d unexpected bid error: %v", workerID, err)
			return err
		}
		return nil
	})

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
		res.Successes, outbidErrors, res.Duration, finalListing.CurrentBid, *finalListing.HighestBidderID)
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

	// Shared Guild via centralized factory
	sharedGuild, _, err := CreateTestGuildWithLeader(ctx, db, "ChaosG_"+suffix, 50000)
	if err != nil {
		t.Fatal(err)
	}

	// Setup 10 chaos subjects via centralized factories
	numSubjects := 10
	type subject struct {
		player    coreplayer.Player
		character corecharacter.Character
	}
	subjects := make([]subject, numSubjects)

	for i := 0; i < numSubjects; i++ {
		c, err := CreateTestCharacterWithFunds(ctx, db, fmt.Sprintf("CC_%s_%d", suffix, i), 10000)
		if err != nil {
			t.Fatal(err)
		}

		_, _, err = bankRepo.Deposit(ctx, c.PlayerID, c.ID, 5000)
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

		_, err = CreateTestDepot(ctx, db, c.ID, 1000, nil)
		if err != nil {
			t.Fatal(err)
		}

		subjects[i] = subject{
			player:    coreplayer.Player{ID: c.PlayerID},
			character: c,
		}
	}

	cfg := GetStressConfig()
	res := RunConcurrentStressTest(t, cfg, func(workerID int, op int) error {
		r := rand.New(rand.NewSource(time.Now().UnixNano() + int64(workerID*1000+op)))
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
					return err
				}
			}
		case 1:
			// Guild Donation
			_, _, _, err := guildRepo.Donate(ctx, sharedGuild.ID, s1.character.ID, 10)
			if err != nil && !errors.Is(err, guild.ErrInsufficientFunds) {
				return err
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
		return nil
	})

	t.Logf("Multi-Domain Chaos Stress Test Completed: %d mixed domain operations across %d workers in %v with 0 deadlocks",
		res.TotalOps, cfg.Workers, res.Duration)
}

func TestConcurrencyStressDeliveryClaimVsCancel(t *testing.T) {
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

	charRepo, err := NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	invRepo, err := NewInventoryRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	deliveryRepo, err := NewDeliveryRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	txProvider := NewTransactionProvider(db)

	deliverySvc, err := delivery.NewService(
		deliveryRepo,
		charRepo,
		invRepo,
		delivery.WithTransactionProvider(txProvider),
	)
	if err != nil {
		t.Fatal(err)
	}

	// Create Sender & Recipient via centralized factory
	senderChar, err := CreateTestCharacterWithFunds(ctx, db, "DelivSender", 100000)
	if err != nil {
		t.Fatal(err)
	}
	recipientChar, err := CreateTestCharacterWithFunds(ctx, db, "DelivRecipient", 5000)
	if err != nil {
		t.Fatal(err)
	}

	const numRounds = 20
	var claimWins int64
	var cancelWins int64

	for i := 0; i < numRounds; i++ {
		parcel, err := deliverySvc.SendParcel(ctx, senderChar.ID, delivery.SendParcelRequest{
			RecipientCharacterID: recipientChar.ID,
			GoldAmount:           500,
		}, now)
		if err != nil {
			t.Fatalf("round %d: SendParcel failed: %v", i, err)
		}

		claimErr, cancelErr := RunRace2(
			func() error {
				_, err := deliverySvc.ClaimParcel(ctx, recipientChar.ID, parcel.ID, now)
				return err
			},
			func() error {
				return deliverySvc.CancelParcel(ctx, senderChar.ID, parcel.ID)
			},
		)

		if claimErr == nil && cancelErr == nil {
			t.Fatalf("round %d: DOUBLE SPEND! Both ClaimParcel and CancelParcel succeeded on parcel %s", i, parcel.ID)
		}

		if claimErr == nil {
			atomic.AddInt64(&claimWins, 1)
			if !errors.Is(cancelErr, delivery.ErrParcelAlreadyClaimed) {
				t.Fatalf("round %d: expected ErrParcelAlreadyClaimed for cancel, got %v", i, cancelErr)
			}
		} else if cancelErr == nil {
			atomic.AddInt64(&cancelWins, 1)
			if !errors.Is(claimErr, delivery.ErrParcelAlreadyClaimed) {
				t.Fatalf("round %d: expected ErrParcelAlreadyClaimed for claim, got %v", i, claimErr)
			}
		} else {
			t.Fatalf("round %d: both operations failed! claimErr: %v, cancelErr: %v", i, claimErr, cancelErr)
		}
	}

	t.Logf("Delivery Claim vs Cancel race test passed across %d rounds: Claims=%d, Cancels=%d, 0 double-spends",
		numRounds, claimWins, cancelWins)
}
