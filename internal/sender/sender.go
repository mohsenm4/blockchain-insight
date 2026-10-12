package sender

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/mohsenm4/blockchain-insight/internal/contracts/erc20"
	"github.com/mohsenm4/blockchain-insight/internal/nonce"
)

type ERC20Sender struct {
	client       *ethclient.Client
	token        *erc20.ERC20
	auth         *bind.TransactOpts
	nonceManager *nonce.NonceManager
}

// setup: runs once at startup
func NewERC20Sender(ctx context.Context, client *ethclient.Client, tokenAddr, privateKey string) (*ERC20Sender, error) {
	chainID, err := client.ChainID(ctx)
	if err != nil {
		return nil, err
	}

	prv := strings.TrimPrefix(privateKey, "0x")

	key, err := crypto.HexToECDSA(prv)
	if err != nil {
		return nil, err
	}

	auth, err := bind.NewKeyedTransactorWithChainID(key, chainID)
	if err != nil {
		return nil, err
	}

	token, err := erc20.NewERC20(common.HexToAddress(tokenAddr), client)
	if err != nil {
		return nil, err
	}

	// Initialize the nonce manager for this sender
	nonceManager := nonce.NewNonceManager(auth.From, client)

	return &ERC20Sender{
		token:        token,
		auth:         auth,
		client:       client,
		nonceManager: nonceManager,
	}, nil
}

func (s *ERC20Sender) Status(ctx context.Context, hash common.Hash) (string, error) {
	receipt, err := s.client.TransactionReceipt(ctx, hash)
	if errors.Is(err, ethereum.NotFound) {
		return "pending", nil
	}
	if err != nil {
		return "", err
	}
	if receipt.Status == types.ReceiptStatusSuccessful {
		return "mined", nil
	}
	return "failed", nil
}

// Sign builds and signs a transfer of amount to to, without sending it.
// An error here means nothing left: no tx exists yet.
func (s *ERC20Sender) Sign(ctx context.Context, to common.Address, amount *big.Int) (*types.Transaction, error) {
	opts := *s.auth
	opts.Context = ctx
	opts.NoSend = true

	nonce, err := s.nonceManager.NextNonce(ctx)
	if err != nil {
		return nil, fmt.Errorf("next nonce: %w", err)
	}
	opts.Nonce = new(big.Int).SetUint64(nonce)

	tx, err := s.token.Transfer(&opts, to, amount)
	if err != nil {
		s.nonceManager.Reset()
		return nil, fmt.Errorf("sign transfer: %w", err)
	}
	return tx, nil
}

// Broadcast sends a signed tx. An error does not mean the tx did not leave:
// the node may have taken it before the connection broke. Ask Check.
func (s *ERC20Sender) Broadcast(ctx context.Context, tx *types.Transaction) error {
	if err := s.client.SendTransaction(ctx, tx); err != nil {
		return fmt.Errorf("broadcast %s: %w", tx.Hash().Hex(), err)
	}
	return nil
}

// ResetNonce makes the next Sign read the nonce from the chain again. Call
// it when a tx that took a nonce may never reach the network.
func (s *ERC20Sender) ResetNonce() {
	s.nonceManager.Reset()
}

// TxState is what the chain says about a signed tx.
type TxState int

const (
	// TxNotSeen: no node we asked has the tx and its nonce is still free.
	// It may still land, so resending the same tx is safe.
	TxNotSeen TxState = iota
	// TxSent: the tx is in the mempool or in a block. The money left.
	TxSent
	// TxDropped: another tx used its nonce. It can never land.
	TxDropped
)

// Check tells whether tx reached the chain.
func (s *ERC20Sender) Check(ctx context.Context, tx *types.Transaction) (TxState, error) {
	// Nonce first: if tx gets mined between the two calls, the lookup
	// below still finds it, so it is never taken for dropped.
	used, err := s.client.NonceAt(ctx, s.auth.From, nil)
	if err != nil {
		return TxNotSeen, fmt.Errorf("nonce at: %w", err)
	}

	_, _, err = s.client.TransactionByHash(ctx, tx.Hash())
	if err == nil {
		return TxSent, nil
	}
	if !errors.Is(err, ethereum.NotFound) {
		return TxNotSeen, fmt.Errorf("tx by hash %s: %w", tx.Hash().Hex(), err)
	}
	if used > tx.Nonce() {
		return TxDropped, nil
	}
	return TxNotSeen, nil
}
