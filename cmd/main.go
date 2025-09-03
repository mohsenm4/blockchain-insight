package main

import (
	"log"

	"github.com/Mohsen20031203/blockchain-insight/config"
	"github.com/Mohsen20031203/blockchain-insight/internal/api"
)

// @title Blockchain Insight API
// @version 1.0
// @BasePath
func main() {

	config, err := config.LoadConfig("../.")
	if err != nil {
		log.Fatal(err)
	}

	server := api.NewServer(config)

	if err := server.Start(":5050"); err != nil {
		log.Fatal(err)
	}
}
