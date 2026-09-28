package grpcapi

import (
	"context"
	"errors"
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

func (unimplementedCoreStub) RecordRoundResult(context.Context, coreclient.RecordRoundResultInput) (coreclient.RoundResult, error) {
	panic("unexpected RecordRoundResult")
}

func (unimplementedCoreStub) GetRoundResults(context.Context, coreclient.GetRoundResultsInput) ([]coreclient.RoundResult, error) {
	panic("unexpected GetRoundResults")
}

func (unimplementedCoreStub) GetAllSports(context.Context) ([]coreclient.Sport, error) {
	panic("unexpected GetAllSports")
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
	for _, authenticated := range []bool{true, false} {
		name := "success"
		if !authenticated {
			name = "missing_authorization"
		}
		t.Run(name, func(t *testing.T) {
			runCorrelationCase(t, authenticated)
		})
	}
}

const (
	correlationRequestID = "123e4567-e89b-12d3-a456-426614174001"
	correlationEntityID  = "123e4567-e89b-12d3-a456-426614174002"
	correlationUserID    = "123e4567-e89b-12d3-a456-426614174003"
)

func runCorrelationCase(t *testing.T, authenticated bool) {
	t.Helper()
	core := newCorrelationCore(t, authenticated)
	conn := newCorrelationClient(t, core)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	incoming := metadata.Pairs(
		"x-request-id", "client-supplied-id",
		"x-user-id", correlationUserID,
	)
	if authenticated {
		incoming.Append("authorization", "Bearer fixture-access-token")
	}
	ctx = metadata.NewOutgoingContext(ctx, incoming)
	var headers metadata.MD
	id := correlationEntityID
	response, err := gatewayv1.NewCoreServiceClient(conn).GetRequestDetails(
		ctx, &gatewayv1.GetRequestDetailsRequest{RequestId: &id}, grpc.Header(&headers),
	)
	checkCorrelationResponse(t, authenticated, core, response, err, headers)
}

func newCorrelationCore(t *testing.T, authenticated bool) *correlationCoreStub {
	t.Helper()
	return &correlationCoreStub{
		get: func(ctx context.Context, input coreclient.GetRequestDetailsInput) (coreclient.ActivityRequestDetails, error) {
			if !authenticated {
				t.Error("Core called without trusted identity")
			}
			if input.RequestID != correlationEntityID {
				t.Errorf("Core request ID = %q, want %q", input.RequestID, correlationEntityID)
			}
			if id, ok := requestcontext.RequestIDFrom(ctx); !ok || id != correlationRequestID {
				t.Errorf("Core correlation ID = %q, present = %v", id, ok)
			}
			if identity, ok := requestcontext.IdentityFrom(ctx); !ok || identity.UserID != correlationUserID {
				t.Errorf("Core identity = %q, present = %v, want %q", identity.UserID, ok, correlationUserID)
			}
			id := correlationEntityID
			return coreclient.ActivityRequestDetails{ID: &id}, nil
		},
	}
}

func newCorrelationClient(t *testing.T, core *correlationCoreStub) *grpc.ClientConn {
	t.Helper()
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer(grpc.UnaryInterceptor(AuthInterceptor(
		fixtureAccessVerifier{userID: correlationUserID},
		func() string { return correlationRequestID },
	)))
	gatewayv1.RegisterCoreServiceServer(server, New(core))
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			t.Errorf("serve gRPC: %v", err)
		}
	}()
	t.Cleanup(func() {
		server.Stop()
		if err := listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			t.Errorf("close listener: %v", err)
		}
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
	t.Cleanup(func() {
		if err := conn.Close(); err != nil {
			t.Errorf("close gRPC connection: %v", err)
		}
	})
	return conn
}

func checkCorrelationResponse(
	t *testing.T, authenticated bool, core *correlationCoreStub,
	response *gatewayv1.GetRequestDetailsResponse, err error, headers metadata.MD,
) {
	t.Helper()
	if values := headers.Get("x-request-id"); len(values) != 1 || values[0] != correlationRequestID {
		t.Errorf("response x-request-id = %v, want exactly [%s]", values, correlationRequestID)
	}
	if authenticated {
		if err != nil {
			t.Fatalf("GetRequestDetails: %v", err)
		}
		if response.GetRequest().GetId() != correlationEntityID {
			t.Errorf("response entity ID = %q, want %q", response.GetRequest().GetId(), correlationEntityID)
		}
		if calls := core.calls.Load(); calls != 1 {
			t.Errorf("Core calls = %d, want 1", calls)
		}
		return
	}
	checkMissingAuthorizationResponse(t, core, err)
}

func checkMissingAuthorizationResponse(t *testing.T, core *correlationCoreStub, err error) {
	t.Helper()
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
	if detail.GetCode() != "AUTHENTICATION_REQUIRED" || detail.GetRequestId() != correlationRequestID {
		t.Errorf("error detail = %v, want AUTHENTICATION_REQUIRED and request ID %s", detail, correlationRequestID)
	}
}
