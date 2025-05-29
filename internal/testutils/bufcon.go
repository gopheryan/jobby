package testutils

import (
	"context"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

const defaultBufferSize = 1024 * 1024

type GrpcLocalServer struct {
	BufferSize int
	clientConn *grpc.ClientConn
	srvDone    chan error
}

// Internally creates a goroutine to serve gRPC requests.
// Stop serving by calling 'Stop' on the provided server
// and wait for the exit status by calling 'Done'
func (g *GrpcLocalServer) ListenAndServe(srv *grpc.Server) error {
	if g.BufferSize <= 0 {
		g.BufferSize = defaultBufferSize
	}
	l := bufconn.Listen(g.BufferSize)
	g.srvDone = make(chan error)
	go func() {
		defer l.Close()
		defer close(g.srvDone)
		g.srvDone <- srv.Serve(l)
	}()

	var err error
	g.clientConn, err = grpc.NewClient("passthrough://bufnet", grpc.WithContextDialer(func(_ context.Context, _ string) (net.Conn, error) {
		return l.Dial()
	}), grpc.WithTransportCredentials(insecure.NewCredentials()))
	return err
}

// Returns a grpc client connection that can be used
// to send requests to the server
func (g *GrpcLocalServer) Conn() *grpc.ClientConn {
	return g.clientConn
}

// Waits for 'Serve' on the provided gRPC server
// to exit and passes along its error value
func (g *GrpcLocalServer) Done() error {
	return <-g.srvDone
}
