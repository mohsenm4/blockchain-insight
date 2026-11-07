package utils

import (
	"fmt"
	"math/big"
)

// WeiToEther converts an amount in wei (1 ether = 1e18 wei) to an ether
// string with exactly 6 decimal places, e.g. "1.500000". It is for display
// only: anything smaller than 0.000001 ether is rounded, so 1 wei becomes
// "0.000000".
func WeiToEther(wei *big.Int) string {
	f := new(big.Float).SetInt(wei)
	etherValue := new(big.Float).Quo(f, big.NewFloat(1e18))
	return fmt.Sprintf("%f", etherValue)
}
