package grpcapi

import (
	"context"
	"errors"
	"testing"

	gatewayv1 "gitlab.com/team-anonyms/athelete-link/api-gateway/api/gen/athletelink/gateway/v1"
	"gitlab.com/team-anonyms/athelete-link/api-gateway/internal/coreclient"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type responseReadTimeout struct{}

func (responseReadTimeout) Error() string   { return "response read timed out" }
func (responseReadTimeout) Timeout() bool   { return true }
func (responseReadTimeout) Temporary() bool { return true }

func TestMapCoreResponseReadFailure(t *testing.T) {
	tests := []struct {
		name       string
		cause      error
		wantStatus codes.Code
		wantCode   string
	}{
		{"timeout", responseReadTimeout{}, codes.DeadlineExceeded, "DEADLINE_EXCEEDED"},
		{"other read failure", errors.New("connection reset"), codes.Unavailable, "UPSTREAM_UNAVAILABLE"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := mapCoreError(context.Background(), &coreclient.TransportError{Cause: tt.cause}, "request-123")
			st := status.Convert(err)
			if st.Code() != tt.wantStatus {
				t.Fatalf("status = %s, want %s", st.Code(), tt.wantStatus)
			}
			details := st.Details()
			if len(details) != 1 {
				t.Fatalf("details = %v, want one GatewayErrorDetail", details)
			}
			detail, ok := details[0].(*gatewayv1.GatewayErrorDetail)
			if !ok || detail.GetCode() != tt.wantCode || detail.GetRequestId() != "request-123" {
				t.Errorf("detail = %v, want code %s and request ID", details[0], tt.wantCode)
			}
		})
	}
}
