package api

import (
	"log"

	"github.com/Mohsen20031203/blockchain-insight/config"
	"github.com/Mohsen20031203/blockchain-insight/internal/enthblock"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gin-gonic/gin"
)

type Server struct {
	client *ethclient.Client
	config config.Config
	router *gin.Engine
}

func NewServer(config config.Config) *Server {

	client, err := enthblock.NewClient(config.RPCURL)
	if err != nil {
		log.Fatal(err)
	}

	server := &Server{
		client: client,
		config: config,
	}

	server.setupRouter()
	return server

}

func (s *Server) setupRouter() {
	router := gin.Default()

	s.router = router
}

func (s *Server) Start(addr string) error {
	return s.router.Run(addr)
}
