package api

import (
	"github.com/Mohsen20031203/blockchain-insight/internal/models"
	"github.com/gin-gonic/gin"
)

func (s *Server) Cache() gin.HandlerFunc {
	return func(c *gin.Context) {

		if cached, ok := s.cach.Get(LastBlock); ok {
			blk := cached.(*models.Block)
			c.JSON(200, blk)

			c.Abort()
			return
		}

		c.Next()
	}
}
