package enth

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
)

type Client struct {
	Eth *ethclient.Client
}

func NewClient(rpcURL string, timeout time.Duration) (*Client, error) {
	eth, err := Dial(context.Background(), rpcURL, timeout)
	if err != nil {
		return nil, err
	}
	return &Client{Eth: eth}, nil
}

// Dial connects to an HTTP JSON-RPC node. Every call gives up after
// timeout, whatever its context: a node that takes the request and never
// answers would otherwise hold the caller forever, without an error to log,
// retry or fall back on.
func Dial(ctx context.Context, rpcURL string, timeout time.Duration) (*ethclient.Client, error) {
	u, err := url.Parse(rpcURL)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("invalid RPC URL %q: scheme must be http or https", rpcURL)
	}
	if timeout <= 0 {
		return nil, fmt.Errorf("RPC timeout must be more than 0, got %v", timeout)
	}

	httpClient := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 32,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	rpcClient, err := rpc.DialOptions(ctx, rpcURL, rpc.WithHTTPClient(httpClient))
	if err != nil {
		return nil, err
	}

	return ethclient.NewClient(rpcClient), nil
}
