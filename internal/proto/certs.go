package proto

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
)

func NewCertificates(certFile, keyFile, caFile string) ([]tls.Certificate, *x509.CertPool, error) {

	c, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, nil, fmt.Errorf("server X509: %w", err)
	}
	data, err := os.ReadFile(caFile)
	if err != nil {
		return nil, nil, fmt.Errorf("CA cert: %w", err)
	}
	p := x509.NewCertPool()
	if !p.AppendCertsFromPEM(data) {
		return nil, nil, fmt.Errorf("CA cert pool")
	}

	return []tls.Certificate{c}, p, nil
}
