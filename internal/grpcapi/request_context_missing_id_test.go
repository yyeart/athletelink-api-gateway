package grpcapi

import (
	"context"
	"testing"

	gatewayv1 "gitlab.com/team-anonyms/athelete-link/api-gateway/api/gen/athletelink/gateway/v1"
	"gitlab.com/team-anonyms/athelete-link/api-gateway/internal/coreclient"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestGetRequestDetailsWithoutInternalRequestID(t *testing.T) {
	core := &correlationCoreStub{
		get: func(context.Context, coreclient.GetRequestDetailsInput) (coreclient.ActivityRequestDetails, error) {
			t.Error("Core called without an internal request ID")
			return coreclient.ActivityRequestDetails{}, nil
		},
	}
	id := correlationEntityID
	_, err := New(core).GetRequestDetails(
		context.Background(),
		&gatewayv1.GetRequestDetailsRequest{RequestId: &id},
	)
	st := status.Convert(err)
	if st.Code() != codes.Internal {
		t.Fatalf("status = %s, want INTERNAL; error = %v", st.Code(), err)
	}
	details := st.Details()
	if len(details) != 1 {
		t.Fatalf("details = %v, want one GatewayErrorDetail", details)
	}
	detail, ok := details[0].(*gatewayv1.GatewayErrorDetail)
	if !ok {
		t.Fatalf("detail type = %T, want GatewayErrorDetail", details[0])
	}
	if detail.GetCode() != "INTERNAL_ERROR" || detail.GetRequestId() != "" {
		t.Errorf("detail = %v, want INTERNAL_ERROR with empty request ID", detail)
	}
	if calls := core.calls.Load(); calls != 0 {
		t.Errorf("Core calls = %d, want 0", calls)
	}
}
