package main

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/Mohsen20031203/blockchain-insight/internal/contracts/erc20"
	"github.com/Mohsen20031203/blockchain-insight/internal/watch"
)

const (
	sepoliaRPC  = "https://ethereum-sepolia-rpc.publicnode.com"
	sepoliaUSDC = "0x1c7D4B196Cb0C7B01d743Fbc6116a902379C7238"
)

// erc20Source adapts a live ERC20 contract to watch.Source.
type erc20Source struct {
	client *ethclient.Client
	token  *erc20.ERC20
}

func (s erc20Source) BlockNumber(ctx context.Context) (uint64, error) {
	return s.client.BlockNumber(ctx)
}

func (s erc20Source) Transfers(ctx context.Context, from, to uint64) ([]watch.Transfer, error) {
	iter, err := s.token.FilterTransfer(
		&bind.FilterOpts{Start: from, End: &to, Context: ctx},
		nil,
		nil,
	)
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var trs []watch.Transfer
	for iter.Next() {
		ev := iter.Event
		trs = append(trs, watch.Transfer{
			Block: ev.Raw.BlockNumber,
			Index: ev.Raw.Index,
			From:  ev.From,
			To:    ev.To,
			Value: new(big.Int).Set(ev.Value),
			Tx:    ev.Raw.TxHash,
		})
	}
	if err := iter.Error(); err != nil {
		return nil, err
	}
	return trs, nil
}

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

	if err := watch.Run(ctx, erc20Source{client: client, token: token}, 10*time.Second, out); err != nil {
		log.Println("watcher stopped:", err)
	}
}
