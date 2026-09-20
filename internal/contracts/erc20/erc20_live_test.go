package erc20

import (
	"context"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

const (
	sepoliaRPC  = "https://ethereum-sepolia-rpc.publicnode.com"
	sepoliaUSDC = "0x1c7D4B196Cb0C7B01d743Fbc6116a902379C7238"
)

func TestLiveBalanceOf(t *testing.T) {
	if testing.Short() {
		t.Skip("needs network")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	client, err := ethclient.DialContext(ctx, sepoliaRPC)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	token, err := NewERC20(common.HexToAddress(sepoliaUSDC), client)
	if err != nil {
		t.Fatal(err)
	}

	opts := &bind.CallOpts{Context: ctx}

	symbol, err := token.Symbol(opts)
	if err != nil {
		t.Fatal(err)
	}
	bal, err := token.BalanceOf(opts, common.HexToAddress("0x0000000000000000000000000000000000000000"))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("symbol=%s balance(0x0)=%s", symbol, bal)
}
