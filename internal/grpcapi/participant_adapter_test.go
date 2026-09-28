package grpcapi

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	gatewayv1 "gitlab.com/team-anonyms/athelete-link/api-gateway/api/gen/athletelink/gateway/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const participantTargetID = "a/b%c"

type participantActionCase struct {
	name    string
	path    string
	success int
	errors  []int
	call    func(context.Context, gatewayv1.CoreServiceClient, string, ...grpc.CallOption) error
}

func participantActionCases() []participantActionCase {
	return []participantActionCase{
		{
			name: "open registration", path: "/requests/" + rpcEntityID + "/registration/open",
			success: http.StatusNoContent, errors: []int{400, 403, 404, 409},
			call: func(ctx context.Context, client gatewayv1.CoreServiceClient, id string, opts ...grpc.CallOption) error {
				_, err := client.OpenRegistration(ctx, &gatewayv1.OpenRegistrationRequest{RequestId: &id}, opts...)
				return err
			},
		},
		{
			name: "close registration", path: "/requests/" + rpcEntityID + "/registration/close",
			success: http.StatusNoContent, errors: []int{400, 403, 404, 409},
			call: func(ctx context.Context, client gatewayv1.CoreServiceClient, id string, opts ...grpc.CallOption) error {
				_, err := client.CloseRegistration(ctx, &gatewayv1.CloseRegistrationRequest{RequestId: &id}, opts...)
				return err
			},
		},
		{
			name: "leave request", path: "/requests/" + rpcEntityID + "/leave",
			success: http.StatusOK, errors: []int{400, 404, 409},
			call: func(ctx context.Context, client gatewayv1.CoreServiceClient, id string, opts ...grpc.CallOption) error {
				_, err := client.LeaveRequest(ctx, &gatewayv1.LeaveRequestRequest{RequestId: &id}, opts...)
				return err
			},
		},
		{
			name: "join request", path: "/requests/" + rpcEntityID + "/join",
			success: http.StatusOK, errors: []int{400, 404, 409},
			call: func(ctx context.Context, client gatewayv1.CoreServiceClient, id string, opts ...grpc.CallOption) error {
				_, err := client.JoinRequest(ctx, &gatewayv1.JoinRequestRequest{RequestId: &id}, opts...)
				return err
			},
		},
		{
			name: "kick participant", path: "/requests/" + rpcEntityID + "/kick/a%2Fb%25c",
			success: http.StatusOK, errors: []int{400, 403, 404, 409},
			call: func(ctx context.Context, client gatewayv1.CoreServiceClient, id string, opts ...grpc.CallOption) error {
				target := participantTargetID
				_, err := client.KickParticipant(ctx, &gatewayv1.KickParticipantRequest{
					RequestId: &id, TargetUserId: &target,
				}, opts...)
				return err
			},
		},
	}
}

func checkParticipantRequest(t *testing.T, r *http.Request, path string) {
	t.Helper()
	if r.Method != http.MethodPost || r.URL.EscapedPath() != path {
		t.Errorf("Core request = %s %s, want POST %s", r.Method, r.URL.EscapedPath(), path)
	}
	checkAdapterHeaders(t, r, rpcUserID)
	if r.URL.RawQuery != "" {
		t.Errorf("unexpected Core query: %q", r.URL.RawQuery)
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Errorf("read Core request body: %v", err)
	}
	if len(body) != 0 {
		t.Errorf("unexpected Core request body: %q", body)
	}
	if got := r.Header.Get("Content-Type"); got != "" {
		t.Errorf("unexpected Core Content-Type: %q", got)
	}
}

func TestParticipantActionsSuccess(t *testing.T) {
	for _, tc := range participantActionCases() {
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

func TestParticipantActionsBusinessErrors(t *testing.T) {
	for _, tc := range participantActionCases() {
		for _, httpStatus := range tc.errors {
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

func checkParticipantBusinessError(t *testing.T, err error, httpStatus int, headers metadata.MD) {
	t.Helper()
	expected, ok := map[int]struct {
		status codes.Code
		code   string
	}{
		http.StatusBadRequest: {codes.InvalidArgument, "INVALID_ARGUMENT"},
		http.StatusForbidden:  {codes.PermissionDenied, "PERMISSION_DENIED"},
		http.StatusNotFound:   {codes.NotFound, "NOT_FOUND"},
		http.StatusConflict:   {codes.FailedPrecondition, "FAILED_PRECONDITION"},
	}[httpStatus]
	if !ok {
		t.Fatalf("test has unsupported HTTP status %d", httpStatus)
	}
	if got := status.Code(err); got != expected.status {
		t.Fatalf("gRPC status = %v, want %v; err = %v", got, expected.status, err)
	}
	if got := headers.Get("x-request-id"); len(got) != 1 || got[0] != rpcRequestID {
		t.Errorf("response request ID = %v, want Gateway ID", got)
	}
	details := status.Convert(err).Details()
	if len(details) != 1 {
		t.Fatalf("error details = %v, want one CoreErrorDetail", details)
	}
	detail, ok := details[0].(*gatewayv1.CoreErrorDetail)
	if !ok {
		t.Fatalf("detail type = %T, want CoreErrorDetail", details[0])
	}
	if int(detail.GetHttpStatus()) != httpStatus || detail.GetCode() != expected.code ||
		detail.GetRequestId() != rpcRequestID || detail.Message != nil || detail.Details != nil {
		t.Errorf("public error detail = %v", detail)
	}
	if strings.Contains(err.Error(), "private") {
		t.Errorf("Core error message leaked: %v", err)
	}
}

func TestParticipantActionsMissingRequestID(t *testing.T) {
	for _, tc := range participantActionCases() {
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

func TestKickParticipantMissingTarget(t *testing.T) {
	var calls atomic.Int32
	client := newRequestAdapterClient(t, func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusOK)
	})
	id := rpcEntityID
	_, err := client.KickParticipant(requestAdapterContext(t),
		&gatewayv1.KickParticipantRequest{RequestId: &id})
	if got := status.Code(err); got != codes.InvalidArgument {
		t.Errorf("gRPC status = %v, want INVALID_ARGUMENT; err = %v", got, err)
	}
	if got := calls.Load(); got != 0 {
		t.Errorf("Core calls = %d, want 0", got)
	}
}
