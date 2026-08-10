package api

import (
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"time"

	"github.com/Mohsen20031203/blockchain-insight/config"
	"github.com/Mohsen20031203/blockchain-insight/internal/enth"
	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
	"github.com/patrickmn/go-cache"
	"golang.org/x/sync/singleflight"
)

type Server struct {
	client  EthClient
	config  config.Config
	router  *gin.Engine
	cach    *cache.Cache
	sfGroup singleflight.Group
}

const LastBlock = "last_block"

func NewServer(config config.Config) *Server {
	client, err := enth.NewClient(config.RPCURL)
	if err != nil {
		log.Fatal(err)
	}

	cach := cache.New(cache.NoExpiration, 1*time.Hour)
	server := &Server{
		client:  client,
		config:  config,
		cach:    cach,
		sfGroup: singleflight.Group{},
	}

	server.setupRouter()
	return server

}

// setupRouter initializes the Gin router and sets up the routes and middleware.
func (s *Server) setupRouter() {
	router := gin.Default()
	router.Use(gzip.Gzip(gzip.BestSpeed))

	router.GET("/balance/:address", s.GetAddressBalance)
	router.GET("/block/:id", s.GetBlockById)
	router.GET("/last/block", s.Cache(), s.GetLastBlock)

	// Swagger — mounted only when built with `-tags swagger`
	mountSwagger(router)

	if os.Getenv("ENABLE_PPROF") == "true" {
		router.GET("/debug/pprof/*any", gin.WrapH(http.DefaultServeMux))
	}

	s.router = router
}

// Start runs the Gin server on the specified address.
func (s *Server) Start(addr string) error {
	return s.router.Run(addr)
}
