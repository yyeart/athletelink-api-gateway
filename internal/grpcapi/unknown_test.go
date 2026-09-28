package grpcapi

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	gatewayv1 "gitlab.com/team-anonyms/athelete-link/api-gateway/api/gen/athletelink/gateway/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/emptypb"
)

func newUnknownRPCTestClient(t *testing.T, requestID string) (*grpc.ClientConn, *correlationCoreStub) {
	t.Helper()
	core := &correlationCoreStub{}
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer(UnknownServiceHandler(func() string { return requestID }))
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
		"passthrough:///unknown-rpc-test",
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
	return conn, core
}

func checkUnknownRPC(t *testing.T, conn *grpc.ClientConn, method, requestID string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs(
		"x-request-id", "client-supplied-id",
	))

	var headers metadata.MD
	err := conn.Invoke(ctx, method, &emptypb.Empty{}, &emptypb.Empty{},
		grpc.Header(&headers))
	if status.Code(err) != codes.Unimplemented {
		t.Fatalf("status = %v, want UNIMPLEMENTED; error = %v", status.Code(err), err)
	}
	if values := headers.Get("x-request-id"); len(values) != 1 || values[0] != requestID {
		t.Errorf("response x-request-id = %v, want [%s]", values, requestID)
	}

	details := status.Convert(err).Details()
	if len(details) != 1 {
		t.Fatalf("error details = %v, want one GatewayErrorDetail", details)
	}
	detail, ok := details[0].(*gatewayv1.GatewayErrorDetail)
	if !ok {
		t.Fatalf("detail type = %T, want GatewayErrorDetail", details[0])
	}
	if detail.GetCode() != "METHOD_NOT_IMPLEMENTED" || detail.GetRequestId() != requestID {
		t.Errorf("error detail = %v, want METHOD_NOT_IMPLEMENTED and request ID %s", detail, requestID)
	}
}

func TestUnknownRPC(t *testing.T) {
	const requestID = "123e4567-e89b-12d3-a456-426614174001"
	conn, core := newUnknownRPCTestClient(t, requestID)

	for _, method := range []string{
		"/athletelink.gateway.v1.CoreService/NoSuchMethod",
		"/athletelink.gateway.v1.NoSuchService/NoSuchMethod",
	} {
		t.Run(method, func(t *testing.T) {
			checkUnknownRPC(t, conn, method, requestID)
		})
	}

	if calls := core.calls.Load(); calls != 0 {
		t.Errorf("Core calls = %d, want 0", calls)
	}
}
