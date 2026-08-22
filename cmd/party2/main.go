package main

import (
	"fmt"
	"log"

	"github.com/witchcraze/party2re/internal/database"
)

func main() {
	db, err := database.OpenFromEnvironment()
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if err := database.Ping(db); err != nil {
		log.Fatal(err)
	}

	fmt.Println("party2 database connection ok")
}
