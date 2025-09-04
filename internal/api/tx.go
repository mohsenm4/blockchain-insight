package api

import "github.com/gin-gonic/gin"

// GetTxByHash godoc
// @Summary      Get transaction by hash
// @Description  Returns the transaction details for the given transaction hash
// @Tags         transaction
// @Accept       json
// @Produce      json
// @Param        tx   path      string  true  "Transaction hash"
// @Success      200  {object}  models.Transaction
// @Failure      500  {object}  map[string]string
// @Router       /tx/{hash} [get]
func (s *Server) GetTxByHash(c *gin.Context) {
	txHash := c.Param("tx")
	hash, err := s.client.GetTxByHash(txHash)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	tx := s.client.ConvertTx(hash)
	c.JSON(200, tx)
}

// GetAddressBalance godoc
// @Summary      Get balance of an address
// @Description  Returns the balance of a given blockchain address
// @Tags         block
// @Accept       json
// @Produce      json
// @Param        address   path      string  true  "Blockchain address"
// @Success      200  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Router       /balace/{address} [get]
func (s *Server) GetAddressBalance(c *gin.Context) {
	address := c.Param("address")
	balance, err := s.client.GetBalance(address)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"balance": balance.String()})
}
