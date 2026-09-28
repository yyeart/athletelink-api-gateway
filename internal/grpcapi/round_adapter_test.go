package grpcapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	gatewayv1 "gitlab.com/team-anonyms/athelete-link/api-gateway/api/gen/athletelink/gateway/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func roundResultJSON(round int) string {
	return `{"id":"` + rpcEntityID + `","requestId":"` + rpcEntityID +
		`","roundNumber":` + strconv.Itoa(round) + `,"winners":["` + rpcUserID +
		`"],"losers":[],"recordedBy":"` + rpcUserID +
		`","createdAt":"2026-09-28T12:00:00Z","futureField":true}`
}

func TestRecordRoundResultAdapter(t *testing.T) {
	const target = "a/b%c"
	client := newRequestAdapterClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.EscapedPath() != "/requests/a%2Fb%25c/rounds/2/result" {
			t.Errorf("Core request = %s %s", r.Method, r.URL.EscapedPath())
		}
		checkAdapterHeaders(t, r, rpcUserID)
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q", got)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		want := map[string]any{"winners": []any{rpcUserID}, "losers": []any{}}
		if !reflect.DeepEqual(body, want) {
			t.Errorf("Core JSON = %#v, want %#v", body, want)
		}
		w.Header().Set("X-Request-Id", "upstream-must-not-override")
		w.Header().Set("Set-Cookie", "must-not-leak=1")
		w.WriteHeader(http.StatusCreated)
		writeStubJSON(t, w, roundResultJSON(2))
	})
	id, round := target, int32(2)
	var headers metadata.MD
	response, err := client.RecordRoundResult(requestAdapterContext(t),
		&gatewayv1.RecordRoundResultRequest{
			RequestId: &id, RoundNumber: &round,
			Winners: &gatewayv1.UuidList{Values: []string{rpcUserID}},
			Losers:  &gatewayv1.UuidList{},
		}, grpc.Header(&headers))
	if err != nil {
		t.Fatal(err)
	}
	result := response.GetResult()
	wantID, wantRecordedBy := rpcEntityID, rpcUserID
	want := &gatewayv1.RoundResult{
		Id: &wantID, RequestId: &wantID, RoundNumber: &round,
		Winners: &gatewayv1.UuidList{Values: []string{rpcUserID}},
		Losers:  &gatewayv1.UuidList{}, RecordedBy: &wantRecordedBy,
		CreatedAt: timestamppb.New(rpcClock),
	}
	if !proto.Equal(result, want) {
		t.Errorf("round result = %v, want %v", result, want)
	}
	if got := headers.Get("x-request-id"); len(got) != 1 || got[0] != rpcRequestID {
		t.Errorf("response request ID = %v", got)
	}
	if got := headers.Get("set-cookie"); len(got) != 0 {
		t.Errorf("upstream cookie leaked: %v", got)
	}
}

func TestRecordRoundResultOptionalLists(t *testing.T) {
	tests := []struct {
		name    string
		winners *gatewayv1.UuidList
		losers  *gatewayv1.UuidList
		want    map[string]any
	}{
		{name: "both omitted", want: map[string]any{}},
		{name: "empty winners", winners: &gatewayv1.UuidList{}, want: map[string]any{"winners": []any{}}},
		{name: "empty losers", losers: &gatewayv1.UuidList{}, want: map[string]any{"losers": []any{}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := newRequestAdapterClient(t, func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(body, tc.want) {
					t.Errorf("Core JSON = %#v, want %#v", body, tc.want)
				}
				w.WriteHeader(http.StatusCreated)
				writeStubJSON(t, w, `{}`)
			})
			id, round := rpcEntityID, int32(0)
			_, err := client.RecordRoundResult(requestAdapterContext(t),
				&gatewayv1.RecordRoundResultRequest{
					RequestId: &id, RoundNumber: &round,
					Winners: tc.winners, Losers: tc.losers,
				})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestGetRoundResultsAdapter(t *testing.T) {
	tests := []struct {
		name string
		body string
		want int
	}{
		{name: "ordered results", body: `[` + roundResultJSON(2) +
			`,{"roundNumber":1,"winners":[],"losers":["` + rpcUserID + `"]}]`, want: 2},
		{name: "empty results", body: `[]`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := newRequestAdapterClient(t, func(w http.ResponseWriter, r *http.Request) {
				checkGetRoundResultsCoreRequest(t, r)
				writeStubJSON(t, w, tc.body)
			})
			id := rpcEntityID
			response, err := client.GetRoundResults(requestAdapterContext(t),
				&gatewayv1.GetRoundResultsRequest{RequestId: &id})
			if err != nil {
				t.Fatal(err)
			}
			if got := len(response.GetResults()); got != tc.want {
				t.Fatalf("result count = %d, want %d", got, tc.want)
			}
			if tc.want == 0 {
				return
			}
			if want := expectedRoundResults(); !proto.Equal(response, want) {
				t.Errorf("round results = %v, want %v", response, want)
			}
		})
	}
}

func checkGetRoundResultsCoreRequest(t *testing.T, r *http.Request) {
	t.Helper()
	if r.Method != http.MethodGet || r.URL.EscapedPath() != "/requests/"+rpcEntityID+"/rounds" {
		t.Errorf("Core request = %s %s", r.Method, r.URL.EscapedPath())
	}
	checkAdapterHeaders(t, r, "")
	if r.URL.RawQuery != "" || r.Header.Get("Content-Type") != "" {
		t.Errorf("unexpected query or content type: %q %q", r.URL.RawQuery, r.Header.Get("Content-Type"))
	}
	body, err := io.ReadAll(r.Body)
	if err != nil || len(body) != 0 {
		t.Errorf("GET body = %q; err = %v", body, err)
	}
}

func expectedRoundResults() *gatewayv1.GetRoundResultsResponse {
	firstRound, secondRound := int32(2), int32(1)
	firstID, firstRecorder := rpcEntityID, rpcUserID
	return &gatewayv1.GetRoundResultsResponse{Results: []*gatewayv1.RoundResult{
		{
			Id: &firstID, RequestId: &firstID, RoundNumber: &firstRound,
			Winners: &gatewayv1.UuidList{Values: []string{rpcUserID}},
			Losers:  &gatewayv1.UuidList{}, RecordedBy: &firstRecorder,
			CreatedAt: timestamppb.New(rpcClock),
		},
		{RoundNumber: &secondRound, Winners: &gatewayv1.UuidList{},
			Losers: &gatewayv1.UuidList{Values: []string{rpcUserID}}},
	}}
}

func TestRoundResultsBusinessErrors(t *testing.T) {
	for _, tc := range []struct {
		name     string
		statuses []int
		call     func(context.Context, gatewayv1.CoreServiceClient, ...grpc.CallOption) error
	}{
		{"record", []int{400, 403, 404, 409}, func(ctx context.Context, client gatewayv1.CoreServiceClient, opts ...grpc.CallOption) error {
			id, round := rpcEntityID, int32(1)
			_, err := client.RecordRoundResult(ctx, &gatewayv1.RecordRoundResultRequest{RequestId: &id, RoundNumber: &round}, opts...)
			return err
		}},
		{"get", []int{400, 404}, func(ctx context.Context, client gatewayv1.CoreServiceClient, opts ...grpc.CallOption) error {
			id := rpcEntityID
			_, err := client.GetRoundResults(ctx, &gatewayv1.GetRoundResultsRequest{RequestId: &id}, opts...)
			return err
		}},
	} {
		for _, code := range tc.statuses {
			t.Run(tc.name+"/"+http.StatusText(code), func(t *testing.T) {
				client := newRequestAdapterClient(t, func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(code)
					writeStubJSON(t, w, `{"code":"PRIVATE","message":"private message","details":{"private":"value"}}`)
				})
				var headers metadata.MD
				checkParticipantBusinessError(t, tc.call(requestAdapterContext(t), client, grpc.Header(&headers)), code, headers)
			})
		}
	}
}

func TestRoundResultsRejectMalformedCoreResponse(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"second JSON value", `[] {}`},
		{"null list element", `[{"winners":[null]}]`},
		{"invalid timestamp", `[{"createdAt":"not-a-date"}]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := newRequestAdapterClient(t, func(w http.ResponseWriter, _ *http.Request) {
				writeStubJSON(t, w, tc.body)
			})
			id := rpcEntityID
			_, err := client.GetRoundResults(requestAdapterContext(t), &gatewayv1.GetRoundResultsRequest{RequestId: &id})
			if got := status.Code(err); got != codes.Internal {
				t.Fatalf("gRPC status = %v, want INTERNAL; err = %v", got, err)
			}
			details := status.Convert(err).Details()
			if len(details) != 1 {
				t.Fatalf("details = %v", details)
			}
			detail, ok := details[0].(*gatewayv1.GatewayErrorDetail)
			if !ok || detail.GetCode() != "UPSTREAM_CONTRACT_VIOLATION" {
				t.Errorf("detail = %v, want UPSTREAM_CONTRACT_VIOLATION", details[0])
			}
		})
	}
}

func TestRoundResultsRejectUnexpectedCoreStatus(t *testing.T) {
	for _, tc := range []struct {
		name string
		code int
		call func(context.Context, gatewayv1.CoreServiceClient) error
	}{
		{"record returned 200", http.StatusOK, func(ctx context.Context, client gatewayv1.CoreServiceClient) error {
			id, round := rpcEntityID, int32(1)
			_, err := client.RecordRoundResult(ctx, &gatewayv1.RecordRoundResultRequest{RequestId: &id, RoundNumber: &round})
			return err
		}},
		{"get returned 201", http.StatusCreated, func(ctx context.Context, client gatewayv1.CoreServiceClient) error {
			id := rpcEntityID
			_, err := client.GetRoundResults(ctx, &gatewayv1.GetRoundResultsRequest{RequestId: &id})
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := newRequestAdapterClient(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.code)
				writeStubJSON(t, w, `[]`)
			})
			err := tc.call(requestAdapterContext(t), client)
			if got := status.Code(err); got != codes.Internal {
				t.Errorf("gRPC status = %v, want INTERNAL; err = %v", got, err)
			}
		})
	}
}

func TestRoundResultsMissingInputsDoNotCallCore(t *testing.T) {
	var calls atomic.Int32
	client := newRequestAdapterClient(t, func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusOK)
	})
	id, round := rpcEntityID, int32(0)
	cases := []struct {
		name string
		call func() error
	}{
		{"record missing ID", func() error {
			_, err := client.RecordRoundResult(requestAdapterContext(t), &gatewayv1.RecordRoundResultRequest{RoundNumber: &round})
			return err
		}},
		{"record missing round", func() error {
			_, err := client.RecordRoundResult(requestAdapterContext(t), &gatewayv1.RecordRoundResultRequest{RequestId: &id})
			return err
		}},
		{"get missing ID", func() error {
			_, err := client.GetRoundResults(requestAdapterContext(t), &gatewayv1.GetRoundResultsRequest{})
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := status.Code(tc.call()); got != codes.InvalidArgument {
				t.Errorf("gRPC status = %v, want INVALID_ARGUMENT", got)
			}
		})
	}
	if got := calls.Load(); got != 0 {
		t.Errorf("Core calls = %d, want 0", got)
	}
}

func TestRoundResultsRejectMissingAuthentication(t *testing.T) {
	var calls atomic.Int32
	client := newRequestAdapterClient(t, func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusOK)
	})
	id, round := rpcEntityID, int32(1)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, recordErr := client.RecordRoundResult(ctx, &gatewayv1.RecordRoundResultRequest{RequestId: &id, RoundNumber: &round})
	_, getErr := client.GetRoundResults(ctx, &gatewayv1.GetRoundResultsRequest{RequestId: &id})
	for _, err := range []error{recordErr, getErr} {
		if status.Code(err) != codes.Unauthenticated || strings.Contains(err.Error(), "fixture-access-token") {
			t.Errorf("unauthenticated call error = %v", err)
		}
	}
	if got := calls.Load(); got != 0 {
		t.Errorf("Core calls = %d, want 0", got)
	}
}
