package api

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// GetBlockById godoc
// @Summary      Get block by number
// @Description  Returns the block information for the given block number
// @Tags         block
// @Accept       json
// @Produce      json
// @Param        id   path      int     true  "Block number"
// @Success      200  {object}  models.Block
// @Failure      400  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Router       /block/{id} [get]
func (s *Server) GetBlockById(c *gin.Context) {
	id := c.Param("id")

	blockNumber, err := strconv.ParseUint(id, 10, 64)
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid block number"})
		return
	}
	block, err := s.client.GetBlockByNumber(blockNumber)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, block)
}

// GetLastBlock godoc
// @Summary      Get the last block
// @Description  Returns the most recent block information from the blockchain
// @Tags         block
// @Accept       json
// @Produce      json
// @Success      200  {object}  models.Block
// @Failure      500  {object}  map[string]string
// @Router       /last/block [get]
func (s *Server) GetLastBlock(c *gin.Context) {

	blockNumber, err := s.client.GetLastBlockNumber()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	block, err := s.client.GetBlockByNumber(blockNumber)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	s.cach.Set(LastBlock, block, 10*time.Second)
	c.JSON(200, block)
}
