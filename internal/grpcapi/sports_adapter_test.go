package grpcapi

import (
	"context"
	"io"
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

func checkSportsCoreRequest(t *testing.T, r *http.Request) {
	t.Helper()
	if r.Method != http.MethodGet || r.URL.EscapedPath() != "/sports" {
		t.Errorf("Core request = %s %s, want GET /sports", r.Method, r.URL.EscapedPath())
	}
	if r.URL.RawQuery != "" || r.Header.Get("Content-Type") != "" {
		t.Errorf("unexpected query/content type: %q / %q", r.URL.RawQuery, r.Header.Get("Content-Type"))
	}
	checkAdapterHeaders(t, r, "")
	if got := r.Header.Get("Cookie"); got != "" {
		t.Errorf("Core received cookie: %q", got)
	}
	body, err := io.ReadAll(r.Body)
	if err != nil || len(body) != 0 {
		t.Errorf("GET body = %q, error = %v", body, err)
	}
}

type sportSuccessCase struct {
	name       string
	body       string
	wantLen    int
	wantIcon   *string
	wantName   *string
	wantZeroID bool
	wantMin    *int32
	wantMax    *int32
}

func TestGetAllSportsAdapter(t *testing.T) {
	tests := []sportSuccessCase{
		{
			name:    "fields, null icon, and unknown property",
			body:    `[{"id":0,"name":"Football","minPlayers":2,"maxPlayers":22,"iconUrl":null,"futureField":true}]`,
			wantLen: 1, wantName: ptr("Football"), wantZeroID: true,
			wantMin: int32ptr(2), wantMax: int32ptr(22),
		},
		{
			name: "explicit empty icon",
			body: `[{"iconUrl":""}]`, wantLen: 1, wantIcon: ptr(""),
		},
		{
			name: "absent fields",
			body: `[{}]`, wantLen: 1,
		},
		{
			name: "empty list",
			body: `[]`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) { runSportsSuccessCase(t, tc) })
	}
}

func runSportsSuccessCase(t *testing.T, tc sportSuccessCase) {
	t.Helper()
	client := newRequestAdapterClient(t, func(w http.ResponseWriter, r *http.Request) {
		checkSportsCoreRequest(t, r)
		w.Header().Set("X-Request-Id", "upstream-must-not-override")
		w.Header().Set("Set-Cookie", "must-not-leak=1")
		writeStubJSON(t, w, tc.body)
	})
	var headers metadata.MD
	response, err := client.GetAllSports(requestAdapterContext(t),
		&gatewayv1.GetAllSportsRequest{}, grpc.Header(&headers))
	if err != nil {
		t.Fatal(err)
	}
	if got := len(response.GetSports()); got != tc.wantLen {
		t.Fatalf("sports count = %d, want %d", got, tc.wantLen)
	}
	assertSportsHeaders(t, headers)
	if tc.wantLen != 0 {
		assertSportFields(t, response.GetSports()[0], tc)
	}
}

func assertSportsHeaders(t *testing.T, headers metadata.MD) {
	t.Helper()
	if got := headers.Get("x-request-id"); len(got) != 1 || got[0] != rpcRequestID {
		t.Errorf("response request ID = %v, want [%s]", got, rpcRequestID)
	}
	if got := headers.Get("set-cookie"); len(got) != 0 {
		t.Errorf("upstream cookie leaked: %v", got)
	}
}

func assertSportFields(t *testing.T, sport *gatewayv1.Sport, tc sportSuccessCase) {
	t.Helper()
	if (sport.Id != nil) != tc.wantZeroID || sport.GetId() != 0 {
		t.Errorf("sport ID presence/value = %v/%d", sport.Id != nil, sport.GetId())
	}
	if (sport.Name == nil) != (tc.wantName == nil) || sport.GetName() != value(tc.wantName) {
		t.Errorf("sport name = %v, want %v", sport.Name, tc.wantName)
	}
	if (sport.IconUrl == nil) != (tc.wantIcon == nil) || sport.GetIconUrl() != value(tc.wantIcon) {
		t.Errorf("sport icon = %v, want %v", sport.IconUrl, tc.wantIcon)
	}
	if (sport.MinPlayers == nil) != (tc.wantMin == nil) || sport.GetMinPlayers() != int32value(tc.wantMin) {
		t.Errorf("minimum players = %v, want %v", sport.MinPlayers, tc.wantMin)
	}
	if (sport.MaxPlayers == nil) != (tc.wantMax == nil) || sport.GetMaxPlayers() != int32value(tc.wantMax) {
		t.Errorf("maximum players = %v, want %v", sport.MaxPlayers, tc.wantMax)
	}
}

type sportFailureCase struct {
	name       string
	statusCode int
	body       string
	wantCode   codes.Code
	wantDetail string
}

func TestGetAllSportsAdapterFailures(t *testing.T) {
	tests := []sportFailureCase{
		{"null array", http.StatusOK, `null`, codes.Internal, "UPSTREAM_CONTRACT_VIOLATION"},
		{"null item", http.StatusOK, `[null]`, codes.Internal, "UPSTREAM_CONTRACT_VIOLATION"},
		{"wrong integer type", http.StatusOK, `[{"minPlayers":"2"}]`, codes.Internal, "UPSTREAM_CONTRACT_VIOLATION"},
		{"null nonnullable field", http.StatusOK, `[{"name":null}]`, codes.Internal, "UPSTREAM_CONTRACT_VIOLATION"},
		{"wrong icon type", http.StatusOK, `[{"iconUrl":2}]`, codes.Internal, "UPSTREAM_CONTRACT_VIOLATION"},
		{"malformed JSON", http.StatusOK, `[{`, codes.Internal, "UPSTREAM_CONTRACT_VIOLATION"},
		{"duplicate field", http.StatusOK, `[{"id":1,"id":2}]`, codes.Internal, "UPSTREAM_CONTRACT_VIOLATION"},
		{"extra JSON value", http.StatusOK, `[] []`, codes.Internal, "UPSTREAM_CONTRACT_VIOLATION"},
		{"unexpected 404", http.StatusNotFound, `{}`, codes.Internal, "UPSTREAM_CONTRACT_VIOLATION"},
		{"upstream 500", http.StatusInternalServerError, `secret`, codes.Unavailable, "UPSTREAM_UNAVAILABLE"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) { runSportsFailureCase(t, tc) })
	}
}

func runSportsFailureCase(t *testing.T, tc sportFailureCase) {
	t.Helper()
	client := newRequestAdapterClient(t, func(w http.ResponseWriter, r *http.Request) {
		checkSportsCoreRequest(t, r)
		w.WriteHeader(tc.statusCode)
		writeStubJSON(t, w, tc.body)
	})
	var headers metadata.MD
	_, err := client.GetAllSports(requestAdapterContext(t),
		&gatewayv1.GetAllSportsRequest{}, grpc.Header(&headers))
	if status.Code(err) != tc.wantCode {
		t.Fatalf("status = %v, want %v; error = %v", status.Code(err), tc.wantCode, err)
	}
	if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "minPlayers") {
		t.Errorf("raw Core data leaked: %v", err)
	}
	assertSportsHeaders(t, headers)
	details := status.Convert(err).Details()
	if len(details) != 1 {
		t.Fatalf("error details = %v, want one GatewayErrorDetail", details)
	}
	detail, ok := details[0].(*gatewayv1.GatewayErrorDetail)
	if !ok || detail.GetCode() != tc.wantDetail || detail.GetRequestId() != rpcRequestID {
		t.Errorf("error detail = %v, want %s and request ID", details[0], tc.wantDetail)
	}
}

func TestGetAllSportsRequiresIdentity(t *testing.T) {
	var coreCalls atomic.Int32
	client := newRequestAdapterClient(t, func(w http.ResponseWriter, _ *http.Request) {
		coreCalls.Add(1)
		writeStubJSON(t, w, `[]`)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := client.GetAllSports(ctx, &gatewayv1.GetAllSportsRequest{})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("status = %v, want UNAUTHENTICATED; error = %v", status.Code(err), err)
	}
	if calls := coreCalls.Load(); calls != 0 {
		t.Errorf("Core calls = %d, want 0", calls)
	}
}

func ptr(value string) *string { return &value }

func int32ptr(value int32) *int32 { return &value }

func value(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func int32value(p *int32) int32 {
	if p == nil {
		return 0
	}
	return *p
}
