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
