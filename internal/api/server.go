package api

import (
	"context"
	"errors"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"time"

	"github.com/Mohsen20031203/blockchain-insight/config"
	"github.com/Mohsen20031203/blockchain-insight/internal/enth"
	"github.com/Mohsen20031203/blockchain-insight/internal/watch"
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
	store   *watch.Store
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
		store:  watch.NewStore(),
	}

	server.setupRouter()
	return server

}

// setupRouter initializes the Gin router and sets up the routes and middleware.
func (s *Server) setupRouter() {
	router := gin.New()
	router.Use(SlogLogger(), gin.Recovery())
	router.Use(gzip.Gzip(gzip.BestSpeed))

	router.GET("/balance/:address", s.GetAddressBalance)
	router.GET("/block/:id", s.GetBlockById)
	router.GET("/last/block", s.Cache(), s.GetLastBlock)
	router.POST("/watch", s.PostWatch)
	router.GET("/watch/:address/transfers", s.GetWatchTransfers)

	// Swagger — mounted only when built with `-tags swagger`
	mountSwagger(router)

	if os.Getenv("ENABLE_PPROF") == "true" {
		router.GET("/debug/pprof/*any", gin.WrapH(http.DefaultServeMux))
	}

	s.router = router
}

// Start serves HTTP until ctx is cancelled, then shuts down gracefully.
func (s *Server) Start(ctx context.Context, addr string) error {
	srv := &http.Server{Addr: addr, Handler: s.router}

	errCh := make(chan error, 1)
	go func() {
		err := srv.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		errCh <- err
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		// NOT ctx: it is already cancelled. Shutdown needs its own deadline
		// to let in-flight requests finish.
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}
