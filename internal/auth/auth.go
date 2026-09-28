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
	key       []byte
	algorithm string
	denylist  Denylist
	clock     func() time.Time
}

func NewVerifier(
	secret string,
	denylist Denylist,
	clock func() time.Time,
) (*Verifier, error) {
	if denylist == nil || clock == nil {
		return nil, errors.New("incomplete verifier configuration")
	}
	algorithm, err := detectAlgorithm(secret)
	if err != nil {
		return nil, err
	}

	return &Verifier{
		key:       []byte(secret),
		algorithm: algorithm,
		denylist:  denylist,
		clock:     clock,
	}, nil
}

func detectAlgorithm(secret string) (string, error) {
	if !utf8.ValidString(secret) {
		return "", errors.New("JWT secret must be valid UTF-8")
	}

	length := len([]byte(secret))

	switch {
	case length >= 64:
		return "HS512", nil

	case length >= 48:
		return "HS384", nil

	case length >= 32:
		return "HS256", nil

	default:
		return "", errors.New("JWT secret must be at least 32 bytes")
	}
}

func (v *Verifier) VerifyAccessToken(
	ctx context.Context,
	raw string,
) (requestcontext.Identity, error) {
	var identity requestcontext.Identity

	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{v.algorithm}),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithTimeFunc(v.clock),
		jwt.WithLeeway(60*time.Second),
	)

	var c claims
	token, err := parser.ParseWithClaims(raw, &c, func(t *jwt.Token) (any, error) {
		if t.Method.Alg() != v.algorithm {
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

	now := v.clock()
	validIAT := !now.Add(60 * time.Second).Before(c.IssuedAt.Time)
	validEXP := now.Before(c.ExpiresAt.Time)
	if !validIAT || !validEXP {
		return identity, ErrInvalidToken
	}

	denied, err := v.denylist.IsDenied(ctx, c.ID)
	if err != nil {
		return identity, ErrCheckUnavailable
	}
	if denied {
		return identity, ErrInvalidToken
	}

	return requestcontext.Identity{UserID: c.Subject}, nil
}
