package api

import (
	"github.com/Mohsen20031203/blockchain-insight/config"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gin-gonic/gin"
)

type Server struct {
	client *ethclient.Client
	config config.Config
	router *gin.Engine
}
