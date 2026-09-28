package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	testUserID = "123e4567-e89b-42d3-a456-426614174001"
	testJTI    = "123e4567-e89b-42d3-a456-426614174002"
)

var (
	testSigningKey = []byte("test-only-signing-key-1234567890")
	testClockTime  = time.Date(2026, time.September, 28, 12, 0, 0, 0, time.UTC)
)

type denylistStub struct {
	calls         int
	jti           string
	contextMarker bool
	denied        bool
	err           error
}

func (s *denylistStub) IsDenied(ctx context.Context, jti string) (bool, error) {
	s.calls++
	s.jti = jti
	s.contextMarker = ctx.Value(testContextKey{}) == "marker"
	return s.denied, s.err
}

func accessClaims() jwt.MapClaims {
	return jwt.MapClaims{
		"sub":  testUserID,
		"jti":  testJTI,
		"type": "ACCESS",
		"iat":  testClockTime.Add(-time.Minute).Unix(),
		"exp":  testClockTime.Add(time.Minute).Unix(),
	}
}

func signTestToken(t *testing.T, method jwt.SigningMethod, key []byte, claims jwt.MapClaims) string {
	t.Helper()
	raw, err := jwt.NewWithClaims(method, claims).SignedString(key)
	if err != nil {
		t.Fatalf("sign test token: %v", err)
	}
	return raw
}

func newTestVerifier(t *testing.T, denylist Denylist) *Verifier {
	t.Helper()
	verifier, err := NewVerifier(string(testSigningKey), denylist, func() time.Time {
		return testClockTime
	})
	if err != nil {
		t.Fatalf("new verifier: %v", err)
	}
	return verifier
}

func TestNewVerifierSelectsAlgorithmBySecretByteLength(t *testing.T) {
	tests := []struct {
		name      string
		secret    string
		algorithm string
		wantErr   bool
	}{
		{name: "empty", wantErr: true},
		{name: "invalid UTF-8", secret: string([]byte{0xff}), wantErr: true},
		{name: "31 bytes", secret: strings.Repeat("a", 31), wantErr: true},
		{name: "32 bytes", secret: strings.Repeat("a", 32), algorithm: "HS256"},
		{name: "47 bytes", secret: strings.Repeat("a", 47), algorithm: "HS256"},
		{name: "48 bytes", secret: strings.Repeat("a", 48), algorithm: "HS384"},
		{name: "63 bytes", secret: strings.Repeat("a", 63), algorithm: "HS384"},
		{name: "64 bytes", secret: strings.Repeat("a", 64), algorithm: "HS512"},
		{name: "UTF-8 bytes", secret: strings.Repeat("é", 16), algorithm: "HS256"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			checkVerifierAlgorithm(t, tc.secret, tc.algorithm, tc.wantErr)
		})
	}
}

func checkVerifierAlgorithm(t *testing.T, secret, algorithm string, wantErr bool) {
	t.Helper()
	lookup := &denylistStub{}
	verifier, err := NewVerifier(secret, lookup, func() time.Time {
		return testClockTime
	})
	if wantErr {
		if err == nil {
			t.Fatal("NewVerifier() error = nil, want non-nil error")
		}
		return
	}
	if err != nil {
		t.Fatalf("NewVerifier() error = %v", err)
	}
	if verifier.algorithm != algorithm {
		t.Errorf("algorithm = %q, want %q", verifier.algorithm, algorithm)
	}
	if string(verifier.key) != secret {
		t.Error("verifier key does not match the UTF-8 secret bytes")
	}

	methods := map[string]jwt.SigningMethod{
		"HS256": jwt.SigningMethodHS256,
		"HS384": jwt.SigningMethodHS384,
		"HS512": jwt.SigningMethodHS512,
	}
	raw := signTestToken(t, methods[algorithm], []byte(secret), accessClaims())
	identity, err := verifier.VerifyAccessToken(context.Background(), raw)
	if err != nil || identity.UserID != testUserID || lookup.calls != 1 {
		t.Errorf("VerifyAccessToken() = (%+v, %v), denylist calls = %d; want valid token",
			identity, err, lookup.calls)
	}
}

func TestVerifierAcceptsAccessToken(t *testing.T) {
	lookup := &denylistStub{}
	verifier := newTestVerifier(t, lookup)
	raw := signTestToken(t, jwt.SigningMethodHS256, testSigningKey, accessClaims())
	ctx := context.WithValue(context.Background(), testContextKey{}, "marker")

	identity, err := verifier.VerifyAccessToken(ctx, raw)
	if err != nil {
		t.Fatalf("verify access token: %v", err)
	}
	if identity.UserID != testUserID {
		t.Errorf("user ID = %q, want %q", identity.UserID, testUserID)
	}
	if lookup.calls != 1 || lookup.jti != testJTI || !lookup.contextMarker {
		t.Errorf("denylist lookup = (%d, %q, context marker: %v)",
			lookup.calls, lookup.jti, lookup.contextMarker)
	}
}

type testContextKey struct{}

func TestVerifierRejectsInvalidTokensBeforeDenylist(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(jwt.MapClaims)
		method jwt.SigningMethod
		key    []byte
	}{
		{name: "refresh", mutate: func(c jwt.MapClaims) { c["type"] = "REFRESH" }},
		{name: "missing type", mutate: func(c jwt.MapClaims) { delete(c, "type") }},
		{name: "invalid type", mutate: func(c jwt.MapClaims) { c["type"] = 123 }},
		{name: "missing subject", mutate: func(c jwt.MapClaims) { delete(c, "sub") }},
		{name: "invalid subject", mutate: func(c jwt.MapClaims) { c["sub"] = "not-a-uuid" }},
		{name: "missing jti", mutate: func(c jwt.MapClaims) { delete(c, "jti") }},
		{name: "invalid jti", mutate: func(c jwt.MapClaims) { c["jti"] = "not-a-uuid" }},
		{name: "missing issued at", mutate: func(c jwt.MapClaims) { delete(c, "iat") }},
		{name: "invalid issued at", mutate: func(c jwt.MapClaims) { c["iat"] = "yesterday" }},
		{name: "missing expiration", mutate: func(c jwt.MapClaims) { delete(c, "exp") }},
		{name: "invalid expiration", mutate: func(c jwt.MapClaims) { c["exp"] = "tomorrow" }},
		{name: "expired beyond leeway", mutate: func(c jwt.MapClaims) { c["exp"] = testClockTime.Add(-time.Minute).Unix() }},
		{name: "issued beyond leeway", mutate: func(c jwt.MapClaims) { c["iat"] = testClockTime.Add(time.Minute + time.Second).Unix() }},
		{name: "not before beyond leeway", mutate: func(c jwt.MapClaims) { c["nbf"] = testClockTime.Add(time.Minute + time.Second).Unix() }},
		{name: "wrong signature", key: []byte("different-test-key")},
		{name: "unsupported algorithm", method: jwt.SigningMethodHS384},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assertInvalidTokenBeforeDenylist(t, tc.mutate, tc.method, tc.key)
		})
	}

	lookup := &denylistStub{}
	identity, err := newTestVerifier(t, lookup).VerifyAccessToken(context.Background(), "not-a-jwt")
	if !errors.Is(err, ErrInvalidToken) || identity.UserID != "" || lookup.calls != 0 {
		t.Errorf("malformed token: identity = %+v, error = %v, denylist calls = %d",
			identity, err, lookup.calls)
	}
}

func assertInvalidTokenBeforeDenylist(
	t *testing.T,
	mutate func(jwt.MapClaims),
	method jwt.SigningMethod,
	key []byte,
) {
	t.Helper()
	claims := accessClaims()
	if mutate != nil {
		mutate(claims)
	}
	if method == nil {
		method = jwt.SigningMethodHS256
	}
	if key == nil {
		key = testSigningKey
	}
	lookup := &denylistStub{}
	identity, err := newTestVerifier(t, lookup).VerifyAccessToken(
		context.Background(), signTestToken(t, method, key, claims),
	)
	if !errors.Is(err, ErrInvalidToken) || identity.UserID != "" {
		t.Errorf("identity = %+v, error = %v; want invalid token", identity, err)
	}
	if lookup.calls != 0 {
		t.Errorf("denylist calls = %d, want 0", lookup.calls)
	}
}

func TestVerifierTimeBoundary(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(jwt.MapClaims)
		wantErr bool
	}{
		{name: "issued now", mutate: func(c jwt.MapClaims) { c["iat"] = testClockTime.Unix() }},
		{name: "issued one second ahead", mutate: func(c jwt.MapClaims) { c["iat"] = testClockTime.Add(time.Second).Unix() }},
		{name: "issued at leeway boundary", mutate: func(c jwt.MapClaims) { c["iat"] = testClockTime.Add(time.Minute).Unix() }},
		{name: "not before now", mutate: func(c jwt.MapClaims) { c["nbf"] = testClockTime.Unix() }},
		{name: "not before one second ahead", mutate: func(c jwt.MapClaims) { c["nbf"] = testClockTime.Add(time.Second).Unix() }},
		{name: "not before at leeway boundary", mutate: func(c jwt.MapClaims) { c["nbf"] = testClockTime.Add(time.Minute).Unix() }},
		{name: "expires one second later", mutate: func(c jwt.MapClaims) { c["exp"] = testClockTime.Add(time.Second).Unix() }},
		{name: "expires now", mutate: func(c jwt.MapClaims) { c["exp"] = testClockTime.Unix() }, wantErr: true},
		{name: "expired one second ago", mutate: func(c jwt.MapClaims) { c["exp"] = testClockTime.Add(-time.Second).Unix() }, wantErr: true},
		{name: "expired inside leeway", mutate: func(c jwt.MapClaims) { c["exp"] = testClockTime.Add(-time.Minute + time.Second).Unix() }, wantErr: true},
		{name: "expired at leeway boundary", mutate: func(c jwt.MapClaims) { c["exp"] = testClockTime.Add(-time.Minute).Unix() }, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			claims := accessClaims()
			tc.mutate(claims)
			lookup := &denylistStub{}
			identity, err := newTestVerifier(t, lookup).VerifyAccessToken(
				context.Background(), signTestToken(t, jwt.SigningMethodHS256, testSigningKey, claims),
			)
			if tc.wantErr {
				if !errors.Is(err, ErrInvalidToken) || identity.UserID != "" || lookup.calls != 0 {
					t.Errorf("identity = %+v, error = %v, denylist calls = %d; want rejection",
						identity, err, lookup.calls)
				}
				return
			}
			if err != nil || identity.UserID != testUserID || lookup.calls != 1 {
				t.Errorf("identity = %+v, error = %v, denylist calls = %d; want success",
					identity, err, lookup.calls)
			}
		})
	}
}

func TestVerifierFailsClosedOnDenylist(t *testing.T) {
	tests := []struct {
		name    string
		lookup  denylistStub
		wantErr error
	}{
		{name: "revoked", lookup: denylistStub{denied: true}, wantErr: ErrInvalidToken},
		{name: "lookup failure", lookup: denylistStub{err: errors.New("redis down")}, wantErr: ErrCheckUnavailable},
		{name: "lookup timeout", lookup: denylistStub{err: context.DeadlineExceeded}, wantErr: ErrCheckUnavailable},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lookup := &tc.lookup
			identity, err := newTestVerifier(t, lookup).VerifyAccessToken(
				context.Background(),
				signTestToken(t, jwt.SigningMethodHS256, testSigningKey, accessClaims()),
			)
			if !errors.Is(err, tc.wantErr) || identity.UserID != "" || lookup.calls != 1 {
				t.Errorf("identity = %+v, error = %v, denylist calls = %d; want %v",
					identity, err, lookup.calls, tc.wantErr)
			}
		})
	}
}
