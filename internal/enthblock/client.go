package enthblock

import "github.com/ethereum/go-ethereum/ethclient"

type Client struct {
	Eth *ethclient.Client
}

func NewClient(rpcURL string) (*Client, error) {
	eth, err := ethclient.Dial(rpcURL)
	if err != nil {
		return nil, err
	}
	client := &Client{
		Eth: eth,
	}
	return client, nil
}
