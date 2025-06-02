package commands

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"

	"github.com/gopheryan/jobby/jobmanagerpb"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

const (
	clientKeyPath  = "client/ryan/client.key"
	clientCertPath = "client/ryan/client.crt"
	caPath         = "ca/ca.crt"
)

var (
	jobmanagerClient jobmanagerpb.JobManagerClient
	grpcClient       *grpc.ClientConn
)

func init() {
	rootCmd.PersistentFlags().String("host", "localhost:8443", "server hostname:port")

	// I tried using 'PostRun' on the root command, but that
	// apparently doesn't run if your RunE function returns an error
	cobra.OnFinalize(func() {
		if grpcClient != nil {
			_ = grpcClient.Close()
			grpcClient = nil
		}
	})
}

var rootCmd = &cobra.Command{
	Use:          "jobcli",
	Short:        "A command line client for Jobby Job Management Servers",
	SilenceUsage: true,
	CompletionOptions: cobra.CompletionOptions{
		DisableDefaultCmd: true,
	},
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		host, _ := cmd.Flags().GetString("host")
		cfg, err := newTLSConfig()
		if err != nil {
			return fmt.Errorf("error creating TLS config: %w", err)
		}

		grpcClient, err = grpc.NewClient(host, grpc.WithTransportCredentials(credentials.NewTLS(cfg)))
		jobmanagerClient = jobmanagerpb.NewJobManagerClient(grpcClient)
		return err
	},
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
