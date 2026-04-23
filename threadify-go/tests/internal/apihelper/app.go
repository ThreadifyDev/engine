package apihelper

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"

	"go.uber.org/zap"

	"threadify-go/api/app"
	"threadify-go/shared/config"
)

type App struct {
	BaseURL string
	Client  *http.Client

	ts  *httptest.Server
	app *app.App
}

type StartOptions struct {
	ConfigPath string
}

func Start(ctx context.Context, opts StartOptions) (*App, error) {
	if opts.ConfigPath == "" {
		return nil, fmt.Errorf("config path is required")
	}

	cfg, err := config.Load(opts.ConfigPath)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	logger, _ := zap.NewDevelopment()

	rbacPaths, err := app.ResolveRBACPaths(logger)
	if err != nil {
		return nil, fmt.Errorf("resolve rbac paths: %w", err)
	}

	application, err := app.New(ctx, cfg, rbacPaths, logger)
	if err != nil {
		return nil, fmt.Errorf("app initialize: %w", err)
	}

	ts := httptest.NewServer(application.Handler)

	return &App{
		BaseURL: ts.URL,
		Client:  ts.Client(),
		ts:      ts,
		app:     application,
	}, nil
}

func (a *App) Stop(ctx context.Context) error {
	if a == nil {
		return nil
	}
	if a.ts != nil {
		a.ts.Close()
	}
	if a.app != nil {
		logger, _ := zap.NewDevelopment()
		return a.app.Close(ctx, logger)
	}
	return nil
}
