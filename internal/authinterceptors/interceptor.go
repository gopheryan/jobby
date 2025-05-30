package authinterceptors

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

type userID string

const userValue userID = "jobby"

func GetUserContext(ctx context.Context) string {
	return string(ctx.Value(userValue).(string))
}

func WithUser(ctx context.Context, user string) context.Context {
	return context.WithValue(ctx, userValue, user)
}

// Dig into the context until we find the certificate
// presented by the client. This function assumes that clients
// will present exactly one certificate to the server
func getUser(ctx context.Context) (string, error) {
	peerInfo, ok := peer.FromContext(ctx)
	if !ok {
		return "", status.Error(codes.Unknown, "Could not determine peer info")
	}

	tls, ok := peerInfo.AuthInfo.(credentials.TLSInfo)
	if !ok {
		return "", status.Error(codes.Unauthenticated, "No TLS info")
	}

	if len(tls.State.PeerCertificates) == 1 {
		// huzzah!
		user := tls.State.PeerCertificates[0].Subject.CommonName
		return user, nil
	} else {
		return "", status.Error(codes.Unauthenticated, "Client must present exactly one certificate")
	}
}

func AuthHandlerUnaryInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
	user, err := getUser(ctx)
	if err != nil {
		return nil, err
	}
	ctx = WithUser(ctx, user)
	return handler(ctx, req)
}

type replacementStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (r *replacementStream) Context() context.Context {
	return r.ctx
}

func AuthHandlerStreamInterceptor(srv interface{}, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) (err error) {
	user, err := getUser(stream.Context())
	if err != nil {
		return err
	}
	// We can't poke a new context into the existing server stream,
	// but we can replace the server stream entirely with our own thin wrapper
	return handler(srv, &replacementStream{
		ServerStream: stream,
		ctx:          WithUser(stream.Context(), user),
	})
}
