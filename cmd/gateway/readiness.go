package main

import (
	"context"
	"fmt"
	"net/http"

	"gitlab.com/team-anonyms/athelete-link/api-gateway/internal/config"
)

func newReadinessCheck(
	cfg config.Config,
	checkRedis func(context.Context) error,
) func(context.Context) error {
	client := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	services := [...]struct {
		name string
		url  string
	}{
		{"Auth", cfg.AuthHealthURL},
		{"Core", cfg.CoreHealthURL},
		{"Game", cfg.GameHealthURL},
	}

	return func(parent context.Context) error {
		ctx, cancel := context.WithTimeout(parent, cfg.HealthTimeout)
		defer cancel()

		if err := checkRedis(ctx); err != nil {
			return fmt.Errorf("Redis health check: %w", err)
		}

		for _, service := range services {
			request, err := http.NewRequestWithContext(ctx, http.MethodGet, service.url, nil)
			if err != nil {
				return fmt.Errorf("%s health request: %w", service.name, err)
			}

			response, err := client.Do(request)
			if err != nil {
				return fmt.Errorf("%s health check: %w", service.name, err)
			}
			if err := response.Body.Close(); err != nil {
				return fmt.Errorf("%s health response: %w", service.name, err)
			}
			if response.StatusCode != http.StatusOK {
				return fmt.Errorf("%s health status: %d", service.name, response.StatusCode)
			}
		}

		return ctx.Err()
	}
}
