package sso

import "github.com/gin-gonic/gin"

func RegisterRoutes(c *gin.Engine) {
	c.GET("/sso/redirect", ssoRedirectHandler)
	c.GET("/sso/callback", ssoCallbackHandler)
}
