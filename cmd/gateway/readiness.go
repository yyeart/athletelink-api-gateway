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
			return fmt.Errorf("redis health check: %w", err)
		}

		for _, service := range services {
			if err := checkServiceHealth(ctx, client, service.name, service.url); err != nil {
				return err
			}
		}

		return ctx.Err()
	}
}

func checkServiceHealth(ctx context.Context, client *http.Client, name, url string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("%s health request: %w", name, err)
	}

	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("%s health check: %w", name, err)
	}
	if err := response.Body.Close(); err != nil {
		return fmt.Errorf("%s health response: %w", name, err)
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("%s health status: %d", name, response.StatusCode)
	}
	return nil
}
