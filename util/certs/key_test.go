package certs

import (
	"crypto/rsa"
	"k8s.io/client-go/util/keyutil"
	"testing"
)

func TestLoadCertsFromFile(t *testing.T) {
	prk,err := keyutil.PrivateKeyFromFile("testdata/ca-key.pem")
	if err != nil {
		t.Logf("parse private failed : %v", err)
	}
	prkRSA := prk.(*rsa.PrivateKey)
	t.Logf("primes : %v ", prkRSA.Primes)

	data,err := keyutil.MarshalPrivateKeyToPEM(prkRSA)

	if err != nil {
		t.Logf("MarshalPrivateKeyToPEM failed : %v", err)
	}



	prkRSA2,err  := keyutil.ParsePrivateKeyPEM(data)
	if err != nil {
		t.Logf("ParsePrivateKeyPEM failed : %v", err)
	}

	t.Logf("should be equeal : %v", prkRSA.Equal(prkRSA2))


	pks, err := keyutil.PublicKeysFromFile("testdata/ca.pem")
	if err != nil {
		t.Logf("parse public failed : %v", err)
	}
	for _, pk := range pks {
		pkRSA := pk.(*rsa.PublicKey)
		t.Logf("E: %d",pkRSA.E)
	}





	//pk := prk.(rsa.PrivateKey)
	//t.Logf("primes : %v ", prkRSA.Primes)


}