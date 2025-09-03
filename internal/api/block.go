package api

import (
	"strconv"

	"github.com/Mohsen20031203/blockchain-insight/internal/enthblock"
	"github.com/gin-gonic/gin"
)

// GetAddressBalance godoc
// @Summary      Get balance of an address
// @Description  Returns the balance of a given blockchain address
// @Tags         address
// @Accept       json
// @Produce      json
// @Param        address   path      string  true  "Blockchain address"
// @Success      200  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Router       /address/{address}/balance [get]
func (s *Server) GetAddressBalance(c *gin.Context) {
	address := c.Param("address")
	balance, err := enthblock.GetBalance(s.client, address)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"balance": balance.String()})
}

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
	block, err := enthblock.GetBlockByNumber(s.client, blockNumber)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, block)
}

// GetLastBlock godoc
// @Summary      Get the latest block
// @Description  Returns the most recent block information from the blockchain
// @Tags         block
// @Accept       json
// @Produce      json
// @Success      200  {object}  models.Block
// @Failure      500  {object}  map[string]string
// @Router       /block/latest [get]
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
