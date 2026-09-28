package grpcapi

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	gatewayv1 "gitlab.com/team-anonyms/athelete-link/api-gateway/api/gen/athletelink/gateway/v1"
	"gitlab.com/team-anonyms/athelete-link/api-gateway/internal/coreclient"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func newRequestAdapterClient(t *testing.T, handler http.HandlerFunc) gatewayv1.CoreServiceClient {
	t.Helper()
	stub := httptest.NewServer(handler)
	t.Cleanup(stub.Close)
	baseURL, err := url.Parse(stub.URL)
	if err != nil {
		t.Fatal(err)
	}
	core, err := coreclient.New(baseURL, stub.Client())
	if err != nil {
		t.Fatal(err)
	}
	conn := newAuthRPCClient(t, fixtureAccessVerifier{userID: rpcUserID}, core)
	return gatewayv1.NewCoreServiceClient(conn)
}

func requestAdapterContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return metadata.NewOutgoingContext(ctx, metadata.Pairs(
		"authorization", "Bearer fixture-access-token",
		"x-user-id", "forged-user-id",
		"x-request-id", "forged-request-id",
	))
}

func checkAdapterHeaders(t *testing.T, r *http.Request, userID string) {
	t.Helper()
	if got := r.Header.Get("X-Request-Id"); got != rpcRequestID {
		t.Errorf("Core X-Request-Id = %q, want %q", got, rpcRequestID)
	}
	if got := r.Header.Get("X-User-Id"); got != userID {
		t.Errorf("Core X-User-Id = %q, want %q", got, userID)
	}
	if got := r.Header.Get("Authorization"); got != "" {
		t.Errorf("Core received Authorization header: %q", got)
	}
}

func writeStubJSON(t *testing.T, w http.ResponseWriter, body string) {
	t.Helper()
	if _, err := w.Write([]byte(body)); err != nil {
		t.Errorf("write Core stub response: %v", err)
	}
}

func TestRequestAdapterGetDetails(t *testing.T) {
	client := newRequestAdapterClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.EscapedPath() != "/requests/"+rpcEntityID {
			t.Errorf("Core request = %s %s", r.Method, r.URL.EscapedPath())
		}
		checkAdapterHeaders(t, r, "")
		writeStubJSON(t, w, `{"id":"`+rpcEntityID+`","status":"PLANNED",`+
			`"participants":[{"userId":"`+rpcUserID+`","role":"ORGANIZER",`+
			`"status":"ACCEPTED","joinedAt":"2026-09-28T12:00:00Z"}],"futureField":true}`)
	})
	id := rpcEntityID
	response, err := client.GetRequestDetails(requestAdapterContext(t),
		&gatewayv1.GetRequestDetailsRequest{RequestId: &id})
	if err != nil {
		t.Fatal(err)
	}
	request := response.GetRequest()
	if request.GetId() != rpcEntityID ||
		request.GetStatus() != gatewayv1.ActivityRequestStatus_ACTIVITY_REQUEST_STATUS_PLANNED ||
		len(request.GetParticipants().GetValues()) != 1 ||
		request.GetParticipants().GetValues()[0].GetUserId() != rpcUserID {
		t.Errorf("unexpected mapped details: %v", request)
	}
}

func TestRequestAdapterUpdate(t *testing.T) {
	const pathID = "a/b%c"
	client := newRequestAdapterClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.EscapedPath() != "/requests/a%2Fb%25c" {
			t.Errorf("Core request = %s %s", r.Method, r.URL.EscapedPath())
		}
		checkAdapterHeaders(t, r, rpcUserID)
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q", got)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		want := map[string]any{
			"description": "", "maxPlayers": float64(0),
			"eventDate": "2026-09-28T10:00:00Z",
		}
		if !reflect.DeepEqual(body, want) {
			t.Errorf("Core JSON = %#v, want %#v", body, want)
		}
		writeStubJSON(t, w, `{"id":"`+rpcEntityID+`","title":"updated"}`)
	})
	description, maxPlayers := "", int32(0)
	eventDate := timestamppb.New(time.Date(2026, 9, 28, 13, 0, 0, 0,
		time.FixedZone("UTC+3", 3*3600)))
	requestID := pathID
	response, err := client.UpdateRequest(requestAdapterContext(t),
		&gatewayv1.UpdateRequestRequest{
			RequestId: &requestID, Description: &description,
			MaxPlayers: &maxPlayers, EventDate: eventDate,
		})
	if err != nil {
		t.Fatal(err)
	}
	if response.GetRequest().GetId() != rpcEntityID || response.GetRequest().GetTitle() != "updated" {
		t.Errorf("unexpected mapped update: %v", response)
	}
}

func TestRequestAdapterSearch(t *testing.T) {
	zero, lon, sportID := float64(0), float64(37.5), int64(0)
	tests := []struct {
		name     string
		request  *gatewayv1.SearchNearbyRequestsRequest
		query    url.Values
		body     string
		wantSize int
	}{
		{
			name: "optional zero and UTC date",
			request: &gatewayv1.SearchNearbyRequestsRequest{
				Lat: &zero, Lon: &lon,
				Radius: &zero, SportId: &sportID,
				StartDate: timestamppb.New(time.Date(2026, 9, 28, 13, 0, 0, 0,
					time.FixedZone("UTC+3", 3*3600))),
			},
			query: url.Values{
				"lat": {"0"}, "lon": {"37.5"}, "radius": {"0"},
				"sportId": {"0"}, "startDate": {"2026-09-28T10:00:00Z"},
			},
			body:     `[{"id":"` + rpcEntityID + `","title":"football","eventDate":"2026-09-28T12:00:00Z"}]`,
			wantSize: 1,
		},
		{
			name:    "absent filters and empty list",
			request: &gatewayv1.SearchNearbyRequestsRequest{},
			query:   url.Values{},
			body:    `[]`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runSearchAdapterCase(t, tc.request, tc.query, tc.body, tc.wantSize)
		})
	}
}

func runSearchAdapterCase(
	t *testing.T,
	request *gatewayv1.SearchNearbyRequestsRequest,
	query url.Values,
	body string,
	wantSize int,
) {
	t.Helper()
	client := newRequestAdapterClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/requests" {
			t.Errorf("Core request = %s %s", r.Method, r.URL.Path)
		}
		checkAdapterHeaders(t, r, "")
		if got := r.URL.Query(); !reflect.DeepEqual(got, query) {
			t.Errorf("query = %v, want %v", got, query)
		}
		writeStubJSON(t, w, body)
	})
	response, err := client.SearchNearbyRequests(requestAdapterContext(t), request)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(response.GetRequests()); got != wantSize {
		t.Errorf("feed count = %d, want %d", got, wantSize)
	}
	if wantSize == 1 && response.GetRequests()[0].GetId() != rpcEntityID {
		t.Errorf("unexpected mapped feed: %v", response.GetRequests()[0])
	}
}

func TestRequestAdapterCreate(t *testing.T) {
	client := newRequestAdapterClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/requests" {
			t.Errorf("Core request = %s %s", r.Method, r.URL.Path)
		}
		checkAdapterHeaders(t, r, rpcUserID)
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		want := map[string]any{
			"title": "request", "description": "", "sportId": float64(0),
			"maxPlayers": float64(0), "eventDate": "2026-09-28T12:00:00Z",
			"numberOfRounds": float64(0), "addressText": "",
			"latitude": float64(0), "longitude": float64(0),
		}
		if !reflect.DeepEqual(body, want) {
			t.Errorf("Core JSON = %#v, want %#v", body, want)
		}
		w.WriteHeader(http.StatusCreated)
		writeStubJSON(t, w, `{"requestId":"`+rpcEntityID+`"}`)
	})
	title, empty := "request", ""
	zeroInt32, zeroInt64, zeroFloat := int32(0), int64(0), float64(0)
	response, err := client.CreateRequest(requestAdapterContext(t),
		&gatewayv1.CreateRequestRequest{
			Title: &title, Description: &empty, SportId: &zeroInt64,
			MaxPlayers: &zeroInt32, EventDate: timestamppb.New(rpcClock),
			NumberOfRounds: &zeroInt32, AddressText: &empty,
			Latitude: &zeroFloat, Longitude: &zeroFloat,
		})
	if err != nil {
		t.Fatal(err)
	}
	if response.GetRequestId() != rpcEntityID {
		t.Errorf("created request ID = %q, want %q", response.GetRequestId(), rpcEntityID)
	}
}

func TestRequestAdapterRejectsNonfiniteValues(t *testing.T) {
	var calls atomic.Int32
	client := newRequestAdapterClient(t, func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusOK)
	})
	for _, tc := range []struct {
		name string
		call func() error
	}{
		{"search NaN", func() error {
			v := math.NaN()
			_, err := client.SearchNearbyRequests(requestAdapterContext(t),
				&gatewayv1.SearchNearbyRequestsRequest{Lat: &v})
			return err
		}},
		{"create Infinity", func() error {
			v := math.Inf(1)
			_, err := client.CreateRequest(requestAdapterContext(t),
				&gatewayv1.CreateRequestRequest{Latitude: &v})
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call()
			if status.Code(err) != codes.InvalidArgument {
				t.Fatalf("status = %v, want INVALID_ARGUMENT; err = %v", status.Code(err), err)
			}
			details := status.Convert(err).Details()
			if len(details) != 1 {
				t.Fatalf("error details = %v, want one detail", details)
			}
			detail, ok := details[0].(*gatewayv1.GatewayErrorDetail)
			if !ok || detail.GetCode() != "INVALID_REQUEST" {
				t.Errorf("error details = %v, want INVALID_REQUEST", details)
			}
		})
	}
	if got := calls.Load(); got != 0 {
		t.Errorf("Core calls = %d, want 0", got)
	}
}

func TestRequestAdapterUpdateBusinessErrors(t *testing.T) {
	for _, tc := range []struct {
		httpStatus int
		grpcCode   codes.Code
		publicCode string
	}{
		{400, codes.InvalidArgument, "INVALID_ARGUMENT"},
		{403, codes.PermissionDenied, "PERMISSION_DENIED"},
		{404, codes.NotFound, "NOT_FOUND"},
		{409, codes.FailedPrecondition, "FAILED_PRECONDITION"},
	} {
		t.Run(http.StatusText(tc.httpStatus), func(t *testing.T) {
			client := newRequestAdapterClient(t, func(w http.ResponseWriter, r *http.Request) {
				checkAdapterHeaders(t, r, rpcUserID)
				w.WriteHeader(tc.httpStatus)
				writeStubJSON(t, w, `{"code":"SECRET_CODE","message":"private message",`+
					`"details":{"private":"value"}}`)
			})
			id := rpcEntityID
			_, err := client.UpdateRequest(requestAdapterContext(t),
				&gatewayv1.UpdateRequestRequest{RequestId: &id})
			if status.Code(err) != tc.grpcCode {
				t.Fatalf("status = %v, want %v; err = %v", status.Code(err), tc.grpcCode, err)
			}
			details := status.Convert(err).Details()
			if len(details) != 1 {
				t.Fatalf("error details = %v, want one CoreErrorDetail", details)
			}
			detail, ok := details[0].(*gatewayv1.CoreErrorDetail)
			if !ok || int(detail.GetHttpStatus()) != tc.httpStatus ||
				detail.GetCode() != tc.publicCode || detail.GetRequestId() != rpcRequestID ||
				detail.Message != nil || detail.Details != nil {
				t.Errorf("public error detail = %v", details[0])
			}
		})
	}
}
