// challenge-migrate upgrades one quiesced Challenge run without renewing its lifetime.
package main

import (
	"context"
	"flag"
	"log"
	"time"

	"github.com/witchcraze/party2re/internal/challenge"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/valkey"
)

func main() {
	actor := flag.String("character-id", "", "admitted actor authorizing the migration")
	run := flag.String("run-id", "", "immutable SQL admission/run identity")
	flag.Parse()
	if *actor == "" || *run == "" {
		log.Fatal("character-id and run-id are required")
	}
	if err := migrate(*actor, *run); err != nil {
		log.Fatal(err)
	}
}

func migrate(actor, run string) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	db, err := database.OpenFromEnvironment()
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return err
	}
	client, err := valkey.NewClient()
	if err != nil {
		return err
	}
	defer client.Close()
	repo, err := database.NewChallengeRepository(db)
	if err != nil {
		return err
	}
	characters, err := database.NewCharacterRepository(db)
	if err != nil {
		return err
	}
	store, err := challenge.NewValkeySessionRepository(client)
	if err != nil {
		return err
	}
	svc, err := challenge.NewService(repo, characters, nil, challenge.WithActiveSessionStore(store))
	if err != nil {
		return err
	}
	return svc.MigrateSession(ctx, actor, run)
}
