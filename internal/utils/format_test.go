package utils

import (
	"math/big"
	"testing"
)

func TestWeiToEther_OneEther(t *testing.T) {
	wei := big.NewInt(1_000_000_000_000_000_000) // 1e18
	got := WeiToEther(wei)
	want := "1.000000"

	if got != want {
		t.Errorf("WeiToEther() = %q, want %q", got, want)
	}
}

func TestWeiToEnther_Zero(t *testing.T) {
	wei := big.NewInt(0)
	got := WeiToEther(wei)
	want := "0.000000"

	if got != want {
		t.Errorf("WeiToEther() = %q, want %q", got, want)
	}
}

func TestWeiToEther_OneWei(t *testing.T) {
	wei := big.NewInt(1) // 1 wei
	got := WeiToEther(wei)
	want := "0.000000"

	if got != want {
		t.Errorf("WeiToEther() = %q, want %q", got, want)
	}
}
