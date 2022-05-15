package main

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"log"
	"math/big"
	"os"
	"time"
)

func GenerateSelfSignedCertificate(commonName, organization, keyFile, crtFile string) {
	// Generate key
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		log.Fatal("Private key cannot be created.", err.Error())
	}

	// Generate a pem block with the private key
	keyBuf := new(bytes.Buffer)
	err = pem.Encode(keyBuf, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})

	if err == nil {
		_ = os.WriteFile(keyFile, keyBuf.Bytes(), 0600)
	}

	// Generate certificate:
	tml := x509.Certificate{
		NotBefore:    time.Now(),
		NotAfter:     time.Now().AddDate(5, 0, 0),
		SerialNumber: big.NewInt(time.Now().Unix()),
		Subject: pkix.Name{
			CommonName:   commonName,
			Organization: []string{organization},
		},
		BasicConstraintsValid: true,
	}
	cert, err := x509.CreateCertificate(rand.Reader, &tml, &tml, &key.PublicKey, key)
	if err != nil {
		log.Fatal("Certificate cannot be created.", err.Error())
	}

	// Generate a pem block with the certificate
	certBuf := new(bytes.Buffer)
	err = pem.Encode(certBuf, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert,
	})

	if err == nil {
		_ = os.WriteFile(crtFile, certBuf.Bytes(), 0600)
	}
}
