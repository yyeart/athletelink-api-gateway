package auth

import (
	"context"
	"errors"
	"regexp"
	"time"
	"unicode/utf8"

	"github.com/golang-jwt/jwt/v5"
	"gitlab.com/team-anonyms/athelete-link/api-gateway/internal/requestcontext"
)

var (
	ErrInvalidToken     = errors.New("invalid access token")
	ErrCheckUnavailable = errors.New("denylist check unavailable")
	ErrRedisUnavailable = errors.New("redis unavailable or timed out")
)

var uuidPattern = regexp.MustCompile(
	`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`,
)

type Denylist interface {
	IsDenied(ctx context.Context, jti string) (bool, error)
}

type claims struct {
	Type string `json:"type"`
	jwt.RegisteredClaims
}

type Verifier struct {
	key      []byte
	denylist Denylist
	clock    func() time.Time
}

func NewVerifier(
	secret string,
	denylist Denylist,
	clock func() time.Time,
) (*Verifier, error) {
	if denylist == nil || clock == nil {
		return nil, errors.New("incomplete verifier configuration")
	}
	if secret == "" {
		return nil, errors.New("JWT secret must not be empty")
	}
	if !utf8.ValidString(secret) {
		return nil, errors.New("JWT secret must be valid UTF-8")
	}

	return &Verifier{
		key:      []byte(secret),
		denylist: denylist,
		clock:    clock,
	}, nil
}

func (v *Verifier) VerifyAccessToken(
	ctx context.Context,
	raw string,
) (requestcontext.Identity, error) {
	var identity requestcontext.Identity
	now := v.clock()

	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{jwt.SigningMethodHS512.Alg()}),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithTimeFunc(func() time.Time { return now }),
	)

	var c claims
	token, err := parser.ParseWithClaims(raw, &c, func(t *jwt.Token) (any, error) {
		if t.Method.Alg() != jwt.SigningMethodHS512.Alg() {
			return nil, ErrInvalidToken
		}

		return v.key, nil
	})
	if err != nil || !token.Valid {
		return identity, ErrInvalidToken
	}

	if c.Type != "ACCESS" ||
		!uuidPattern.MatchString(c.Subject) ||
		!uuidPattern.MatchString(c.ID) ||
		c.IssuedAt == nil ||
		c.ExpiresAt == nil {
		return identity, ErrInvalidToken
	}

	validIAT := !now.Before(c.IssuedAt.Time)
	validEXP := now.Before(c.ExpiresAt.Time)
	if !validIAT || !validEXP {
		return identity, ErrInvalidToken
	}

	denied, err := v.denylist.IsDenied(ctx, c.ID)
	if ctx.Err() != nil {
		return identity, ctx.Err()
	}
	if err != nil {
		if errors.Is(err, ErrRedisUnavailable) {
			return requestcontext.Identity{UserID: c.Subject}, nil
		}
		return identity, ErrCheckUnavailable
	}
	if denied {
		return identity, ErrInvalidToken
	}

	return requestcontext.Identity{UserID: c.Subject}, nil
}
