package main

import (
	"github.com/gin-gonic/gin"
)

func main() {
	r := gin.Default()

	// Ping handler
	r.GET("/ping", func(c *gin.Context) {
		c.String(200, "pong")
	})

	r.RunTLS(":8080","/Users/weenxin//go/src/github.com/weenxin/test_circle_ci/server.csr",
		"/Users/weenxin/go/src/github.com/weenxin/test_circle_ci/server.key")
}