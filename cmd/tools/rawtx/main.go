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

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
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

	from := crypto.PubkeyToAddress(key.PublicKey)

	nonce, err := client.PendingNonceAt(ctx, from)
	if err != nil {
		log.Fatalf("get nonce: %v", err)
	}

	tip, err := client.SuggestGasTipCap(ctx)
	if err != nil {
		log.Fatalf("get gas tip cap: %v", err)
	}

	header, err := client.HeaderByNumber(ctx, nil)
	if err != nil {
		log.Fatalf("get latest block header: %v", err)
	}

	fee := new(big.Int).Mul(header.BaseFee, big.NewInt(2))
	feeCap := new(big.Int).Add(fee, tip)

	abi, err := erc20.ERC20MetaData.GetAbi()
	if err != nil {
		log.Fatal(err)
	}

	to := common.HexToAddress("0xb958BbF92100a04ab37CB00Ae1E6c1aA0f3B6CA9")
	amount := big.NewInt(1_000_000)

	data, err := abi.Pack("transfer", to, amount)
	if err != nil {
		log.Fatalf("pack data: %v", err)
	}

	token := common.HexToAddress(cfg.TokenAddress)
	gas, err := client.EstimateGas(ctx, ethereum.CallMsg{From: from, To: &token, Data: data})
	if err != nil {
		log.Fatalf("estimate gas: %v", err)
	}

	tx := types.NewTx(&types.DynamicFeeTx{
		ChainID:   chainID,
		Nonce:     nonce,
		GasTipCap: tip,
		GasFeeCap: feeCap,
		Gas:       gas,
		To:        &token,
		Value:     big.NewInt(0),
		Data:      data,
	})

	signedTx, err := types.SignTx(tx, types.NewLondonSigner(chainID), key)
	if err != nil {
		log.Fatalf("sign tx: %v", err)
	}

	err = client.SendTransaction(ctx, signedTx)
	if err != nil {
		log.Fatalf("send transaction: %v", err)
	}

	fmt.Println("signed tx hash:", signedTx.Hash().Hex())
	fmt.Println("estimated gas:", gas)
	fmt.Printf("signed tx: %x\n", signedTx.Hash())
	fmt.Println("address  :", from.Hex())
	fmt.Println("chain ID :", chainID)
	fmt.Println("nonce    :", nonce)
	fmt.Println("tip cap  :", tip)
	fmt.Println("block    :", header.Number)
	fmt.Println("fee cap  :", feeCap)
	fmt.Printf("data: %x\n", data)

}
