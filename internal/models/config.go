package models

type Config struct {
	RPCURL      string `mapstructure:"rpc_url"`
	NetworkName string `mapstructure:"network_name"`
	TimeoutSec  int    `mapstructure:"timeout_sec"`
}
