package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net"
	"os"
	"os/signal"

	"github.com/gopheryan/jobby/internal/authinterceptors"
	"github.com/gopheryan/jobby/internal/service"
	grpc_recovery "github.com/grpc-ecosystem/go-grpc-middleware/recovery"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	grpc_reflection "google.golang.org/grpc/reflection"
)

const address = "localhost:8443"

type UserGetterFunc func(context.Context) string

func (u UserGetterFunc) GetUserContext(ctx context.Context) string {
	return u(ctx)
}

// we have log.Fatal, but let's be consistent with slog
func slogFatal(msg string, args ...any) {
	slog.Error(msg, args...)
	os.Exit(1)
}

func main() {

	tlsConfig, err := NewTLSConfig()
	if err != nil {
		slogFatal("Failed to create TLS config", "error", err)
	}
	// Harcoded!
	listener, err := net.Listen("tcp", address)
	if err != nil {
		slogFatal("Failed to create TLS listener", "error", err)
	}
	defer listener.Close()

	grpcServer := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			grpc_recovery.UnaryServerInterceptor(),
			authinterceptors.AuthHandlerUnaryInterceptor,
		),
		grpc.ChainStreamInterceptor(
			grpc_recovery.StreamServerInterceptor(),
			authinterceptors.AuthHandlerStreamInterceptor,
		),
		grpc.Creds(credentials.NewTLS(&tlsConfig)),
	)

	jobbyService := service.NewJobService(UserGetterFunc(authinterceptors.GetUserContext), os.TempDir())
	jobbyService.Register(grpcServer)

	// So I can poke at this thing with grpcurl
	grpc_reflection.Register(grpcServer)

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt)

	// Catch sigterm and exit
	go func() {
		<-signalChan
		slog.Info("Caught signal. Stopping Server")
		grpcServer.Stop()
	}()

	slog.Info("Listening for gRPC requests!", "address", address)
	err = grpcServer.Serve(listener)
	if err != nil {
		log.Fatalf("gRPC server returned with error: %s", err)
	}

	slog.Info("nighty night!")
}

// Hardcoded!
func NewTLSConfig() (tls.Config, error) {
	localPool := x509.NewCertPool()

	caCertData, err := os.ReadFile("ca/ca.crt")
	if err != nil {
		return tls.Config{}, fmt.Errorf("error loading ca crt: %w", err)
	}

	ok := localPool.AppendCertsFromPEM(caCertData)
	if !ok {
		return tls.Config{}, errors.New("error parsing ca cert")
	}

	serverCertificate, err := tls.LoadX509KeyPair("server/server.crt", "server/server.key")
	if err != nil {
		return tls.Config{}, fmt.Errorf("error loading server cert/key: %w", err)
	}

	return tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{serverCertificate},
		ClientCAs:    localPool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
	}, nil
}
