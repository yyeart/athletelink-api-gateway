package grpcapi

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"

	gatewayv1 "gitlab.com/team-anonyms/athelete-link/api-gateway/api/gen/athletelink/gateway/v1"
	"gitlab.com/team-anonyms/athelete-link/api-gateway/internal/coreclient"
	"gitlab.com/team-anonyms/athelete-link/api-gateway/internal/requestcontext"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

type correlationCoreStub struct {
	calls atomic.Int32
	get   func(context.Context, coreclient.GetRequestDetailsInput) (coreclient.ActivityRequestDetails, error)
}

func (s *correlationCoreStub) GetRequestDetails(
	ctx context.Context,
	input coreclient.GetRequestDetailsInput,
) (coreclient.ActivityRequestDetails, error) {
	s.calls.Add(1)
	return s.get(ctx, input)
}

func TestGetRequestDetailsCorrelation(t *testing.T) {
	const requestID = "123e4567-e89b-12d3-a456-426614174001"
	const entityID = "123e4567-e89b-12d3-a456-426614174002"
	const userID = "123e4567-e89b-12d3-a456-426614174003"

	for _, authenticated := range []bool{true, false} {
		name := "success"
		if !authenticated {
			name = "missing_identity"
		}

		t.Run(name, func(t *testing.T) {
			core := &correlationCoreStub{
				get: func(ctx context.Context, input coreclient.GetRequestDetailsInput) (coreclient.ActivityRequestDetails, error) {
					if !authenticated {
						t.Error("Core called without trusted identity")
					}
					if input.RequestID != entityID {
						t.Errorf("Core request ID = %q, want %q", input.RequestID, entityID)
					}
					if id, ok := requestcontext.RequestIDFrom(ctx); !ok || id != requestID {
						t.Errorf("Core correlation ID = %q, present = %v", id, ok)
					}
					id := entityID
					return coreclient.ActivityRequestDetails{ID: &id}, nil
				},
			}

			listener := bufconn.Listen(1024 * 1024)
			server := grpc.NewServer(grpc.UnaryInterceptor(
				func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
					ctx = requestcontext.WithRequestID(ctx, requestID)
					if authenticated {
						ctx = requestcontext.WithIdentity(ctx, requestcontext.Identity{UserID: userID})
					}
					return next(ctx, req)
				},
			))
			gatewayv1.RegisterCoreServiceServer(server, New(core))
			go func() { _ = server.Serve(listener) }()
			t.Cleanup(func() {
				server.Stop()
				_ = listener.Close()
			})

			conn, err := grpc.NewClient(
				"passthrough:///correlation-test",
				grpc.WithTransportCredentials(insecure.NewCredentials()),
				grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
					return listener.Dial()
				}),
			)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = conn.Close() })

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			// Incoming metadata must not replace the trusted fixture or supply identity.
			ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs(
				"x-request-id", "client-supplied-id",
				"x-user-id", userID,
			))
			var headers metadata.MD
			id := entityID
			response, err := gatewayv1.NewCoreServiceClient(conn).GetRequestDetails(
				ctx,
				&gatewayv1.GetRequestDetailsRequest{RequestId: &id},
				grpc.Header(&headers),
			)

			if values := headers.Get("x-request-id"); len(values) != 1 || values[0] != requestID {
				t.Errorf("response x-request-id = %v, want exactly [%s]", values, requestID)
			}

			if authenticated {
				if err != nil {
					t.Fatalf("GetRequestDetails: %v", err)
				}
				if response.GetRequest().GetId() != entityID {
					t.Errorf("response entity ID = %q, want %q", response.GetRequest().GetId(), entityID)
				}
				if calls := core.calls.Load(); calls != 1 {
					t.Errorf("Core calls = %d, want 1", calls)
				}
				return
			}

			if status.Code(err) != codes.Unauthenticated {
				t.Fatalf("status = %v, want UNAUTHENTICATED; error = %v", status.Code(err), err)
			}
			if calls := core.calls.Load(); calls != 0 {
				t.Errorf("Core calls = %d, want 0", calls)
			}
			details := status.Convert(err).Details()
			if len(details) != 1 {
				t.Fatalf("error details = %v, want one GatewayErrorDetail", details)
			}
			detail, ok := details[0].(*gatewayv1.GatewayErrorDetail)
			if !ok {
				t.Fatalf("detail type = %T, want GatewayErrorDetail", details[0])
			}
			if detail.GetCode() != "AUTHENTICATION_REQUIRED" || detail.GetRequestId() != requestID {
				t.Errorf("error detail = %v, want AUTHENTICATION_REQUIRED and request ID %s", detail, requestID)
			}
		})
	}
}
