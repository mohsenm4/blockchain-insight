package enthblock

import "github.com/ethereum/go-ethereum/ethclient"

func NewClient(rpcURL string) (*ethclient.Client, error) {
	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		return nil, err
	}
	return client, nil
}
