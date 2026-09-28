package grpcapi

import (
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func UnknownServiceHandler(newRequestID func() string) grpc.ServerOption {
	return grpc.UnknownServiceHandler(func(_ any, stream grpc.ServerStream) error {
		requestID := newRequestID()
		if requestID == "" {
			return status.Error(codes.Internal, "Internal Gateway error")
		}

		if err := stream.SetHeader(metadata.Pairs("x-request-id", requestID)); err != nil {
			return gatewayFailure(
				codes.Internal,
				"INTERNAL_ERROR",
				"Internal Gateway error",
				requestID,
			)
		}

		return gatewayFailure(
			codes.Unimplemented,
			"METHOD_NOT_IMPLEMENTED",
			"RPC method not implemented",
			requestID,
		)
	})
}
