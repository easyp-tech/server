package https

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"net/netip"
	"os"
	"time"
)

const ReadHeaderTimeout = 10 * time.Second

func ListenAndServe(
	listen netip.AddrPort,
	handler http.Handler,
	certFileName,
	keyFileName,
	caFileName string,
) error {
	server := &http.Server{ //nolint:exhaustruct
		Addr:              listen.String(),
		Handler:           handler,
		ReadHeaderTimeout: ReadHeaderTimeout,
	}

	if caFileName != "" {
		caCertPool, err := loadCA(caFileName)
		if err != nil {
			return fmt.Errorf("loading CA %q: %w", caFileName, err)
		}

		server.TLSConfig = &tls.Config{ //nolint:exhaustruct
			ClientCAs:  caCertPool,
			ClientAuth: tls.RequireAndVerifyClientCert,
			MinVersion: tls.VersionTLS13,
		}
	}

	return server.ListenAndServeTLS(certFileName, keyFileName) //nolint:wrapcheck
}

func loadCA(fileName string) (*x509.CertPool, error) {
	caCert, err := os.ReadFile(fileName)
	if err != nil {
		return nil, fmt.Errorf("loading cert: %w", err)
	}

	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	return caCertPool, nil
}
