package ch3

import (
	"crypto/x509"
	"fmt"
	"k8s.io/client-go/util/cert"
	cryptorand "crypto/rand"
	"crypto/rsa"
	"net"
	"testing"
)

const COMMON_NAME = "foo.example.com"
func TestNewSelfSignedCACert(t *testing.T) {
	key, err := rsa.GenerateKey(cryptorand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa key failed to generate: %v", err)
	}
	selfSingedCert , err := cert.NewSelfSignedCACert(cert.Config{
		CommonName:   COMMON_NAME,
		Organization: nil,
		AltNames:     cert.AltNames{},
		Usages:       nil,
	},key)
	for _, dns := range selfSingedCert.DNSNames {
		t.Log(dns)
	}
}

