package config

import (
	"log"

	"github.com/spf13/viper"
)

type Config struct {
	RPCURL      string `mapstructure:"rpc_url"`
	NetworkName string `mapstructure:"network_name"`
	TimeoutSec  int    `mapstructure:"timeout_sec"`
}

func LoadConfig(path string) (config Config, err error) {
	viper.AddConfigPath(path)
	viper.SetConfigName("app")
	viper.SetConfigType("env")

	viper.AutomaticEnv()

	err = viper.ReadInConfig()
	if err != nil {
		return
	}

	err = viper.Unmarshal(&config)
	url := config.RPCURL
	if url == "" {
		log.Fatal("Environment variable RPC_URL must be set")
	}
	return
}
