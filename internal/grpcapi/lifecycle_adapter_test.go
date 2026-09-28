package grpcapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	gatewayv1 "gitlab.com/team-anonyms/athelete-link/api-gateway/api/gen/athletelink/gateway/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type lifecycleCase struct {
	name    string
	path    string
	success int
	call    func(context.Context, gatewayv1.CoreServiceClient, string, ...grpc.CallOption) error
}

func lifecycleCases() []lifecycleCase {
	return []lifecycleCase{
		{
			name: "start", path: "/requests/" + rpcEntityID + "/start", success: http.StatusNoContent,
			call: func(ctx context.Context, c gatewayv1.CoreServiceClient, id string, opts ...grpc.CallOption) error {
				_, err := c.StartRequest(ctx, &gatewayv1.StartRequestRequest{RequestId: &id}, opts...)
				return err
			},
		},
		{
			name: "cancel", path: "/requests/" + rpcEntityID + "/cancel", success: http.StatusOK,
			call: func(ctx context.Context, c gatewayv1.CoreServiceClient, id string, opts ...grpc.CallOption) error {
				_, err := c.CancelRequest(ctx, &gatewayv1.CancelRequestRequest{RequestId: &id}, opts...)
				return err
			},
		},
		{
			name: "complete", path: "/requests/" + rpcEntityID + "/complete", success: http.StatusNoContent,
			call: func(ctx context.Context, c gatewayv1.CoreServiceClient, id string, opts ...grpc.CallOption) error {
				_, err := c.CompleteRequest(ctx, &gatewayv1.CompleteRequestRequest{RequestId: &id}, opts...)
				return err
			},
		},
	}
}

func TestLifecycleActionsSuccess(t *testing.T) {
	for _, tc := range lifecycleCases() {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			client := newRequestAdapterClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				checkParticipantRequest(t, r, tc.path)
				w.Header().Set("X-Request-Id", "upstream-must-not-override")
				w.Header().Set("Set-Cookie", "must-not-leak=1")
				w.WriteHeader(tc.success)
			})
			var headers metadata.MD
			if err := tc.call(requestAdapterContext(t), client, rpcEntityID, grpc.Header(&headers)); err != nil {
				t.Fatal(err)
			}
			if got := calls.Load(); got != 1 {
				t.Errorf("Core calls = %d, want 1", got)
			}
			if got := headers.Get("x-request-id"); len(got) != 1 || got[0] != rpcRequestID {
				t.Errorf("response request ID = %v, want Gateway ID", got)
			}
			if got := headers.Get("set-cookie"); len(got) != 0 {
				t.Errorf("upstream cookie leaked: %v", got)
			}
		})
	}
}

func TestLifecycleActionsBusinessErrors(t *testing.T) {
	for _, tc := range lifecycleCases() {
		for _, httpStatus := range []int{400, 403, 404, 409} {
			t.Run(tc.name+"/"+http.StatusText(httpStatus), func(t *testing.T) {
				var calls atomic.Int32
				client := newRequestAdapterClient(t, func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					checkParticipantRequest(t, r, tc.path)
					w.WriteHeader(httpStatus)
					writeStubJSON(t, w, `{"code":"PRIVATE","message":"private message",`+
						`"details":{"private":"value"},"futureField":true}`)
				})
				var headers metadata.MD
				err := tc.call(requestAdapterContext(t), client, rpcEntityID, grpc.Header(&headers))
				checkParticipantBusinessError(t, err, httpStatus, headers)
				if got := calls.Load(); got != 1 {
					t.Errorf("Core calls = %d, want 1", got)
				}
			})
		}
	}
}

func TestLifecycleActionsRejectMissingID(t *testing.T) {
	for _, tc := range lifecycleCases() {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			client := newRequestAdapterClient(t, func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.WriteHeader(tc.success)
			})
			err := tc.call(requestAdapterContext(t), client, "")
			if got := status.Code(err); got != codes.InvalidArgument {
				t.Errorf("gRPC status = %v, want INVALID_ARGUMENT; err = %v", got, err)
			}
			if got := calls.Load(); got != 0 {
				t.Errorf("Core calls = %d, want 0", got)
			}
		})
	}
}

func TestLifecycleActionsRejectMissingAuthorization(t *testing.T) {
	for _, tc := range lifecycleCases() {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			client := newRequestAdapterClient(t, func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.WriteHeader(tc.success)
			})
			ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(
				"x-user-id", rpcUserID,
				"x-request-id", "forged-request-id",
			))
			err := tc.call(ctx, client, rpcEntityID)
			if got := status.Code(err); got != codes.Unauthenticated {
				t.Errorf("gRPC status = %v, want UNAUTHENTICATED; err = %v", got, err)
			}
			if got := calls.Load(); got != 0 {
				t.Errorf("Core calls = %d, want 0", got)
			}
		})
	}
}

func TestLifecycleActionsRejectUnexpectedSuccessStatus(t *testing.T) {
	for _, tc := range lifecycleCases() {
		t.Run(tc.name, func(t *testing.T) {
			client := newRequestAdapterClient(t, func(w http.ResponseWriter, _ *http.Request) {
				if tc.success == http.StatusOK {
					w.WriteHeader(http.StatusNoContent)
				} else {
					w.WriteHeader(http.StatusOK)
				}
			})
			err := tc.call(requestAdapterContext(t), client, rpcEntityID)
			if got := status.Code(err); got != codes.Internal {
				t.Fatalf("gRPC status = %v, want INTERNAL; err = %v", got, err)
			}
			if details := status.Convert(err).Details(); len(details) != 1 {
				t.Fatalf("error details = %v, want one GatewayErrorDetail", details)
			} else if detail, ok := details[0].(*gatewayv1.GatewayErrorDetail); !ok ||
				detail.GetCode() != "UPSTREAM_CONTRACT_VIOLATION" {
				t.Errorf("error detail = %v, want UPSTREAM_CONTRACT_VIOLATION", details[0])
			}
		})
	}
}

func TestLifecycleActionsRejectMalformedCoreError(t *testing.T) {
	for _, tc := range lifecycleCases() {
		t.Run(tc.name, func(t *testing.T) {
			client := newRequestAdapterClient(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusConflict)
				writeStubJSON(t, w, `{"code":null,"message":"private"}`)
			})
			err := tc.call(requestAdapterContext(t), client, rpcEntityID)
			if got := status.Code(err); got != codes.Internal {
				t.Fatalf("gRPC status = %v, want INTERNAL; err = %v", got, err)
			}
			if strings.Contains(err.Error(), "private") {
				t.Errorf("Core error leaked: %v", err)
			}
		})
	}
}

func TestLifecycleActionsPropagateCancellation(t *testing.T) {
	for _, tc := range lifecycleCases() {
		t.Run(tc.name, func(t *testing.T) {
			started := make(chan struct{}, 1)
			upstreamCanceled := make(chan error, 1)
			client := newRequestAdapterClient(t, func(_ http.ResponseWriter, r *http.Request) {
				checkParticipantRequest(t, r, tc.path)
				started <- struct{}{}
				<-r.Context().Done()
				upstreamCanceled <- r.Context().Err()
			})
			ctx, cancel := context.WithCancel(requestAdapterContext(t))
			defer cancel()
			result := make(chan error, 1)
			go func() { result <- tc.call(ctx, client, rpcEntityID) }()

			select {
			case <-started:
			case <-time.After(3 * time.Second):
				t.Fatal("Core request did not start")
			}
			cancel()
			checkCancellationResults(t, result, upstreamCanceled)
		})
	}
}

func checkCancellationResults(t *testing.T, result, upstreamCanceled <-chan error) {
	t.Helper()
	select {
	case err := <-result:
		if got := status.Code(err); got != codes.Canceled {
			t.Errorf("gRPC status = %v, want CANCELLED; err = %v", got, err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("gRPC call did not stop after cancellation")
	}
	select {
	case err := <-upstreamCanceled:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Core request context = %v, want cancellation", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Core request was not cancelled")
	}
}
