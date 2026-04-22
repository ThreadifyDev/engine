package engine

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/threadify/engine/tests/internal/apihelper"
	"github.com/threadify/engine/tests/internal/enginetest"
	"github.com/threadify/engine/tests/internal/enginehelper"
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
