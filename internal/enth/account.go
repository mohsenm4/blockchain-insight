package enth

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

func (c *Client) GetBalance(address string) (*big.Int, error) {
	account := common.HexToAddress(address)

	background := context.Background()
	balance, err := c.Eth.BalanceAt(background, account, nil)
	if err != nil {
		return nil, err
	}
	return balance, nil
}
