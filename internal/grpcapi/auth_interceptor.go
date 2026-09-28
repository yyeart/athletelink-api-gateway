package grpcapi

import (
	"context"
	"errors"
	"strings"

	"gitlab.com/team-anonyms/athelete-link/api-gateway/internal/auth"
	"gitlab.com/team-anonyms/athelete-link/api-gateway/internal/requestcontext"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
)

type AccessVerifier interface {
	VerifyAccessToken(
		context.Context,
		string,
	) (requestcontext.Identity, error)
}

func AuthInterceptor(
	verifier AccessVerifier,
	newRequestID func() string,
) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		_ *grpc.UnaryServerInfo,
		next grpc.UnaryHandler,
	) (any, error) {
		requestID := newRequestID()
		if requestID == "" {
			return nil, gatewayFailure(
				codes.Internal, "INTERNAL_ERROR", "Internal Gateway error", "",
			)
		}

		ctx = requestcontext.WithRequestID(ctx, requestID)
		if err := grpc.SetHeader(
			ctx, metadata.Pairs("x-request-id", requestID),
		); err != nil {
			return nil, gatewayFailure(
				codes.Internal, "INTERNAL_ERROR",
				"Internal Gateway error", requestID,
			)
		}

		raw, ok := bearerToken(ctx)
		if !ok {
			return nil, authenticationRequired(requestID)
		}

		identity, err := verifier.VerifyAccessToken(ctx, raw)
		if err != nil {
			return nil, accessVerificationError(ctx, err, requestID)
		}

		return next(requestcontext.WithIdentity(ctx, identity), req)
	}
}

func bearerToken(ctx context.Context) (string, bool) {
	md, _ := metadata.FromIncomingContext(ctx)
	values := md.Get("authorization")
	if len(values) != 1 || !strings.HasPrefix(values[0], "Bearer ") {
		return "", false
	}
	raw := strings.TrimPrefix(values[0], "Bearer ")
	return raw, raw != "" && !strings.ContainsAny(raw, " \t\r\n")
}

func accessVerificationError(ctx context.Context, err error, requestID string) error {
	if ctx.Err() != nil {
		return mapContextError(ctx.Err(), requestID)
	}
	if errors.Is(err, auth.ErrCheckUnavailable) {
		return gatewayFailure(
			codes.Unavailable,
			"AUTH_CHECK_UNAVAILABLE",
			"Authentication service unavailable",
			requestID,
		)
	}
	return authenticationRequired(requestID)
}

func authenticationRequired(requestID string) error {
	return gatewayFailure(
		codes.Unauthenticated,
		"AUTHENTICATION_REQUIRED",
		"Authentication required",
		requestID,
	)
}
