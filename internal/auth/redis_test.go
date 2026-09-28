package auth

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

type existsClientStub struct {
	calls         int
	contextMarker bool
	keys          []string
	count         int64
	err           error
}

func (s *existsClientStub) Exists(ctx context.Context, keys ...string) *redis.IntCmd {
	s.calls++
	s.contextMarker = ctx.Value(testContextKey{}) == "marker"
	s.keys = append([]string(nil), keys...)
	if err := ctx.Err(); err != nil {
		return redis.NewIntResult(0, err)
	}
	return redis.NewIntResult(s.count, s.err)
}

func TestRedisDenylistChecksExactKey(t *testing.T) {
	tests := []struct {
		name       string
		count      int64
		wantDenied bool
	}{
		{name: "key absent", count: 0},
		{name: "key exists with empty value", count: 1, wantDenied: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := &existsClientStub{count: tc.count}
			ctx := context.WithValue(context.Background(), testContextKey{}, "marker")
			denied, err := NewRedisDenylist(client).IsDenied(ctx, testJTI)
			if err != nil || denied != tc.wantDenied {
				t.Errorf("denied = %v, error = %v; want denied = %v",
					denied, err, tc.wantDenied)
			}
			if client.calls != 1 || !client.contextMarker ||
				!reflect.DeepEqual(client.keys, []string{"jwt:denylist:" + testJTI}) {
				t.Errorf("EXISTS calls = %d, keys = %v, context marker = %v",
					client.calls, client.keys, client.contextMarker)
			}
		})
	}
}

func TestRedisDenylistPropagatesFailures(t *testing.T) {
	tests := []struct {
		name string
		ctx  func() (context.Context, context.CancelFunc)
		err  error
	}{
		{name: "redis error", ctx: backgroundContext, err: errors.New("redis down")},
		{name: "redis timeout", ctx: backgroundContext, err: context.DeadlineExceeded},
		{name: "cancelled context", ctx: cancelledContext, err: context.Canceled},
		{name: "expired context", ctx: expiredContext, err: context.DeadlineExceeded},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := tc.ctx()
			defer cancel()
			client := &existsClientStub{err: tc.err}
			denied, err := NewRedisDenylist(client).IsDenied(ctx, testJTI)
			if denied || !errors.Is(err, tc.err) || client.calls != 1 {
				t.Errorf("denied = %v, error = %v, EXISTS calls = %d; want %v",
					denied, err, client.calls, tc.err)
			}
		})
	}
}

func backgroundContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(context.Background())
}

func cancelledContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx, cancel
}

func expiredContext() (context.Context, context.CancelFunc) {
	return context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
}
