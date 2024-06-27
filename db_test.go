package ripoff

import (
	"context"
	"fmt"
	"os"
	"path"
	"runtime"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func runTestData(t *testing.T, ctx context.Context, tx pgx.Tx, testDir string) {
	schemaFile, err := os.ReadFile(path.Join(testDir, "schema.sql"))
	require.NoError(t, err)
	_, err = tx.Exec(ctx, string(schemaFile))
	require.NoError(t, err)
	totalRipoff, err := RipoffFromDirectory(testDir)
	require.NoError(t, err)
	err = RunRipoff(ctx, tx, totalRipoff)
	require.NoError(t, err)
	// Run again to implicitly test upsert behavior.
	err = RunRipoff(ctx, tx, totalRipoff)
	require.NoError(t, err)
	// Try to verify that the number of generated rows matches the ripoff.
	tableCount := map[string]int{}
	for rowId := range totalRipoff.Rows {
		tableName := strings.Split(rowId, ":")
		if len(tableName) > 0 {
			tableCount[tableName[0]]++
		}
	}
	for tableName, expectedCount := range tableCount {
		row := tx.QueryRow(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s;", pq.QuoteIdentifier(tableName)))
		var realCount int
		err := row.Scan(&realCount)
		require.NoError(t, err)
		require.Equal(t, expectedCount, realCount)
	}
	// Test output further if needed.
	validationFile, err := os.ReadFile(path.Join(testDir, "validate.sql"))
	if err == nil {
		row := tx.QueryRow(ctx, string(validationFile))
		var success int
		var debug string
		err := row.Scan(&success, &debug)
		require.NoError(t, err)
		if success != 1 {
			t.Fatalf("Validation failed with debug content: %s", debug)
		}
	}
}

func TestRipoff(t *testing.T) {
	envUrl := os.Getenv("RIPOFF_TEST_DATABASE_URL")
	if envUrl == "" {
		envUrl = "postgres:///ripoff-test-db"
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, envUrl)
	if err != nil {
		require.NoError(t, err)
	}
	defer conn.Close(ctx)

	_, filename, _, _ := runtime.Caller(0)
	dir := path.Join(path.Dir(filename), "testdata")
	dirEntry, err := os.ReadDir(dir)
	require.NoError(t, err)

	for _, e := range dirEntry {
		if !e.IsDir() {
			continue
		}
		tx, err := conn.Begin(ctx)
		require.NoError(t, err)
		runTestData(t, ctx, tx, path.Join(dir, e.Name()))
		err = tx.Rollback(ctx)
		require.NoError(t, err)
	}
}
