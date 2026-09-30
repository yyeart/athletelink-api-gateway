package httpapi

import (
	"net/http"
	"net/url"
	"strings"
)

type upstream uint8

const (
	authUpstream upstream = iota
	gameUpstream
)

type accessPolicy uint8

const (
	publicAccess accessPolicy = iota
	authenticatedAccess
	ownUserAccess
)

type routeSpec struct {
	method   string
	pattern  string
	upstream upstream
	access   accessPolicy
}

type routeMatch struct {
	spec   routeSpec
	userID string
}

var apiRoutes = [...]routeSpec{
	{http.MethodPut, "/api/v1/user/update-user/{userId}", authUpstream, ownUserAccess},
	{http.MethodPut, "/api/v1/user/change-password/{userId}", authUpstream, ownUserAccess},
	{http.MethodPut, "/api/v1/user/change-email/{userId}", authUpstream, ownUserAccess},
	{http.MethodPost, "/api/v1/verification/verify-password-reset", authUpstream, publicAccess},
	{http.MethodPost, "/api/v1/verification/verify-email/{userId}", authUpstream, ownUserAccess},
	{http.MethodPost, "/api/v1/verification/send-password-reset-code", authUpstream, publicAccess},
	{http.MethodPost, "/api/v1/verification/send-email-verification-code/{userId}", authUpstream, ownUserAccess},
	{http.MethodPost, "/api/v1/user/register", authUpstream, publicAccess},
	{http.MethodPost, "/api/v1/auth/refresh", authUpstream, publicAccess},
	{http.MethodPost, "/api/v1/auth/login", authUpstream, publicAccess},
	{http.MethodGet, "/api/v1/session/get-all/{userId}", authUpstream, ownUserAccess},
	{http.MethodDelete, "/api/v1/user/delete/{userId}", authUpstream, ownUserAccess},
	{http.MethodDelete, "/api/v1/session/terminate/{userId}", authUpstream, ownUserAccess},
	{http.MethodDelete, "/api/v1/session/terminate-all/{userId}", authUpstream, ownUserAccess},
	{http.MethodDelete, "/api/v1/auth/logout", authUpstream, authenticatedAccess},

	{http.MethodGet, "/api/v1/results/by-request/{activityRequestId}", gameUpstream, authenticatedAccess},
	{http.MethodGet, "/api/v1/rank-tiers", gameUpstream, authenticatedAccess},
	{http.MethodGet, "/api/v1/players/{userId}", gameUpstream, authenticatedAccess},
	{http.MethodGet, "/api/v1/players/{userId}/sports/{sportId}", gameUpstream, authenticatedAccess},
	{http.MethodGet, "/api/v1/players/{userId}/matches", gameUpstream, authenticatedAccess},
	{http.MethodGet, "/api/v1/leaderboards/{sportId}", gameUpstream, authenticatedAccess},
}

func findAPIRoute(escapedPath string) (routeMatch, bool) {
	for _, spec := range apiRoutes {
		userID, ok := matchPattern(spec.pattern, escapedPath)
		if ok {
			return routeMatch{
				spec:   spec,
				userID: userID,
			}, true
		}
	}

	return routeMatch{}, false
}

func matchPattern(pattern, escapedPath string) (userID string, ok bool) {
	if !strings.HasPrefix(pattern, "/") ||
		!strings.HasPrefix(escapedPath, "/") {
		return "", false
	}

	want := strings.Split(pattern[1:], "/")
	got := strings.Split(escapedPath[1:], "/")
	if len(want) != len(got) {
		return "", false
	}

	for i, segment := range want {
		value := got[i]

		if strings.HasPrefix(segment, "{") && strings.HasSuffix(segment, "}") {
			decoded, valid := validPathParameter(value)
			if !valid {
				return "", false
			}

			if segment == "{userId}" {
				userID = decoded
			}

			continue
		}

		if value != segment {
			return "", false
		}
	}

	return userID, true
}

func validPathParameter(value string) (string, bool) {
	if value == "" {
		return "", false
	}
	decoded, err := url.PathUnescape(value)
	if err != nil || decoded == "" || strings.Contains(decoded, "/") || decoded == "." || decoded == ".." {
		return "", false
	}
	return decoded, true
}
