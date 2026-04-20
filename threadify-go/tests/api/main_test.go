package api

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/threadify/engine/tests/internal/apihelper"
	"github.com/threadify/engine/tests/internal/testenv"
)

var (
	testCtx  context.Context
	env      *testenv.Environment
	supabase *apihelper.FakeSupabase
	plunk    *apihelper.FakePlunk
	apiApp   *apihelper.App
)

func TestMain(m *testing.M) {
	flag.Parse()
	if testing.Short() {
		os.Exit(0)
	}

	if err := os.Chdir(filepath.Clean(filepath.Join("..", "..", "api"))); err != nil {
		panic(err)
	}

	testCtx = context.Background()

	startCtx, cancel := context.WithTimeout(testCtx, 2*time.Minute)
	defer cancel()

	var err error
	env, err = testenv.Start(startCtx)
	if err != nil {
		panic(err)
	}

	supabase = apihelper.StartFakeSupabase()
	plunk = apihelper.StartFakePlunk()

	tmpDir, err := os.MkdirTemp("", "threadify-api-it-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tmpDir)

	cfgPath, err := apihelper.WriteConfig(tmpDir, apihelper.APIServerConfig{
		PostgresURL: env.Postgres.ConnectionString,
		NATSURL:     env.Nats.URI,
		SupabaseURL: supabase.URL,
		PlunkAPIURL: plunk.URL,
		Port:        8080,
	})
	if err != nil {
		panic(err)
	}

	apiApp, err = apihelper.Start(testCtx, apihelper.StartOptions{
		ConfigPath: cfgPath,
	})
	if err != nil {
		panic(err)
	}

	code := m.Run()

	if apiApp != nil {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer stopCancel()
		_ = apiApp.Stop(stopCtx)
	}
	if plunk != nil {
		plunk.Close()
	}
	if supabase != nil {
		supabase.Close()
	}
	if env != nil {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer stopCancel()
		env.Stop(stopCtx)
	}

	os.Exit(code)
}
