//go:build swagger

package api

import (
	"github.com/gin-gonic/gin"
	_ "github.com/mohsenm4/blockchain-insight/docs"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

func mountSwagger(router *gin.Engine) {
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
}
