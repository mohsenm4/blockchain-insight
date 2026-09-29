package sender

import (
	"context"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/mohsenm4/blockchain-insight/internal/contracts/erc20"
)

type ERC20Sender struct {
	token *erc20.ERC20
	auth  *bind.TransactOpts
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

	return &ERC20Sender{
		token: token,
		auth:  auth,
	}, nil
}

func (s *ERC20Sender) Transfer(ctx context.Context, to common.Address, amount *big.Int) (common.Hash, error) {
	opts := *s.auth
	opts.Context = ctx

	tx, err := s.token.Transfer(&opts, to, amount)
	if err != nil {
		return common.Hash{}, err
	}
	return tx.Hash(), nil
}
