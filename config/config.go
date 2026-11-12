package config

import (
	"errors"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	RPCURL        string `mapstructure:"rpc_url"`
	ArchiveRPCURL string `mapstructure:"archive_rpc_url"`
	NetworkName   string `mapstructure:"network_name"`
	TimeoutSec    int    `mapstructure:"timeout_sec"`
	TokenAddress  string `mapstructure:"token_address"`
	PrivateKey    string `mapstructure:"private_key"`
	DatabaseURL   string `mapstructure:"database_url"`
	RedisURL      string `mapstructure:"redis_url"`
	SendLimit     int64  `mapstructure:"send_limit"`
	Confirmations uint64 `mapstructure:"confirmations"`
	Finality      uint64 `mapstructure:"finality"`
}

var ErrMissingRPCURL = errors.New("RPC_URL must be set (via app.env or environment)")
var ErrMissingPrivateKey = errors.New("PRIVATE_KEY must be set (via app.env or environment)")
var ErrMissingDatabaseURL = errors.New("DATABASE_URL must be set (via app.env or environment)")
var ErrBadTimeout = errors.New("TIMEOUT_SEC must be more than 0")

// RPCTimeout is the longest one RPC call may take. A node that takes the
// request and never answers gives an error after it, instead of holding
// the caller forever.
func (c Config) RPCTimeout() time.Duration {
	return time.Duration(c.TimeoutSec) * time.Second
}

func LoadConfig(path string) (config Config, err error) {
	viper.AddConfigPath(path)
	viper.SetConfigName("app")
	viper.SetConfigType("env")

	viper.AutomaticEnv()
	viper.SetDefault("confirmations", 12) // Sepolia; also lets CONFIRMATIONS come from env only
	viper.SetDefault("finality", 64)      // two epochs: a block this deep is final
	viper.SetDefault("redis_url", "")     // optional; also lets REDIS_URL come from env only
	viper.SetDefault("send_limit", 5)     // POST /send requests per user per minute
	viper.SetDefault("timeout_sec", 10)   // one RPC call

	if err = viper.ReadInConfig(); err != nil {
		return
	}

	if err = viper.Unmarshal(&config); err != nil {
		return
	}

	if config.RPCURL == "" {
		err = ErrMissingRPCURL
		return
	}

	if config.PrivateKey == "" {
		err = ErrMissingPrivateKey
		return
	}

	if config.DatabaseURL == "" {
		err = ErrMissingDatabaseURL
		return
	}

	if config.TimeoutSec <= 0 {
		err = ErrBadTimeout
		return
	}

	return
}
