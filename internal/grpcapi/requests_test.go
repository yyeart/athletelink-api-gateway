package grpcapi

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"

	gatewayv1 "gitlab.com/team-anonyms/athelete-link/api-gateway/api/gen/athletelink/gateway/v1"
	"gitlab.com/team-anonyms/athelete-link/api-gateway/internal/auth"
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
	unimplementedCoreStub
	calls atomic.Int32
	get   func(context.Context, coreclient.GetRequestDetailsInput) (coreclient.ActivityRequestDetails, error)
}

type unimplementedCoreStub struct{}

func (unimplementedCoreStub) UpdateRequest(context.Context, coreclient.UpdateRequestInput) (coreclient.ActivityRequestDetails, error) {
	panic("unexpected UpdateRequest")
}

func (unimplementedCoreStub) SearchNearbyRequests(context.Context, coreclient.SearchNearbyRequestsInput) ([]coreclient.ActivityRequestFeed, error) {
	panic("unexpected SearchNearbyRequests")
}

func (unimplementedCoreStub) CreateRequest(context.Context, coreclient.CreateRequestInput) (string, error) {
	panic("unexpected CreateRequest")
}

func (unimplementedCoreStub) OpenRegistration(context.Context, coreclient.ActionInput) error {
	panic("unexpected OpenRegistration")
}

func (unimplementedCoreStub) CloseRegistration(context.Context, coreclient.ActionInput) error {
	panic("unexpected CloseRegistration")
}

func (unimplementedCoreStub) LeaveRequest(context.Context, coreclient.ActionInput) error {
	panic("unexpected LeaveRequest")
}

func (unimplementedCoreStub) JoinRequest(context.Context, coreclient.ActionInput) error {
	panic("unexpected JoinRequest")
}

func (unimplementedCoreStub) KickParticipant(context.Context, coreclient.KickParticipantInput) error {
	panic("unexpected KickParticipant")
}

func (unimplementedCoreStub) StartRequest(context.Context, coreclient.ActionInput) error {
	panic("unexpected StartRequest")
}

func (unimplementedCoreStub) CancelRequest(context.Context, coreclient.ActionInput) error {
	panic("unexpected CancelRequest")
}

func (unimplementedCoreStub) CompleteRequest(context.Context, coreclient.ActionInput) error {
	panic("unexpected CompleteRequest")
}

type fixtureAccessVerifier struct {
	userID string
}

func (v fixtureAccessVerifier) VerifyAccessToken(_ context.Context, raw string) (requestcontext.Identity, error) {
	if raw != "fixture-access-token" {
		return requestcontext.Identity{}, auth.ErrInvalidToken
	}

	return requestcontext.Identity{UserID: v.userID}, nil
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
			name = "missing_authorization"
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
					if identity, ok := requestcontext.IdentityFrom(ctx); !ok || identity.UserID != userID {
						t.Errorf("Core identity = %q, present = %v, want %q", identity.UserID, ok, userID)
					}
					id := entityID
					return coreclient.ActivityRequestDetails{ID: &id}, nil
				},
			}

			listener := bufconn.Listen(1024 * 1024)
			server := grpc.NewServer(grpc.UnaryInterceptor(
				AuthInterceptor(
					fixtureAccessVerifier{userID: userID},
					func() string { return requestID },
				),
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
			incoming := metadata.Pairs(
				"x-request-id", "client-supplied-id",
				"x-user-id", userID,
			)
			if authenticated {
				incoming.Append("authorization", "Bearer fixture-access-token")
			}
			ctx = metadata.NewOutgoingContext(ctx, incoming)
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
