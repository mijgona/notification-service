//go:build integration

// Package integration holds the build-tagged end-to-end suite. It stands up
// real Postgres and Mailpit via testcontainers and a real Temporal server via
// the SDK dev server, applies the goose migrations, and drives the production
// components against them. Excluded from the default `go test ./...`; run with
// `go test -tags=integration ./test/integration/...` (or `make test-integration`).
package integration

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // database/sql driver "pgx" for goose
	"github.com/pressly/goose/v3"
	tc "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.temporal.io/sdk/testsuite"
)

// Env holds connection details for the shared real dependencies (see
// contracts/integration-harness.md).
type Env struct {
	PostgresURL       string
	TemporalHostPort  string
	MailpitSMTPAddr   string
	MailpitAPIBaseURL string
}

var (
	testEnv   Env
	testPool  *pgxpool.Pool
	teardowns []func()
)

func addTeardown(fn func()) { teardowns = append(teardowns, fn) }

func runTeardowns() {
	for i := len(teardowns) - 1; i >= 0; i-- {
		teardowns[i]()
	}
	teardowns = nil
}

// TestMain stands up the shared dependencies once for the whole suite. If no
// container engine is available the suite is skipped (FR-006). Everything is
// torn down on the way out (FR-003); testcontainers' Ryuk is the backstop.
func TestMain(m *testing.M) {
	if !dockerAvailable() {
		fmt.Fprintln(os.Stderr, "integration: no container engine available; skipping suite")
		os.Exit(0)
	}

	ctx := context.Background()
	if err := setup(ctx); err != nil {
		runTeardowns()
		fmt.Fprintf(os.Stderr, "integration: setup failed: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()
	runTeardowns()
	os.Exit(code)
}

func dockerAvailable() bool {
	p, err := tc.NewDockerProvider()
	if err != nil {
		return false
	}
	defer func() { _ = p.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return p.Health(ctx) == nil
}

func setup(ctx context.Context) error {
	// --- Postgres (T003) ---
	pgC, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("notify"),
		postgres.WithUsername("notify"),
		postgres.WithPassword("notify"),
		tc.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		return fmt.Errorf("start postgres: %w", err)
	}
	addTeardown(func() { _ = pgC.Terminate(context.Background()) })

	pgURL, err := pgC.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return fmt.Errorf("postgres conn string: %w", err)
	}
	testEnv.PostgresURL = pgURL

	// --- Migrations (T004) ---
	if err := applyMigrations(pgURL); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}

	// shared pool for resetState / direct assertions
	pool, err := pgxpool.New(ctx, pgURL)
	if err != nil {
		return fmt.Errorf("test pool: %w", err)
	}
	testPool = pool
	addTeardown(pool.Close)

	// --- Mailpit (T003) ---
	mpReq := tc.ContainerRequest{
		Image:        "axllent/mailpit:latest",
		ExposedPorts: []string{"1025/tcp", "8025/tcp"},
		WaitingFor:   wait.ForListeningPort("8025/tcp").WithStartupTimeout(60 * time.Second),
	}
	mpC, err := tc.GenericContainer(ctx, tc.GenericContainerRequest{
		ContainerRequest: mpReq,
		Started:          true,
	})
	if err != nil {
		return fmt.Errorf("start mailpit: %w", err)
	}
	addTeardown(func() { _ = mpC.Terminate(context.Background()) })

	mpHost, err := mpC.Host(ctx)
	if err != nil {
		return fmt.Errorf("mailpit host: %w", err)
	}
	smtpPort, err := mpC.MappedPort(ctx, "1025")
	if err != nil {
		return fmt.Errorf("mailpit smtp port: %w", err)
	}
	apiPort, err := mpC.MappedPort(ctx, "8025")
	if err != nil {
		return fmt.Errorf("mailpit api port: %w", err)
	}
	testEnv.MailpitSMTPAddr = fmt.Sprintf("%s:%s", mpHost, smtpPort.Port())
	testEnv.MailpitAPIBaseURL = fmt.Sprintf("http://%s:%s", mpHost, apiPort.Port())

	// --- Real Temporal dev server (T005) ---
	devServer, err := testsuite.StartDevServer(ctx, testsuite.DevServerOptions{})
	if err != nil {
		return fmt.Errorf("start temporal dev server: %w", err)
	}
	addTeardown(func() { _ = devServer.Stop() })
	testEnv.TemporalHostPort = devServer.FrontendHostPort()

	return nil
}

func applyMigrations(pgURL string) error {
	db, err := sql.Open("pgx", pgURL)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	return goose.Up(db, migrationsDir())
}

func migrationsDir() string {
	_, thisFile, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "migrations")
}

// resetState truncates the mutable tables so each test starts clean and the
// global relay never sees another test's pending rows (U1, FR-007).
func resetState(t *testing.T, env Env) {
	t.Helper()
	_ = env
	_, err := testPool.Exec(context.Background(),
		`TRUNCATE delivery_attempts, outbox, notifications RESTART IDENTITY CASCADE`)
	if err != nil {
		t.Fatalf("resetState: %v", err)
	}
}
