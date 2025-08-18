package tls

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net/http"

	"distributed-job-processor/internal/config"
	"distributed-job-processor/internal/logger"
)

type TLSConfig struct {
	Enabled  bool
	CertFile string
	KeyFile  string
}

func NewTLSConfig(cfg *config.SecurityConfig) *TLSConfig {
	return &TLSConfig{
		Enabled:  cfg.TLSEnabled,
		CertFile: cfg.CertFile,
		KeyFile:  cfg.KeyFile,
	}
}

func (t *TLSConfig) CreateTLSConfig() (*tls.Config, error) {
	if !t.Enabled {
		return nil, nil
	}

	cert, err := tls.LoadX509KeyPair(t.CertFile, t.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load TLS certificate: %w", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		},
	}

	return tlsConfig, nil
}

func (t *TLSConfig) CreateHTTPSServer(handler http.Handler, addr string) (*http.Server, error) {
	if !t.Enabled {
		return &http.Server{
			Addr:    addr,
			Handler: handler,
		}, nil
	}

	tlsConfig, err := t.CreateTLSConfig()
	if err != nil {
		return nil, err
	}

	server := &http.Server{
		Addr:      addr,
		Handler:   handler,
		TLSConfig: tlsConfig,
	}

	logger.WithField("addr", addr).Info("TLS enabled for HTTP server")
	return server, nil
}

func (t *TLSConfig) CreateHTTPClient(caCertFile string) (*http.Client, error) {
	if !t.Enabled {
		return &http.Client{}, nil
	}

	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	if caCertFile != "" {
		caCert, err := ioutil.ReadFile(caCertFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA certificate: %w", err)
		}

		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}

		tlsConfig.RootCAs = caCertPool
	}

	transport := &http.Transport{
		TLSClientConfig: tlsConfig,
	}

	return &http.Client{
		Transport: transport,
	}, nil
}

func GenerateSelfSignedCert(certFile, keyFile string) error {
	logger.Info("Generating self-signed certificate for development")
	
	return fmt.Errorf("certificate generation not implemented - use openssl or similar tool")
}