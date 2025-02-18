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
	"gopkg.in/yaml.v3"
)

func runExportTestData(t *testing.T, ctx context.Context, tx pgx.Tx, testDir string) {
	// Set up schema and initial rows.
	setupFile, err := os.ReadFile(path.Join(testDir, "setup.sql"))
	require.NoError(t, err)
	_, err = tx.Exec(ctx, string(setupFile))
	require.NoError(t, err)

	// Generate new ripoff file.
	ripoffFile, err := ExportToRipoff(ctx, tx)
	require.NoError(t, err)

	// Ensure ripoff file matches expected output.
	// The marshal/unmashal dance here lets us ensure everything is an interface{}
	newRipoffFile := &RipoffFile{}
	newRipoffBytes, err := yaml.Marshal(ripoffFile)
	require.NoError(t, err)
	err = yaml.Unmarshal(newRipoffBytes, newRipoffFile)
	require.NoError(t, err)
	expectedRipoffYaml, err := os.ReadFile(path.Join(testDir, "ripoff.yml"))
	require.NoError(t, err)
	expectedRipoffFile := &RipoffFile{}
	err = yaml.Unmarshal(expectedRipoffYaml, expectedRipoffFile)
	require.NoError(t, err)
	require.Equal(t, expectedRipoffFile, newRipoffFile)

	// Wipe database.
	truncateFile, err := os.ReadFile(path.Join(testDir, "truncate.sql"))
	require.NoError(t, err)
	_, err = tx.Exec(ctx, string(truncateFile))
	require.NoError(t, err)
	// Run generated ripoff.
	err = RunRipoff(ctx, tx, ripoffFile)
	require.NoError(t, err)
	// Try to verify that the number of generated rows matches the ripoff.
	tableCount := map[string]int{}
	for rowId := range ripoffFile.Rows {
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
}

func TestRipoffExport(t *testing.T) {
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
	dir := path.Join(path.Dir(filename), "testdata", "export")
	dirEntry, err := os.ReadDir(dir)
	require.NoError(t, err)

	for _, e := range dirEntry {
		if !e.IsDir() {
			continue
		}
		tx, err := conn.Begin(ctx)
		require.NoError(t, err)
		runExportTestData(t, ctx, tx, path.Join(dir, e.Name()))
		err = tx.Rollback(ctx)
		require.NoError(t, err)
	}
}
