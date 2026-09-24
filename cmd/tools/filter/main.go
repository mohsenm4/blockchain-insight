package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/mohsenm4/blockchain-insight/internal/contracts/erc20"
	"github.com/mohsenm4/blockchain-insight/internal/watch"
)

const (
	sepoliaRPC  = "https://ethereum-sepolia-rpc.publicnode.com"
	sepoliaUSDC = "0x1c7D4B196Cb0C7B01d743Fbc6116a902379C7238"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	client, err := ethclient.DialContext(ctx, sepoliaRPC)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	token, err := erc20.NewERC20(common.HexToAddress(sepoliaUSDC), client)
	if err != nil {
		log.Fatal(err)
	}

	out := make(chan watch.Transfer)
	defer close(out) // owner of the channel closes it

	go func() {
		for tr := range out {
			fmt.Printf("block=%d idx=%d %s -> %s value=%s tx=%s\n",
				tr.Block, tr.Index, tr.From.Hex(), tr.To.Hex(), tr.Value, tr.Tx.Hex())
		}
	}()

	if err := watch.Run(ctx, watch.NewERC20Source(client, token), 10*time.Second, out); err != nil {
		log.Println("watcher stopped:", err)
	}
}
