package player_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/fleamarket"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/player"
	"github.com/witchcraze/party2re/internal/store"
	"github.com/witchcraze/party2re/internal/testutil"
)

func waitMarketDeletion(ctx context.Context, ready <-chan struct{}) error {
	select {
	case <-ready:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

type marketDeletionRepo struct {
	*failingCharacterRepo
	sellerID         string
	passes           int
	discovered       chan struct{}
	purchaseLocked   chan struct{}
	purchaseVerified chan struct{}
}

func (r *marketDeletionRepo) DeleteSharedRows(ctx context.Context, charID string) error {
	if charID != r.sellerID {
		return r.failingCharacterRepo.DeleteSharedRows(ctx, charID)
	}
	r.passes++
	if r.passes > 1 {
		if err := waitMarketDeletion(ctx, r.purchaseVerified); err != nil {
			return err
		}
	}
	if err := r.failingCharacterRepo.DeleteSharedRows(ctx, charID); err != nil {
		return err
	}
	if r.passes == 1 {
		close(r.discovered)
		return waitMarketDeletion(ctx, r.purchaseLocked)
	}
	return nil
}

type deletionStoreDelivery struct{ *database.DepotRepository }

func (d deletionStoreDelivery) HoldPayment(ctx context.Context, recipientID, buyerName string, amount int) error {
	return d.AddRemittance(ctx, depot.Remittance{ID: id.New(), CharacterID: recipientID,
		Amount: amount, Source: depot.RemittanceStorePayment, SenderName: buyerName})
}

// Notices are outside this SQL locking regression; payments use the real repository.
func (deletionStoreDelivery) NotifyItem(context.Context, string, string, string) error { return nil }

func TestMarketCreatedAfterDeletionDiscovery(t *testing.T) {
	for _, market := range []string{"flea", "gold", "barter"} {
		for _, mode := range []string{"character", "account", "account rollback"} {
			for iteration := range testutil.GetStressConfig().OpsPerWorker {
				t.Run(fmt.Sprintf("%s/%s/%d", market, mode, iteration), func(t *testing.T) {
					f := newDeletionFixture(t)
					ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
					defer cancel()
					seller, buyer := f.character(t, ""), f.character(t, "")
					sibling := f.character(t, "")
					t.Cleanup(func() {
						cleanupCtx, stop := context.WithTimeout(context.Background(), 60*time.Second)
						defer stop()
						for _, charID := range []string{seller.ID, buyer.ID, sibling.ID} {
							if err := f.charSvc.Delete(cleanupCtx, "", charID); err != nil && !errors.Is(err, character.ErrNotFound) {
								t.Error(err)
							}
						}
						for _, playerID := range []string{seller.PlayerID, buyer.PlayerID, sibling.PlayerID} {
							if err := f.players.PlayerRepository.Delete(cleanupCtx, playerID); err != nil {
								t.Error(err)
							}
						}
					})
					if _, err := f.db.ExecContext(ctx, `UPDATE characters SET player_id = ? WHERE id = ?`, seller.PlayerID, sibling.ID); err != nil {
						t.Fatal(err)
					}
					g := f.createGuild(t, seller)
					seller, err := f.chars.FindByID(ctx, seller.ID)
					if err != nil {
						t.Fatal(err)
					}
					if _, _, err := f.accounts.CreateAPIToken(ctx, seller.PlayerID, "market", nil); err != nil {
						t.Fatal(err)
					}
					if mode == "account rollback" {
						f.players.failDelete = true
					}
					depots, err := database.NewDepotRepository(f.db)
					if err != nil {
						t.Fatal(err)
					}
					item, err := coreitem.NewInstanceWithEnhancement("weapon-01", 1, 3)
					if err != nil {
						t.Fatal(err)
					}
					wish, err := coreitem.NewInstanceWithEnhancement("armor-01", 1, 2)
					if err != nil {
						t.Fatal(err)
					}
					if _, err := database.CreateTestDepot(ctx, f.db, seller.ID, 0, []coreitem.Instance{item}); err != nil {
						t.Fatal(err)
					}
					if _, err := database.CreateTestDepot(ctx, f.db, buyer.ID, 0, []coreitem.Instance{wish}); err != nil {
						t.Fatal(err)
					}
					fleaRepo, err := database.NewFleaMarketRepository(f.db)
					if err != nil {
						t.Fatal(err)
					}
					tx := database.NewTransactionProvider(f.db)
					flea, err := fleamarket.NewService(fleaRepo, f.chars, depots, fleamarket.WithTransactionProvider(tx))
					if err != nil {
						t.Fatal(err)
					}
					storeRepo, err := database.NewStoreRepository(f.db)
					if err != nil {
						t.Fatal(err)
					}
					if err := storeRepo.SaveStore(ctx, store.Store{ID: id.New(), CharacterID: seller.ID,
						TownID: "town1", StoreName: "Del_" + id.New()[:8], ExpiresAt: time.Now().Add(time.Hour),
						CreatedAt: time.Now(), UpdatedAt: time.Now()}); err != nil {
						t.Fatal(err)
					}
					shop := store.NewService(storeRepo, f.chars, depots, nil, tx,
						store.WithDeliveryService(deletionStoreDelivery{depots}))
					r := &marketDeletionRepo{failingCharacterRepo: f.chars, sellerID: seller.ID,
						discovered: make(chan struct{}), purchaseLocked: make(chan struct{}), purchaseVerified: make(chan struct{})}
					locked := make(chan struct{})
					hookPasses := 0
					chars, err := character.NewService(r, character.WithTransactionProvider(tx),
						character.WithCleanupHook(character.CleanupHookFunc(func(ctx context.Context, ids ...string) error {
							if err := f.guilds.RemoveFromGuilds(ctx, ids...); err != nil {
								return err
							}
							hookPasses++
							if hookPasses == 1 {
								close(locked)
							}
							return nil
						})))
					if err != nil {
						t.Fatal(err)
					}
					accounts, err := player.NewService(f.players, player.NewValkeySessionRepository(nil),
						player.WithCharacterService(chars), player.WithTransactionProvider(tx))
					if err != nil {
						t.Fatal(err)
					}
					var sellerMoney, held, guildsBeforeRetry int
					var sellerDepot depot.Depot
					errDelete, errBuy := testutil.RunRace2(func() error {
						if mode == "character" {
							return chars.Delete(ctx, seller.PlayerID, seller.ID)
						}
						return accounts.DeleteAccount(ctx, seller.PlayerID, "")
					}, func() (err error) {
						defer func() {
							if err != nil {
								cancel()
							}
							close(r.purchaseVerified)
						}()
						if err = waitMarketDeletion(ctx, r.discovered); err != nil {
							return err
						}
						var listingID string
						if market == "flea" {
							listing, createErr := flea.CreateListing(ctx, seller.ID, item.ID, 100, time.Now())
							listingID, err = listing.ID, createErr
						} else {
							var sale *store.Sale
							if market == "gold" {
								sale, err = shop.ListGoldItem(ctx, seller.ID, item.ID, 100)
							} else {
								sale, err = shop.ListBarterItem(ctx, seller.ID, item.ID, wish.DefinitionID)
							}
							if err == nil {
								listingID = sale.ID
							}
						}
						if err != nil {
							return err
						}
						err = database.RunInTx(ctx, f.db, func(txCtx context.Context) error {
							if market == "flea" {
								_, err = fleaRepo.GetListingByIDForUpdate(txCtx, listingID)
							} else {
								_, err = storeRepo.GetSaleByIDForUpdate(txCtx, listingID)
							}
							if err != nil {
								return err
							}
							close(r.purchaseLocked)
							if err := waitMarketDeletion(txCtx, locked); err != nil {
								return err
							}
							if market == "flea" {
								_, err := flea.PurchaseListing(txCtx, buyer.ID, listingID, time.Now())
								return err
							}
							if market == "gold" {
								return shop.BuyItem(txCtx, buyer.ID, listingID)
							}
							return shop.TradeItem(txCtx, buyer.ID, listingID, wish.ID)
						})
						if err != nil {
							return err
						}
						// Independent reload before the next deletion attempt can remove seller assets.
						c, err := f.chars.FindByID(ctx, seller.ID)
						if err != nil {
							return err
						}
						sellerMoney = c.Money
						if sellerDepot, err = depots.FindByCharacterID(ctx, seller.ID); err != nil {
							return err
						}
						payments, err := depots.ListRemittances(ctx, seller.ID)
						if err != nil {
							return err
						}
						for _, p := range payments {
							held += p.Amount
						}
						return f.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM guilds WHERE id = ?`, g.ID).Scan(&guildsBeforeRetry)
					})
					if errBuy != nil || testutil.IsDeadlockError(errDelete) {
						t.Fatalf("purchase=%v deletion=%v", errBuy, errDelete)
					}
					if mode == "account rollback" {
						if !errors.Is(errDelete, errInjected) {
							t.Fatalf("deletion=%v, want injected failure", errDelete)
						}
						c, err := f.chars.FindByID(ctx, seller.ID)
						if err != nil || c.Money != sellerMoney {
							t.Fatalf("failed deletion changed committed payment: %+v, %v", c, err)
						}
					} else if errDelete != nil {
						t.Fatal(errDelete)
					}
					if r.passes != 2 || hookPasses != 2 || guildsBeforeRetry != 1 {
						t.Fatalf("whole deletion not rolled back/restarted: discovery=%d hooks=%d guilds=%d", r.passes, hookPasses, guildsBeforeRetry)
					}
					b, err := f.chars.FindByID(ctx, buyer.ID)
					if err != nil {
						t.Fatal(err)
					}
					buyerDepot, err := depots.FindByCharacterID(ctx, buyer.ID)
					if err != nil {
						t.Fatal(err)
					}
					wantMoney, wantItems := buyer.Money-100, 2
					if market == "barter" {
						wantMoney, wantItems = buyer.Money, 1
						if len(sellerDepot.Items) != 1 || sellerDepot.Items[0].DefinitionID != wish.DefinitionID || sellerDepot.Items[0].EnhancementLevel != 2 {
							t.Fatalf("seller lost barter item: %+v", sellerDepot.Items)
						}
					} else if len(sellerDepot.Items) != 0 {
						t.Fatalf("seller kept listed item: %+v", sellerDepot.Items)
					}
					if b.Money != wantMoney || sellerMoney+held+b.Money != seller.Money+buyer.Money || len(buyerDepot.Items) != wantItems {
						t.Fatalf("asset mismatch: buyer=%d seller=%d held=%d depot=%+v", b.Money, sellerMoney, held, buyerDepot.Items)
					}
					found := false
					for _, it := range buyerDepot.Items {
						found = found || it.DefinitionID == item.DefinitionID && it.EnhancementLevel == 3 && it.Quantity == 1
					}
					if !found {
						t.Fatal("buyer did not receive the enhanced listing")
					}
					wantListings := 0
					if mode == "account rollback" && market == "flea" {
						wantListings = 1 // The successful purchase's sold receipt survives rollback.
					}
					if f.count(t, `SELECT COUNT(*) FROM fleamarket_listings WHERE seller_character_id = ?`, seller.ID) != wantListings ||
						f.count(t, `SELECT COUNT(*) FROM store_sales WHERE character_id = ?`, seller.ID) != 0 {
						t.Fatal("seller market rows survived")
					}
					wantSeller, wantSibling, wantPlayer := 0, 0, 0
					if mode == "character" {
						wantSibling, wantPlayer = 1, 1
					} else if mode == "account rollback" {
						wantSeller, wantSibling, wantPlayer = 1, 1, 1
					}
					if f.count(t, `SELECT COUNT(*) FROM characters WHERE id = ?`, seller.ID) != wantSeller ||
						f.count(t, `SELECT COUNT(*) FROM characters WHERE id = ?`, sibling.ID) != wantSibling ||
						f.count(t, `SELECT COUNT(*) FROM players WHERE id = ?`, seller.PlayerID) != wantPlayer ||
						f.count(t, `SELECT COUNT(*) FROM player_api_tokens WHERE player_id = ?`, seller.PlayerID) != wantPlayer ||
						f.count(t, `SELECT COUNT(*) FROM guilds WHERE id = ?`, g.ID) != wantSeller {
						t.Fatal("account, credentials or guild cleanup did not commit/roll back together")
					}
				})
			}
		}
	}
}
