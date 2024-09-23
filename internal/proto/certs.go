package proto

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
)

var ErrCertPool = errors.New("cert pool")

// NewCertificates reads in cert, key and CA file
func NewCertificates(certFile, keyFile, caFile string) ([]tls.Certificate, *x509.CertPool, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, nil, fmt.Errorf("server X509: %w", err)
	}
	data, err := os.ReadFile(caFile)
	if err != nil {
		return nil, nil, fmt.Errorf("CA cert: %w", err)
	}
	p := x509.NewCertPool()
	if !p.AppendCertsFromPEM(data) {
		return nil, nil, ErrCertPool
	}

	return []tls.Certificate{cert}, p, nil
}
