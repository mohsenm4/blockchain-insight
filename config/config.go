package config

import (
	"errors"

	"github.com/spf13/viper"
)

type Config struct {
	RPCURL      string `mapstructure:"rpc_url"`
	NetworkName string `mapstructure:"network_name"`
	TimeoutSec  int    `mapstructure:"timeout_sec"`
}

var ErrMissingRPCURL = errors.New("RPC_URL must be set (via app.env or environment)")

func LoadConfig(path string) (config Config, err error) {
	viper.AddConfigPath(path)
	viper.SetConfigName("app")
	viper.SetConfigType("env")

	viper.AutomaticEnv()

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
	return
}
