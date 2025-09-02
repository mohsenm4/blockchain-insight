package api

import (
	"github.com/Mohsen20031203/blockchain-insight/internal/enthblock"
	"github.com/gin-gonic/gin"
)

func (s *Server) LatestBlockNumber(c *gin.Context) {
	address := c.Param("address")
	balance, err := enthblock.GetBalance(s.client, address)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"balance": balance.String()})
}
