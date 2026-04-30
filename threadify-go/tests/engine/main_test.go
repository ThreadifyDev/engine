package engine

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/threadify/engine/tests/internal/apihelper"
	"github.com/threadify/engine/tests/internal/enginehelper"
	"github.com/threadify/engine/tests/internal/enginetest"
	"github.com/threadify/engine/tests/internal/testenv"
	"go.uber.org/zap"
)

var (
	testCtx   context.Context
	env       *testenv.Environment
	supabase  *apihelper.FakeSupabase
	engineApp *enginehelper.EngineApp
	logger    *zap.Logger
	httpc     *enginetest.HTTPClient
)

func requireDockerForIntegration() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("THREADIFY_REQUIRE_DOCKER"))) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

func TestMain(m *testing.M) {
	flag.Parse()
	if testing.Short() {
		os.Exit(0)
	}

	// Change directory to root so we can find shared/rbac files
	if err := os.Chdir(filepath.Clean(filepath.Join("..", ".."))); err != nil {
		panic(err)
	}

	testCtx = context.Background()
	logger, _ = zap.NewDevelopment()

	startCtx, cancel := context.WithTimeout(testCtx, 2*time.Minute)
	defer cancel()

	var err error
	env, err = testenv.Start(startCtx)
	if err != nil {
		if errors.Is(err, testenv.ErrDockerUnavailable) && !requireDockerForIntegration() {
			_, _ = fmt.Fprintf(os.Stderr, "SKIP: engine integration tests require Docker (set THREADIFY_REQUIRE_DOCKER=1 to fail instead): %v\n", err)
			os.Exit(0)
		}
		panic(err)
	}

	supabase = apihelper.StartFakeSupabase()

	cfg, err := enginehelper.GenerateTestConfig(
		env.Postgres.ConnectionString,
		env.Valkey.URI,
		env.Nats.URI,
		supabase.URL,
	)
	if err != nil {
		panic(err)
	}

	engineApp, err = enginehelper.StartApp(testCtx, cfg, logger)
	if err != nil {
		panic(err)
	}
	httpc = enginetest.NewHTTPClient(engineApp.BaseURL, engineApp.WSURL, engineApp.Client)

	code := m.Run()

	if engineApp != nil {
		engineApp.Stop(testCtx)
	}
	if supabase != nil {
		supabase.Close()
	}
	if env != nil {
		stopCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		env.Stop(stopCtx)
	}

	os.Exit(code)
}
