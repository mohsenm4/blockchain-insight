package watch

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/mohsenm4/blockchain-insight/internal/contracts/erc20"
)

// fakeNode answers eth_blockNumber with head and eth_getLogs with logs,
// and keeps the params of the last eth_getLogs call.
type fakeNode struct {
	head    uint64
	logs    []map[string]any
	getLogs map[string]any
}

func (n *fakeNode) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID     json.RawMessage   `json:"id"`
		Method string            `json:"method"`
		Params []json.RawMessage `json:"params"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var result any
	switch req.Method {
	case "eth_blockNumber":
		result = hexutil.EncodeUint64(n.head)
	case "eth_getLogs":
		if err := json.Unmarshal(req.Params[0], &n.getLogs); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		result = n.logs
	default:
		http.Error(w, "unexpected method "+req.Method, http.StatusBadRequest)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result})
}

var testToken = common.HexToAddress("0x1c7D4B196Cb0C7B01d743Fbc6116a902379C7238")

func newTestERC20Source(t *testing.T, node *fakeNode) ERC20Source {
	t.Helper()
	srv := httptest.NewServer(node)
	t.Cleanup(srv.Close)

	client, err := ethclient.DialContext(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)

	token, err := erc20.NewERC20(testToken, client)
	if err != nil {
		t.Fatal(err)
	}
	return NewERC20Source(client, token)
}

// The watcher stays `confirmations` blocks behind the head.
func TestERC20SourceBlockNumberLagsHead(t *testing.T) {
	for _, tc := range []struct{ head, want uint64 }{
		{1000, 1000 - confirmations},
		{confirmations, 0},
		{3, 0}, // young chain: never below 0
	} {
		src := newTestERC20Source(t, &fakeNode{head: tc.head})

		got, err := src.BlockNumber(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if got != tc.want {
			t.Errorf("head %d: BlockNumber = %d, want %d", tc.head, got, tc.want)
		}
	}
}

func TestERC20SourceTransfers(t *testing.T) {
	from := common.HexToAddress("0x00000000000000000000000000000000000000aa")
	to := common.HexToAddress("0x000000000000000000000000000000000000dead")
	blockHash := common.HexToHash("0xb10c")
	txHash := common.HexToHash("0x7e57")
	node := &fakeNode{logs: []map[string]any{{
		"address": testToken.Hex(),
		"topics": []string{
			crypto.Keccak256Hash([]byte("Transfer(address,address,uint256)")).Hex(),
			common.BytesToHash(from.Bytes()).Hex(),
			common.BytesToHash(to.Bytes()).Hex(),
		},
		"data":             hexutil.Encode(common.LeftPadBytes(big.NewInt(1_000_000).Bytes(), 32)),
		"blockNumber":      hexutil.EncodeUint64(1005),
		"blockHash":        blockHash.Hex(),
		"transactionHash":  txHash.Hex(),
		"transactionIndex": "0x3",
		"logIndex":         "0x7",
		"removed":          false,
	}}}
	src := newTestERC20Source(t, node)

	trs, err := src.Transfers(context.Background(), 1000, 1010)
	if err != nil {
		t.Fatal(err)
	}

	if got, want := fmt.Sprint(node.getLogs["fromBlock"], " ", node.getLogs["toBlock"]), "0x3e8 0x3f2"; got != want {
		t.Errorf("eth_getLogs range = %s, want %s (1000..1010)", got, want)
	}
	if len(trs) != 1 {
		t.Fatalf("transfers = %d, want 1", len(trs))
	}
	got := trs[0]
	if got.Block != 1005 || got.BlockHash != blockHash || got.Index != 7 || got.Tx != txHash ||
		got.From != from || got.To != to || got.Value.Cmp(big.NewInt(1_000_000)) != 0 {
		t.Errorf("transfer = %+v, want block 1005/%s log 7, %s → %s, 1000000, tx %s",
			got, blockHash.Hex(), from.Hex(), to.Hex(), txHash.Hex())
	}
}
