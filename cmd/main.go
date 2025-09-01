package main

import (
	"log"

	"github.com/Mohsen20031203/blockchain-insight/config"
	"github.com/Mohsen20031203/blockchain-insight/internal/enthblock"
)

func main() {

	config, err := config.LoadConfig("../.")
	if err != nil {
		log.Fatal(err)
	}
	client, err := enthblock.NewClient(config.RPCURL)
	if err != nil {
		log.Fatal(err)
	}
	_ = client
}
