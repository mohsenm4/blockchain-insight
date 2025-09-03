package api

import (
	"github.com/gin-gonic/gin"
)

func (s *Server) initCache(c *gin.Context) {

	block, ok := s.cach.Get("latest_block")
	if ok {
		c.JSON(200, block)
		return
	}

}
