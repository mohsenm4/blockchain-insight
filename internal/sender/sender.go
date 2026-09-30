package sender

import (
	"context"
	"errors"
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

func (s *ERC20Sender) Transfer(ctx context.Context, to common.Address, amount *big.Int) (common.Hash, error) {
	opts := *s.auth
	opts.Context = ctx

	// Get the next nonce from the nonce manager
	nonce, err := s.nonceManager.NextNonce(ctx)
	if err != nil {
		return common.Hash{}, err
	}
	opts.Nonce = new(big.Int).SetUint64(nonce)

	tx, err := s.token.Transfer(&opts, to, amount)
	if err != nil {
		s.nonceManager.Reset()
		return common.Hash{}, err
	}
	return tx.Hash(), nil
}
