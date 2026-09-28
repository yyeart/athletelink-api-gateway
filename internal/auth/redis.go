package auth

import (
	"context"

	"github.com/redis/go-redis/v9"
)

type existsClient interface {
	Exists(ctx context.Context, keys ...string) *redis.IntCmd
}

type RedisDenylist struct {
	client existsClient
}

func NewRedisDenylist(client existsClient) *RedisDenylist {
	return &RedisDenylist{client: client}
}

func (r *RedisDenylist) IsDenied(
	ctx context.Context,
	jti string,
) (bool, error) {
	count, err := r.client.Exists(ctx, "jwt:denylist:"+jti).Result()
	if err != nil {
		return false, err
	}

	return count != 0, nil
}
