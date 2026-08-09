package enth

import (
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

func TestGetTxByHash(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{
			"jsonrpc":"2.0",
			"id":1,
			"result":{
				"hash":"0x88df016429689c079f3b2f6ad39fa052532c56795b733da78a91ebe6a713944b",
				"nonce":"0x0",
				"blockHash":"0x1d59ff54b1eb26b013ce3cb5fc9dab3705b415a67127a003c3e61eb445bb8df2",
				"blockNumber":"0x5daf3b",
				"transactionIndex":"0x41",
				"from":"0xa1e4380a3b1f749673e270229993ee55f35663b4",
				"to":"0x5df9b87991262f6ba471f09758cde1c0fc1de734",
				"value":"0x7a69",
				"gas":"0x5208",
				"gasPrice":"0x2540be400",
				"input":"0x",
				"v":"0x25",
				"r":"0x1b5e176d927f8e9ab405058b2d2457392da3e20f328b16ddabcebc33eaac5fea",
				"s":"0x4ba69724e8f69de52f0125ad8b3c5c2cef33019bac3249e2c0a2192766d1721c"
			}
		}`)); err != nil {
			t.Fatalf("write response: %v", err)
		}
	}))

	defer server.Close()

	client, err := NewClient(server.URL)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	txHash := "0x88df016429689c079f3b2f6ad39fa052532c56795b733da78a91ebe6a713944b"
	tx, err := client.GetTxByHash(txHash)
	if err != nil {
		t.Fatalf("GetTxByHash() error = %v", err)
	}
	if tx == nil {
		t.Fatal("expected tx, got nil")
	}

	wantHash := "0xcf847ea253ab83a35bc2fb1677a7c96d1ec14be77c57870d365df2d5c097e0b9"
	if tx.Hash().Hex() != wantHash {
		t.Errorf("Hash = %q, want %q", tx.Hash().Hex(), wantHash)
	}

	wantValue := big.NewInt(31337)
	if tx.Value().Cmp(wantValue) != 0 {
		t.Errorf("Value = %s, want %s", tx.Value().String(), wantValue.String())
	}

}

func TestGetTxByHash_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer server.Close()

	client, err := NewClient(server.URL)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	tx, err := client.GetTxByHash("0x88df016429689c079f3b2f6ad39fa052532c56795b733da78a91ebe6a713944b")

	if err == nil {
		t.Error("expected error, got nil")
	}
	if tx != nil {
		t.Errorf("expected nil tx, got %v", tx)
	}
}

func TestConvertTx(t *testing.T) {

	privateKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}

	expectedFrom := crypto.PubkeyToAddress(privateKey.PublicKey).Hex()

	chainID := big.NewInt(1)
	signer := types.LatestSignerForChainID(chainID)

	toAddr := common.HexToAddress("0x1234567890123456789012345678901234567890")
	unsignedTx := types.NewTx(&types.LegacyTx{
		Nonce:    3,
		To:       &toAddr,
		Value:    big.NewInt(500),
		Gas:      21000,
		GasPrice: big.NewInt(20e9),
	})
	signedTx, err := types.SignTx(unsignedTx, signer, privateKey)
	if err != nil {
		t.Fatalf("SignTx failed: %v", err)
	}
	tests := []struct {
		name       string
		tx         *types.Transaction
		wantTo     string
		wantFrom   string
		wantValue  string
		wantGasFee string
	}{
		{
			name:       "tx with To address",
			tx:         types.NewTransaction(1, common.HexToAddress("0x1234567890123456789012345678901234567890"), big.NewInt(1e18), 21000, big.NewInt(20e9), nil),
			wantTo:     "0x1234567890123456789012345678901234567890",
			wantFrom:   "",
			wantValue:  "1000000000000000000",
			wantGasFee: "20000000000",
		},
		{
			name: "contract creation (To is nil)",
			tx: types.NewContractCreation(
				2,                       // nonce
				big.NewInt(0),           // value
				21000,                   // gas
				big.NewInt(20000000000), // gasPrice
				nil,                     // data
			),
			wantTo:     "",
			wantFrom:   "",
			wantValue:  "0",
			wantGasFee: "20000000000",
		},
		{
			name:       "signed tx (Sender succeeds)",
			tx:         signedTx,
			wantTo:     toAddr.Hex(),
			wantFrom:   expectedFrom,
			wantValue:  "500",
			wantGasFee: "20000000000",
		},
	}

	client := &Client{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := client.ConvertTx(tt.tx)
			if result.Value != tt.wantValue {
				t.Errorf("Value = %q, want %q", result.Value, tt.wantValue)
			}
			if result.To != tt.wantTo {
				t.Errorf("To = %q, want %q", result.To, tt.wantTo)
			}
			if result.From != tt.wantFrom {
				t.Errorf("From = %q, want %q", result.From, tt.wantFrom)
			}
			if result.GasFee != tt.wantGasFee {
				t.Errorf("GasFee = %q, want %q", result.GasFee, tt.wantGasFee)
			}
		})
	}
}
