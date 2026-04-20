package enginehelper

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/threadify/engine/internal/app"
	"github.com/threadify/engine/internal/config"
	"github.com/threadify/engine/internal/database"

	"go.uber.org/zap"
)

type EngineApp struct {
	BaseURL string
	WSURL   string
	Client  *http.Client

	valkey *database.ValkeyService

	ts  *httptest.Server
	app *app.App
	cfg *config.Config
}

func StartApp(ctx context.Context, cfg *config.Config, logger *zap.Logger) (*EngineApp, error) {
	a, err := app.New(ctx, cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("app new: %w", err)
	}

	ts := httptest.NewServer(a.Handler)
	baseURL := ts.URL
	wsURL := strings.Replace(baseURL, "http://", "ws://", 1)

	return &EngineApp{
		BaseURL: baseURL,
		WSURL:   wsURL,
		Client:  ts.Client(),
		valkey:  a.Valkey(),
		ts:      ts,
		app:     a,
		cfg:     cfg,
	}, nil
}

func (ea *EngineApp) NATSConfig() *config.NATSConfig {
	return &ea.cfg.NATS
}

func (ea *EngineApp) Valkey() *database.ValkeyService {
	return ea.valkey
}

func (ea *EngineApp) Stop(ctx context.Context) {
	if ea.ts != nil {
		ea.ts.Close()
	}
	if ea.app != nil {
		ea.app.Close(ctx)
	}
}
