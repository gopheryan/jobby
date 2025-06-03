package commands

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

const (
	clientKeyPath  = "client/ryan/client.key"
	clientCertPath = "client/ryan/client.crt"
	caPath         = "ca/ca.crt"
)

func init() {
	rootCmd.PersistentFlags().String("host", "localhost:8443", "server hostname:port")
}

var rootCmd = &cobra.Command{
	Use:          "jobcli",
	Short:        "A command line client for Jobby Job Management Servers",
	SilenceUsage: true,
	CompletionOptions: cobra.CompletionOptions{
		DisableDefaultCmd: true,
	},
}

func newClientConnection(host string) (*grpc.ClientConn, error) {
	cfg, err := newTLSConfig()
	if err != nil {
		return nil, fmt.Errorf("error creating TLS config: %w", err)
	}
	return grpc.NewClient(host, grpc.WithTransportCredentials(credentials.NewTLS(cfg)))
}

func newTLSConfig() (*tls.Config, error) {
	clientCert, err := tls.LoadX509KeyPair(clientCertPath, clientKeyPath)
	if err != nil {
		return nil, fmt.Errorf("error loading client key/cert: %w", err)
	}

	caData, err := os.ReadFile(caPath)
	if err != nil {
		return nil, fmt.Errorf("error loading ca certificate %w", err)
	}

	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(caData)

	return &tls.Config{
		RootCAs:      pool,
		Certificates: []tls.Certificate{clientCert},
		MinVersion:   tls.VersionTLS13,
	}, nil
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
