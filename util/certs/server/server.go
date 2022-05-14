package main

import (
	"flag"
	"github.com/gin-gonic/gin"
)

var (
	privateKey = flag.String("private-key","./private.key","server private key")
	publicCert = flag.String("public-cert","server.crt","server public key")
)

func main() {
	r := gin.Default()

	// Ping handler
	r.GET("/ping", func(c *gin.Context) {
		c.String(200, "pong")
	})

	err := r.RunTLS(":8080",*publicCert,*privateKey)
	if err != nil {
		panic(err)
	}
}