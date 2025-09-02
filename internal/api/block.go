package api

import (
	"strconv"

	"github.com/Mohsen20031203/blockchain-insight/internal/enthblock"
	"github.com/gin-gonic/gin"
)

func (s *Server) GetAddressBalance(c *gin.Context) {
	address := c.Param("address")
	balance, err := enthblock.GetBalance(s.client, address)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"balance": balance.String()})
}

func (s *Server) GetBlockById(c *gin.Context) {
	id := c.Param("id")

	blockNumber, err := strconv.ParseUint(id, 10, 64)
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid block number"})
		return
	}
	block, err := enthblock.GetBlockByNumber(s.client, blockNumber)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, block)
}

func (s *Server) GetLastBlock(c *gin.Context) {

	blockNumber, err := enthblock.GetLatestBlockNumber(s.client)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	block, err := enthblock.GetBlockByNumber(s.client, blockNumber)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, block)
}
