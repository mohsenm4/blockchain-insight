package main

import (
	"log"

	"github.com/Mohsen20031203/blockchain-insight/config"
	"github.com/Mohsen20031203/blockchain-insight/internal/api"
)

func main() {

	config, err := config.LoadConfig("../.")
	if err != nil {
		log.Fatal(err)
	}

	server := api.NewServer(config)

	if err := server.Start(":8080"); err != nil {
		log.Fatal(err)
	}
}
