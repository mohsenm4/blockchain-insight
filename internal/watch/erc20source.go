package watch

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/Mohsen20031203/blockchain-insight/internal/contracts/erc20"
)

// ERC20Source adapts a live ERC20 contract to Source.
type ERC20Source struct {
	client *ethclient.Client
	token  *erc20.ERC20
}

func NewERC20Source(client *ethclient.Client, token *erc20.ERC20) ERC20Source {
	return ERC20Source{client: client, token: token}
}

// confirmations is how far behind the chain head we stay: it absorbs
// load-balanced RPC nodes lagging each other, and matches how an exchange
// waits before crediting a deposit.
const confirmations = 5

func (s ERC20Source) BlockNumber(ctx context.Context) (uint64, error) {
	head, err := s.client.BlockNumber(ctx)
	if err != nil {
		return 0, err
	}
	if head < confirmations {
		return 0, nil
	}
	return head - confirmations, nil
}

func (s ERC20Source) Transfers(ctx context.Context, from, to uint64) ([]Transfer, error) {
	iter, err := s.token.FilterTransfer(
		&bind.FilterOpts{Start: from, End: &to, Context: ctx},
		nil,
		nil,
	)
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var trs []Transfer
	for iter.Next() {
		ev := iter.Event
		trs = append(trs, Transfer{
			Block: ev.Raw.BlockNumber,
			Index: ev.Raw.Index,
			From:  ev.From,
			To:    ev.To,
			Value: new(big.Int).Set(ev.Value),
			Tx:    ev.Raw.TxHash,
		})
	}
	if err := iter.Error(); err != nil {
		return nil, err
	}
	return trs, nil
}
