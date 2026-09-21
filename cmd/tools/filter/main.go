package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

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

	last := head

	for {
		head, err := client.BlockNumber(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Println("error getting block number:", err)
			continue
		}

		if head > last {
			iter, err := token.FilterTransfer(
				&bind.FilterOpts{
					Start:   last + 1,
					End:     &head,
					Context: ctx,
				},
				nil,
				nil,
			)
			if err != nil {
				log.Println("error filtering transfers:", err)
			} else {
				for iter.Next() {
					ev := iter.Event
					fmt.Printf(
						"block=%d idx=%d %s -> %s value=%s tx=%s\n",
						ev.Raw.BlockNumber,
						ev.Raw.Index,
						ev.From.Hex(),
						ev.To.Hex(),
						ev.Value,
						ev.Raw.TxHash.Hex(),
					)
				}

				if err := iter.Error(); err != nil {
					log.Println("iterator error:", err)
				}

				iter.Close() // نه defer
			}

			last = head
		}

		select {
		case <-time.After(10 * time.Second):
		case <-ctx.Done():
			fmt.Println("shutting down:", ctx.Err())
			return
		}
	}

}
