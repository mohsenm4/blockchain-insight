package api

import "github.com/gin-gonic/gin"

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
