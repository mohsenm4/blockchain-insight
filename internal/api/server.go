package api

import (
	"log"
	"time"

	"github.com/Mohsen20031203/blockchain-insight/config"
	_ "github.com/Mohsen20031203/blockchain-insight/docs"
	"github.com/Mohsen20031203/blockchain-insight/internal/enth"
	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
	"github.com/patrickmn/go-cache"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

type Server struct {
	client *enth.Client
	config config.Config
	router *gin.Engine
	cach   *cache.Cache
}

const LastBlock = "last_block"

func NewServer(config config.Config) *Server {
	client, err := enth.NewClient(config.RPCURL)
	if err != nil {
		log.Fatal(err)
	}

	cach := cache.New(cache.NoExpiration, 1*time.Hour)
	server := &Server{
		client: client,
		config: config,
		cach:   cach,
	}

	go func() {
		for {
			time.Sleep(10 * time.Second)
			// get last block from blockchain
			block, err := client.GetLatestBlockNumber()
			if err != nil {
				continue
			}
			lastBlock, err := client.GetBlockByNumber(block)
			if err != nil {
				continue
			}
			// save block to cache
			cach.Set(LastBlock, lastBlock, cache.DefaultExpiration)
		}
	}()

	server.setupRouter()
	return server

}

func (s *Server) setupRouter() {
	router := gin.Default()
	router.Use(gzip.Gzip(gzip.DefaultCompression))

	router.GET("/balace/:address", s.GetAddressBalance)
	router.GET("/block/:id", s.GetBlockById)
	router.GET("/last/block", s.Cache(), s.GetLastBlock)

	// Swagger
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	s.router = router
}

func (s *Server) Start(addr string) error {
	return s.router.Run(addr)
}
