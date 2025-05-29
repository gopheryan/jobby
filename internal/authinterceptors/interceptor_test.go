package authinterceptors

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

func TestInterceptors(t *testing.T) {
	p := peer.Peer{
		AuthInfo: credentials.TLSInfo{
			State: tls.ConnectionState{
				PeerCertificates: []*x509.Certificate{
					&x509.Certificate{
						Subject: pkix.Name{
							CommonName: "Ryan",
						},
					},
				},
			},
		},
	}
	ctx := peer.NewContext(context.Background(), &p)

	t.Run("unary", func(tt *testing.T) {
		_, err := AuthHandlerUnaryInterceptor(ctx, nil, nil, func(ctx context.Context, _ any) (any, error) {
			assert.Equal(t, "Ryan", GetUserContext(ctx))
			return nil, nil
		})
		assert.NoError(tt, err)
	})

	t.Run("stream", func(tt *testing.T) {
		rs := &replacementStream{
			ctx: ctx,
		}
		err := AuthHandlerStreamInterceptor(nil, rs, nil, func(srv any, stream grpc.ServerStream) error {
			assert.Equal(t, "Ryan", GetUserContext(stream.Context()))
			return nil
		})
		assert.NoError(tt, err)
	})

	t.Run("no-peer", func(tt *testing.T) {
		ctx := context.Background()
		_, err := AuthHandlerUnaryInterceptor(ctx, nil, nil, func(ctx context.Context, _ any) (any, error) {
			assert.Equal(t, "Ryan", GetUserContext(ctx))
			return nil, nil
		})
		assert.Error(tt, err)
		s, ok := status.FromError(err)
		assert.True(tt, ok)
		assert.Equal(tt, codes.Unknown, s.Code())
	})

	t.Run("no-cert", func(tt *testing.T) {
		p := peer.Peer{
			AuthInfo: credentials.TLSInfo{
				State: tls.ConnectionState{},
			},
		}
		ctx := peer.NewContext(context.Background(), &p)
		_, err := AuthHandlerUnaryInterceptor(ctx, nil, nil, func(ctx context.Context, _ any) (any, error) {
			assert.Equal(t, "Ryan", GetUserContext(ctx))
			return nil, nil
		})
		assert.Error(tt, err)
		s, ok := status.FromError(err)
		assert.True(tt, ok)
		assert.Equal(tt, codes.Unauthenticated, s.Code())
	})

}
