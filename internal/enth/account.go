package enth

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

func (c *Client) GetBalance(address string) (*big.Int, error) {
	account := common.HexToAddress(address)
	balance, err := c.Eth.BalanceAt(context.Background(), account, nil)
	if err != nil {
		return nil, err
	}
	return balance, nil
}
