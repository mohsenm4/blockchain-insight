package enthblock

import (
	"context"

	"github.com/ethereum/go-ethereum/ethclient"
)

func GetLatestBlockNumber(client *ethclient.Client) (uint64, error) {
	blockNumber, err := client.BlockNumber(context.Background())
	if err != nil {
		return 0, err
	}
	return blockNumber, nil
}
