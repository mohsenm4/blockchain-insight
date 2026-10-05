package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/mohsenm4/blockchain-insight/internal/ledger"
	"github.com/mohsenm4/blockchain-insight/internal/watch"
)

// fakeLedger is an in-memory Ledger.
type fakeLedger struct {
	owners   map[common.Address]int64
	deposits []ledger.Deposit
	cursor   map[string]uint64
	err      error // returned by every call when set
}

func newFakeLedger() *fakeLedger {
	return &fakeLedger{owners: map[common.Address]int64{}, cursor: map[string]uint64{}}
}

func (f *fakeLedger) AssignAddress(ctx context.Context, userID int64, addr common.Address) error {
	if f.err != nil {
		return f.err
	}
	if _, ok := f.owners[addr]; ok {
		return ledger.ErrAddressTaken
	}
	f.owners[addr] = userID
	return nil
}

func (f *fakeLedger) UserByAddress(ctx context.Context, addr common.Address) (int64, bool, error) {
	if f.err != nil {
		return 0, false, f.err
	}
	id, ok := f.owners[addr]
	return id, ok, nil
}

func (f *fakeLedger) DepositsByAddress(ctx context.Context, addr common.Address) ([]ledger.Deposit, error) {
	if f.err != nil {
		return nil, f.err
	}
	var out []ledger.Deposit
	for _, d := range f.deposits {
		if d.Address == ledger.AddressKey(addr) {
			out = append(out, d)
		}
	}
	return out, nil
}

func (f *fakeLedger) Cursor(ctx context.Context, name string) (uint64, bool, error) {
	if f.err != nil {
		return 0, false, f.err
	}
	b, ok := f.cursor[name]
	return b, ok, nil
}

func (f *fakeLedger) RecordBatch(ctx context.Context, name string, block uint64, deps []ledger.Deposit) (int, error) {
	if f.err != nil {
		return 0, f.err
	}
	f.deposits = append(f.deposits, deps...)
	f.cursor[name] = block
	return len(deps), nil
}

func newTestServer() (*Server, *fakeLedger) {
	lg := newFakeLedger()
	s := &Server{
		ledger: lg,
	}

	s.setupRouter()
	return s, lg
}

func postWatch(s *Server, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/watch", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)
	return rec
}

func TestPostWatchEmptyBody(t *testing.T) {
	s, _ := newTestServer()

	rec := postWatch(s, `{}`)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status code %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestPostWatchMissingUserID(t *testing.T) {
	s, _ := newTestServer()

	rec := postWatch(s, `{"address":"0x0000000000000000000000000000000000000001"}`)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status code %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestPostWatchInvalidAddress(t *testing.T) {
	s, _ := newTestServer()

	rec := postWatch(s, `{"address":"salam","user_id":1}`)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status code %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestPostWatchValidAddress(t *testing.T) {
	s, lg := newTestServer()

	address := "0x0000000000000000000000000000000000000001"

	rec := postWatch(s, `{"address":"`+address+`","user_id":42}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status code %d, got %d", http.StatusOK, rec.Code)
	}

	expected := `{"watching":"` + address + `"}`
	if strings.TrimSpace(rec.Body.String()) != expected {
		t.Errorf("expected body %s, got %s", expected, rec.Body.String())
	}

	if got := lg.owners[common.HexToAddress(address)]; got != 42 {
		t.Errorf("expected address assigned to user 42, got %d", got)
	}
}

func TestPostWatchAddressTaken(t *testing.T) {
	s, _ := newTestServer()

	body := `{"address":"0x0000000000000000000000000000000000000001","user_id":1}`
	postWatch(s, body)
	rec := postWatch(s, body)

	if rec.Code != http.StatusConflict {
		t.Errorf("expected status code %d, got %d", http.StatusConflict, rec.Code)
	}
}

func TestPostWatchLedgerError(t *testing.T) {
	s, lg := newTestServer()
	lg.err = errors.New("db down")

	rec := postWatch(s, `{"address":"0x0000000000000000000000000000000000000001","user_id":1}`)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status code %d, got %d", http.StatusInternalServerError, rec.Code)
	}
}

func TestGetWatchTransfersInvalidAddress(t *testing.T) {
	s, _ := newTestServer()

	req := httptest.NewRequest(
		http.MethodGet,
		"/watch/salam/transfers",
		nil,
	)

	rec := httptest.NewRecorder()

	s.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status code %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestGetWatchTransfers(t *testing.T) {
	s, lg := newTestServer()

	address := "0x0000000000000000000000000000000000000001"

	lg.deposits = []ledger.Deposit{{
		UserID:      1,
		Address:     ledger.AddressKey(common.HexToAddress(address)),
		TxHash:      "0x1234",
		LogIndex:    0,
		Amount:      big.NewInt(100),
		BlockNumber: 123,
		Status:      "pending",
	}}

	req := httptest.NewRequest(
		http.MethodGet,
		"/watch/"+address+"/transfers",
		nil,
	)

	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected GET status code %d, got %d", http.StatusOK, rec.Code)
	}

	body := rec.Body.String()

	if !strings.Contains(body, `"block":123`) {
		t.Errorf("expected response to contain block 123, got %s", body)
	}

	if !strings.Contains(body, `"value":"100"`) {
		t.Errorf("expected response to contain value 100, got %s", body)
	}

	if !strings.Contains(body, `"status":"pending"`) {
		t.Errorf("expected response to contain status pending, got %s", body)
	}
}

func TestGetWatchTransfersEmpty(t *testing.T) {
	s, _ := newTestServer()

	address := "0x0000000000000000000000000000000000000001"

	req := httptest.NewRequest(
		http.MethodGet,
		"/watch/"+address+"/transfers",
		nil,
	)

	rec := httptest.NewRecorder()

	s.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status code %d, got %d", http.StatusOK, rec.Code)
	}

	expected := `{"transfers":[]}`
	if strings.TrimSpace(rec.Body.String()) != expected {
		t.Errorf("expected body %s, got %s", expected, rec.Body.String())
	}
}

func TestRecordBatchKeepsOnlyOurAddresses(t *testing.T) {
	s, lg := newTestServer()

	ours := common.HexToAddress("0x00000000000000000000000000000000000000A1")
	other := common.HexToAddress("0x00000000000000000000000000000000000000B2")
	lg.owners[ours] = 7

	b := watch.Batch{From: 100, To: 105, Transfers: []watch.Transfer{
		{Block: 101, BlockHash: common.HexToHash("0xb101"), Index: 2, To: ours,
			Value: big.NewInt(500), Tx: common.HexToHash("0xaa")},
		{Block: 102, Index: 0, To: other, Value: big.NewInt(9), Tx: common.HexToHash("0xbb")},
	}}

	if err := s.recordBatch(context.Background(), b); err != nil {
		t.Fatalf("recordBatch: %v", err)
	}

	if len(lg.deposits) != 1 {
		t.Fatalf("expected 1 deposit, got %d", len(lg.deposits))
	}
	d := lg.deposits[0]
	if d.UserID != 7 || d.Address != ledger.AddressKey(ours) || d.LogIndex != 2 ||
		d.BlockNumber != 101 || d.Amount.Cmp(big.NewInt(500)) != 0 ||
		d.TxHash != common.HexToHash("0xaa").Hex() || d.BlockHash != common.HexToHash("0xb101").Hex() {
		t.Errorf("unexpected deposit: %+v", d)
	}

	// The cursor moves to the end of the batch, even past blocks with no deposits.
	if got := lg.cursor[watcherName]; got != 105 {
		t.Errorf("expected cursor 105, got %d", got)
	}
}

func TestRecordBatchLedgerError(t *testing.T) {
	s, lg := newTestServer()
	lg.err = errors.New("db down")

	b := watch.Batch{From: 1, To: 1, Transfers: []watch.Transfer{
		{Block: 1, To: common.HexToAddress("0x01"), Value: big.NewInt(1)},
	}}

	// The error must reach watch.Run so it retries the same blocks.
	if err := s.recordBatch(context.Background(), b); err == nil {
		t.Fatal("expected an error, got nil")
	}
}
