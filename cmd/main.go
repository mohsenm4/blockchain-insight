package main

import (
	"fmt"
	"log"

	"github.com/Mohsen20031203/blockchain-insight/config"
	"github.com/Mohsen20031203/blockchain-insight/internal/enthblock"
	"github.com/Mohsen20031203/blockchain-insight/internal/utils"
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

	balance, err := enthblock.GetBalance(client, "0x742d35Cc6634C0532925a3b844Bc454e4438f44e")
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Balance: %s", utils.WeiToEther(balance))

	latestBlock, err := enthblock.GetLatestBlockNumber(client)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Latest Block: %d", latestBlock)

	block, err := enthblock.GetBlockByNumber(client, latestBlock)
	if err != nil {
		log.Fatal(err)
	}
	// print block information
	fmt.Println("Block Information:")
	fmt.Printf("Number: %d\n", block.Number)
	fmt.Printf("Hash: %s\n", block.Hash)
	fmt.Printf("Miner: %s\n", block.Miner)
	fmt.Printf("Timestamp: %d\n", block.Timestamp)
}
