package utils

import (
	"fmt"
	"math/big"
)

func WeiToEther(wei *big.Int) string {
	f := new(big.Float).SetInt(wei)
	etherValue := new(big.Float).Quo(f, big.NewFloat(1e18))
	return fmt.Sprintf("%f", etherValue)
}
