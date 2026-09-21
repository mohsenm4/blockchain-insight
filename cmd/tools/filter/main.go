package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/Mohsen20031203/blockchain-insight/internal/contracts/erc20"
)

const (
	sepoliaRPC  = "https://ethereum-sepolia-rpc.publicnode.com"
	sepoliaUSDC = "0x1c7D4B196Cb0C7B01d743Fbc6116a902379C7238"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := ethclient.DialContext(ctx, sepoliaRPC) // 1. connect to RPC
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	token, err := erc20.NewERC20(common.HexToAddress(sepoliaUSDC), client) // 2. create contract instance
	if err != nil {
		log.Fatal(err)
	}

	head, err := client.BlockNumber(ctx) // 3. get latest block number
	if err != nil {
		log.Fatal(err)
	}
	from := head - 1000

	iter, err := token.FilterTransfer(&bind.FilterOpts{Start: from, End: &head, Context: ctx}, nil, nil) // 4. filter Transfer events
	if err != nil {
		log.Fatal(err)
	}
	defer iter.Close()

	n := 0
	for iter.Next() { // 5. iterate over events
		ev := iter.Event
		fmt.Printf("block=%d idx=%d %s -> %s value=%s tx=%s\n",
			ev.Raw.BlockNumber, ev.Raw.Index, ev.From.Hex(), ev.To.Hex(), ev.Value, ev.Raw.TxHash.Hex())
		n++
	}
	if err := iter.Error(); err != nil {
		log.Fatal(err)
	}
	fmt.Println("total:", n)
}
