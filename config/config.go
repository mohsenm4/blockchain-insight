package config

import (
	"errors"

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
	Confirmations uint64 `mapstructure:"confirmations"`
}

var ErrMissingRPCURL = errors.New("RPC_URL must be set (via app.env or environment)")
var ErrMissingPrivateKey = errors.New("PRIVATE_KEY must be set (via app.env or environment)")
var ErrMissingDatabaseURL = errors.New("DATABASE_URL must be set (via app.env or environment)")

func LoadConfig(path string) (config Config, err error) {
	viper.AddConfigPath(path)
	viper.SetConfigName("app")
	viper.SetConfigType("env")

	viper.AutomaticEnv()
	viper.SetDefault("confirmations", 12) // Sepolia; also lets CONFIRMATIONS come from env only

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

	return
}
