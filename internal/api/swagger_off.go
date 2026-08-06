//go:build !swagger

package api

import "github.com/gin-gonic/gin"

func mountSwagger(_ *gin.Engine) {}
