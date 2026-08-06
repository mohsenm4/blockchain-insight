package enth

import (
	"net/http"
	"time"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
)

type Client struct {
	Eth *ethclient.Client
}

func NewClient(rpcURL string) (*Client, error) {
	httpClient := &http.Client{
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 32,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	rpcClient, err := rpc.DialHTTPWithClient(rpcURL, httpClient)
	if err != nil {
		return nil, err
	}

	return &Client{Eth: ethclient.NewClient(rpcClient)}, nil
}
