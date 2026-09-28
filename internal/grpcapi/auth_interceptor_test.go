package grpcapi

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
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

const (
	rpcUserID    = "123e4567-e89b-42d3-a456-426614174011"
	rpcJTI       = "123e4567-e89b-42d3-a456-426614174012"
	rpcEntityID  = "123e4567-e89b-42d3-a456-426614174013"
	rpcRequestID = "123e4567-e89b-42d3-a456-426614174014"
	rpcSecret    = "grpc-test-only-signing-key-12345678"
)

var rpcClock = time.Date(2026, time.September, 28, 12, 0, 0, 0, time.UTC)

type rpcDenylistStub struct {
	calls  atomic.Int32
	denied bool
	err    error
}

func (s *rpcDenylistStub) IsDenied(_ context.Context, _ string) (bool, error) {
	s.calls.Add(1)
	return s.denied, s.err
}

type authObservation struct {
	userID    string
	requestID string
	entityID  string
}

type authCoreStub struct {
	calls    atomic.Int32
	observed chan authObservation
}

type authRPCCase struct {
	name           string
	authorization  []string
	denied         bool
	redisErr       error
	wantStatus     codes.Code
	wantDetail     string
	wantCoreCalls  int32
	wantRedisCalls int32
}

func (s *authCoreStub) GetRequestDetails(
	ctx context.Context,
	input coreclient.GetRequestDetailsInput,
) (coreclient.ActivityRequestDetails, error) {
	s.calls.Add(1)
	identity, _ := requestcontext.IdentityFrom(ctx)
	requestID, _ := requestcontext.RequestIDFrom(ctx)
	s.observed <- authObservation{
		userID: identity.UserID, requestID: requestID, entityID: input.RequestID,
	}
	entityID := input.RequestID
	return coreclient.ActivityRequestDetails{ID: &entityID}, nil
}

func signedRPCToken(t *testing.T, tokenType string, key []byte) string {
	t.Helper()
	claims := jwt.MapClaims{
		"sub":  rpcUserID,
		"jti":  rpcJTI,
		"type": tokenType,
		"iat":  rpcClock.Add(-time.Minute).Unix(),
		"exp":  rpcClock.Add(time.Minute).Unix(),
	}
	raw, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(key)
	if err != nil {
		t.Fatalf("sign test token: %v", err)
	}
	return raw
}

func newAuthRPCClient(
	t *testing.T,
	verifier AccessVerifier,
	core Core,
) *grpc.ClientConn {
	t.Helper()
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer(grpc.UnaryInterceptor(AuthInterceptor(
		verifier, func() string { return rpcRequestID },
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
		"passthrough:///auth-interceptor-test",
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

func TestAuthInterceptorWithRealVerifier(t *testing.T) {
	valid := signedRPCToken(t, "ACCESS", []byte(rpcSecret))
	refresh := signedRPCToken(t, "REFRESH", []byte(rpcSecret))
	wrongSignature := signedRPCToken(t, "ACCESS", []byte("wrong-test-key"))
	tests := []authRPCCase{
		{name: "valid access", authorization: []string{"Bearer " + valid}, wantStatus: codes.OK,
			wantCoreCalls: 1, wantRedisCalls: 1},
		{name: "missing authorization", wantStatus: codes.Unauthenticated, wantDetail: "AUTHENTICATION_REQUIRED"},
		{name: "duplicate authorization", authorization: []string{"Bearer " + valid, "Bearer " + valid},
			wantStatus: codes.Unauthenticated, wantDetail: "AUTHENTICATION_REQUIRED"},
		{name: "wrong scheme", authorization: []string{"Basic " + valid},
			wantStatus: codes.Unauthenticated, wantDetail: "AUTHENTICATION_REQUIRED"},
		{name: "empty bearer", authorization: []string{"Bearer "},
			wantStatus: codes.Unauthenticated, wantDetail: "AUTHENTICATION_REQUIRED"},
		{name: "extra whitespace", authorization: []string{"Bearer  " + valid},
			wantStatus: codes.Unauthenticated, wantDetail: "AUTHENTICATION_REQUIRED"},
		{name: "refresh token", authorization: []string{"Bearer " + refresh},
			wantStatus: codes.Unauthenticated, wantDetail: "AUTHENTICATION_REQUIRED"},
		{name: "invalid signature", authorization: []string{"Bearer " + wrongSignature},
			wantStatus: codes.Unauthenticated, wantDetail: "AUTHENTICATION_REQUIRED"},
		{name: "denylisted token", authorization: []string{"Bearer " + valid}, denied: true,
			wantStatus: codes.Unauthenticated, wantDetail: "AUTHENTICATION_REQUIRED", wantRedisCalls: 1},
		{name: "redis unavailable", authorization: []string{"Bearer " + valid}, redisErr: errors.New("redis down"),
			wantStatus: codes.Unavailable, wantDetail: "AUTH_CHECK_UNAVAILABLE", wantRedisCalls: 1},
		{name: "redis timeout", authorization: []string{"Bearer " + valid}, redisErr: context.DeadlineExceeded,
			wantStatus: codes.Unavailable, wantDetail: "AUTH_CHECK_UNAVAILABLE", wantRedisCalls: 1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runAuthRPCCase(t, tc)
		})
	}
}

func runAuthRPCCase(t *testing.T, tc authRPCCase) {
	t.Helper()
	denylist := &rpcDenylistStub{denied: tc.denied, err: tc.redisErr}
	verifier, err := auth.NewVerifier(rpcSecret, denylist, func() time.Time { return rpcClock })
	if err != nil {
		t.Fatalf("new verifier: %v", err)
	}
	core := &authCoreStub{observed: make(chan authObservation, 1)}
	conn := newAuthRPCClient(t, verifier, core)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	md := metadata.Pairs("x-user-id", "forged-user-id", "x-request-id", "forged-request-id")
	for _, value := range tc.authorization {
		md.Append("authorization", value)
	}
	ctx = metadata.NewOutgoingContext(ctx, md)
	var headers metadata.MD
	entityID := rpcEntityID
	response, callErr := gatewayv1.NewCoreServiceClient(conn).GetRequestDetails(
		ctx, &gatewayv1.GetRequestDetailsRequest{RequestId: &entityID}, grpc.Header(&headers),
	)

	if got := status.Code(callErr); got != tc.wantStatus {
		t.Errorf("status = %v, want %v; error = %v", got, tc.wantStatus, callErr)
	}
	if values := headers.Get("x-request-id"); len(values) != 1 || values[0] != rpcRequestID {
		t.Errorf("response x-request-id = %v, want exactly [%s]", values, rpcRequestID)
	}
	if calls := core.calls.Load(); calls != tc.wantCoreCalls {
		t.Errorf("Core calls = %d, want %d", calls, tc.wantCoreCalls)
	}
	if calls := denylist.calls.Load(); calls != tc.wantRedisCalls {
		t.Errorf("Redis calls = %d, want %d", calls, tc.wantRedisCalls)
	}
	if tc.wantStatus == codes.OK {
		checkAuthenticatedResponse(t, response, core.observed)
		return
	}
	checkAuthFailure(t, callErr, headers, tc.wantDetail, tc.authorization)
}

func checkAuthenticatedResponse(
	t *testing.T,
	response *gatewayv1.GetRequestDetailsResponse,
	observed <-chan authObservation,
) {
	t.Helper()
	if response == nil || response.GetRequest().GetId() != rpcEntityID {
		t.Errorf("response = %v, want request ID %s", response, rpcEntityID)
	}
	select {
	case got := <-observed:
		if got.userID != rpcUserID || got.requestID != rpcRequestID || got.entityID != rpcEntityID {
			t.Errorf("Core received %+v; want verified user, Gateway request ID, and entity ID", got)
		}
	default:
		t.Error("Core observation missing")
	}
}

func checkAuthFailure(
	t *testing.T,
	err error,
	headers metadata.MD,
	wantCode string,
	authorization []string,
) {
	t.Helper()
	details := status.Convert(err).Details()
	if len(details) != 1 {
		t.Fatalf("error details = %v, want one GatewayErrorDetail", details)
	}
	detail, ok := details[0].(*gatewayv1.GatewayErrorDetail)
	if !ok {
		t.Fatalf("detail type = %T, want GatewayErrorDetail", details[0])
	}
	if detail.GetCode() != wantCode || detail.GetRequestId() != rpcRequestID {
		t.Errorf("error detail = %v, want %s and request ID %s", detail, wantCode, rpcRequestID)
	}
	public := err.Error() + fmt.Sprint(headers) + detail.String()
	if strings.Contains(public, rpcSecret) || strings.Contains(public, "redis down") {
		t.Error("public response exposed token, secret, or Redis error")
	}
	for _, value := range authorization {
		token := strings.TrimPrefix(value, "Bearer ")
		if token != "" && strings.Contains(public, token) {
			t.Error("public response exposed authorization value")
		}
	}
}
