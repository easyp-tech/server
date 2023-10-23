//nolint:wrapcheck
package grpchelper

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

// GRPCCodesConverterHandler is a function that convert your error to gRPC codes.
// The context can be used to extract request scoped metadata and context values.
type GRPCCodesConverterHandler = func(error) *status.Status

// UnaryConvertCodesServerInterceptor returns a new unary server interceptor that converting returns error.
func UnaryConvertCodesServerInterceptor(converter GRPCCodesConverterHandler) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		resp, err := handler(ctx, req)
		if err != nil {
			return nil, converter(err).Err()
		}

		return resp, err
	}
}

// StreamConvertCodesServerInterceptor returns a new unary server interceptor that converting returns error.
func StreamConvertCodesServerInterceptor(converter GRPCCodesConverterHandler) grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		stream grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		err := handler(srv, stream)
		if err != nil {
			return converter(err).Err()
		}

		return nil
	}
}
