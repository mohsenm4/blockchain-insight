package main

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/mohsenm4/blockchain-insight/config"
	"github.com/mohsenm4/blockchain-insight/internal/contracts/erc20"
)

func main() {
	cfg, err := config.LoadConfig("../../")
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	client, err := ethclient.DialContext(ctx, cfg.RPCURL)
	if err != nil {
		log.Fatalf("dial RPC: %v", err)
	}
	defer client.Close()

	chainID, err := client.ChainID(ctx)
	if err != nil {
		log.Fatalf("get chain ID: %v", err)
	}

	prv := strings.TrimPrefix(cfg.PrivateKey, "0x")

	key, err := crypto.HexToECDSA(prv)
	if err != nil {
		log.Fatalf("parse private key: %v", err)
	}

	auth, err := bind.NewKeyedTransactorWithChainID(key, chainID)
	if err != nil {
		log.Fatalf("create transactor: %v", err)
	}

	auth.Context = ctx

	token, err := erc20.NewERC20(common.HexToAddress(cfg.TokenAddress), client)
	if err != nil {
		log.Fatalf("create token instance: %v", err)
	}

	to := common.HexToAddress("0xb958BbF92100a04ab37CB00Ae1E6c1aA0f3B6CA9")
	amount := big.NewInt(1_000_000)

	tx, err := token.Transfer(auth, to, amount)
	if err != nil {
		log.Fatalf("transfer tokens: %v", err)
	}

	fmt.Println("Transaction nonce:", tx.Nonce())
	fmt.Println("Transaction hash:", tx.Hash().Hex())
	fmt.Println("Transaction gas:", tx.Gas())
	fmt.Println("Transaction gas fee cap:", tx.GasFeeCap())
	fmt.Println("Transaction gas tip cap:", tx.GasTipCap())
	fmt.Println("successful")
}
