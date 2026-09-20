package contracts

import (
	"bytes"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

const erc20TransferABI = `[{"name":"transfer","type":"function",
  "inputs":[{"name":"to","type":"address"},{"name":"amount","type":"uint256"}],
  "outputs":[{"name":"","type":"bool"}]}]`

func TestManualTransferEncoding(t *testing.T) {
	to := common.HexToAddress("0x1111111111111111111111111111111111111111")
	amount := big.NewInt(1_000_000)

	var manual []byte

	selector := crypto.Keccak256([]byte("transfer(address,uint256)"))[:4]
	manual = append(manual, selector...)
	manual = append(manual, common.LeftPadBytes(to.Bytes(), 32)...)
	manual = append(manual, common.LeftPadBytes(amount.Bytes(), 32)...)

	parsed, err := abi.JSON(strings.NewReader(erc20TransferABI))
	if err != nil {
		t.Fatal(err)
	}
	expected, err := parsed.Pack("transfer", to, amount)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(manual, expected) {
		t.Fatalf("mismatch\nmanual:   %x\nexpected: %x", manual, expected)
	}
}
