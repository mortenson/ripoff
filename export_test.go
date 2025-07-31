package ripoff

import (
	"context"
	"fmt"
	"os"
	"path"
	"runtime"
	"strings"
	"testing"
	"time"

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
	ripoffFile, err := ExportToRipoff(ctx, tx, []string{}, []string{})
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

// TestExcludeFlag tests that the exclude flag properly excludes tables from export
func TestExcludeFlag(t *testing.T) {
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

	// Start a transaction that we'll roll back at the end
	tx, err := conn.Begin(ctx)
	require.NoError(t, err)
	defer func() {
		err := tx.Rollback(ctx)
		require.NoError(t, err)
	}()

	// Create three tables - one we'll include and two we'll exclude
	_, err = tx.Exec(ctx, `
		CREATE TABLE include_me (
			id SERIAL PRIMARY KEY,
			name TEXT
		);
		
		CREATE TABLE exclude_me (
			id SERIAL PRIMARY KEY,
			description TEXT
		);
		
		CREATE TABLE also_exclude_me (
			id SERIAL PRIMARY KEY,
			data TEXT
		);
		
		INSERT INTO include_me (name) VALUES ('test data 1'), ('test data 2');
		INSERT INTO exclude_me (description) VALUES ('should not appear'), ('also should not appear');
		INSERT INTO also_exclude_me (data) VALUES ('should not appear'), ('also should not appear'), ('third row');
	`)
	require.NoError(t, err)

	// Test 1: Exclude a single table
	t.Run("Single exclude", func(t *testing.T) {
		ripoffFile, err := ExportToRipoff(ctx, tx, []string{"exclude_me"}, []string{})
		require.NoError(t, err)

		// Verify that ripoffFile.Rows contains rows from include_me but not exclude_me
		hasIncludeMe := false
		hasExcludeMe := false
		hasAlsoExcludeMe := false

		for rowId := range ripoffFile.Rows {
			tableName := strings.Split(rowId, ":")
			if len(tableName) > 0 {
				if tableName[0] == "include_me" {
					hasIncludeMe = true
				}
				if tableName[0] == "exclude_me" {
					hasExcludeMe = true
				}
				if tableName[0] == "also_exclude_me" {
					hasAlsoExcludeMe = true
				}
			}
		}

		// We should have rows from include_me
		require.True(t, hasIncludeMe, "Expected to find rows from include_me table")
		
		// We should NOT have rows from exclude_me
		require.False(t, hasExcludeMe, "Found rows from exclude_me table even though it was excluded")
		
		// We should have rows from also_exclude_me (since it wasn't excluded in this test)
		require.True(t, hasAlsoExcludeMe, "Expected to find rows from also_exclude_me table")

		// Count rows to make sure we have the right number
		includeCount := 0
		excludeCount := 0
		alsoExcludeCount := 0

		for rowId := range ripoffFile.Rows {
			tableName := strings.Split(rowId, ":")
			if len(tableName) > 0 {
				if tableName[0] == "include_me" {
					includeCount++
				}
				if tableName[0] == "exclude_me" {
					excludeCount++
				}
				if tableName[0] == "also_exclude_me" {
					alsoExcludeCount++
				}
			}
		}

		require.Equal(t, 2, includeCount, "Expected 2 rows from include_me table")
		require.Equal(t, 0, excludeCount, "Expected 0 rows from exclude_me table")
		require.Equal(t, 3, alsoExcludeCount, "Expected 3 rows from also_exclude_me table")
	})

	// Test 2: Exclude multiple tables
	t.Run("Multiple excludes", func(t *testing.T) {
		ripoffFile, err := ExportToRipoff(ctx, tx, []string{"exclude_me", "also_exclude_me"}, []string{})
		require.NoError(t, err)

		// Verify that ripoffFile.Rows contains rows from include_me but not from the excluded tables
		hasIncludeMe := false
		hasExcludeMe := false
		hasAlsoExcludeMe := false

		for rowId := range ripoffFile.Rows {
			tableName := strings.Split(rowId, ":")
			if len(tableName) > 0 {
				if tableName[0] == "include_me" {
					hasIncludeMe = true
				}
				if tableName[0] == "exclude_me" {
					hasExcludeMe = true
				}
				if tableName[0] == "also_exclude_me" {
					hasAlsoExcludeMe = true
				}
			}
		}

		// We should have rows from include_me
		require.True(t, hasIncludeMe, "Expected to find rows from include_me table")
		
		// We should NOT have rows from exclude_me
		require.False(t, hasExcludeMe, "Found rows from exclude_me table even though it was excluded")
		
		// We should NOT have rows from also_exclude_me
		require.False(t, hasAlsoExcludeMe, "Found rows from also_exclude_me table even though it was excluded")

		// Count rows to make sure we have the right number
		includeCount := 0
		excludeCount := 0
		alsoExcludeCount := 0

		for rowId := range ripoffFile.Rows {
			tableName := strings.Split(rowId, ":")
			if len(tableName) > 0 {
				if tableName[0] == "include_me" {
					includeCount++
				}
				if tableName[0] == "exclude_me" {
					excludeCount++
				}
				if tableName[0] == "also_exclude_me" {
					alsoExcludeCount++
				}
			}
		}

		require.Equal(t, 2, includeCount, "Expected 2 rows from include_me table")
		require.Equal(t, 0, excludeCount, "Expected 0 rows from exclude_me table")
		require.Equal(t, 0, alsoExcludeCount, "Expected 0 rows from also_exclude_me table")
	})
}

// TestIgnoreOnUpdateFlag tests that the ignore-on-update flag properly marks columns
func TestIgnoreOnUpdateFlag(t *testing.T) {
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

	// Start a transaction that we'll roll back at the end
	tx, err := conn.Begin(ctx)
	require.NoError(t, err)
	defer func() {
		err := tx.Rollback(ctx)
		require.NoError(t, err)
	}()

	// Create a table with columns including created_at and updated_at
	_, err = tx.Exec(ctx, `
		CREATE TABLE test_table (
			id SERIAL PRIMARY KEY,
			name TEXT,
			created_at TIMESTAMP DEFAULT NOW(),
			updated_at TIMESTAMP DEFAULT NOW(),
			description TEXT
		);
		
		INSERT INTO test_table (name, description) VALUES 
			('test 1', 'first test'),
			('test 2', 'second test');
	`)
	require.NoError(t, err)

	// Test 1: Ignore created_at on update
	t.Run("Ignore created_at on update", func(t *testing.T) {
		ripoffFile, err := ExportToRipoff(ctx, tx, []string{}, []string{"created_at"})
		require.NoError(t, err)

		// Verify that ripoffFile.Rows contains rows with created_at but marked for ignore on update
		rowCount := 0
		for rowId, row := range ripoffFile.Rows {
			if strings.HasPrefix(rowId, "test_table:") {
				rowCount++
				// Should have all columns including created_at
				_, hasName := row["name"]
				_, hasDescription := row["description"]
				_, hasCreatedAt := row["created_at"]
				_, hasUpdatedAt := row["updated_at"]

				require.True(t, hasName, "Expected name column to be present")
				require.True(t, hasDescription, "Expected description column to be present")
				require.True(t, hasCreatedAt, "Expected created_at column to be present")
				require.True(t, hasUpdatedAt, "Expected updated_at column to be present")

				// Should have ignore-on-update metadata
				ignoreOnUpdate, hasIgnoreOnUpdate := row["~ignore_on_update"]
				require.True(t, hasIgnoreOnUpdate, "Expected ~ignore_on_update metadata to be present")
				
				ignoreList, ok := ignoreOnUpdate.([]string)
				require.True(t, ok, "Expected ~ignore_on_update to be a slice of strings")
				require.Equal(t, []string{"created_at"}, ignoreList, "Expected created_at to be in ignore list")
			}
		}
		require.Equal(t, 2, rowCount, "Expected 2 rows from test_table")
	})

	// Test 2: Ignore multiple columns on update
	t.Run("Ignore multiple columns on update", func(t *testing.T) {
		ripoffFile, err := ExportToRipoff(ctx, tx, []string{}, []string{"created_at", "updated_at"})
		require.NoError(t, err)

		// Verify that ripoffFile.Rows contains rows with both timestamp columns marked for ignore
		rowCount := 0
		for rowId, row := range ripoffFile.Rows {
			if strings.HasPrefix(rowId, "test_table:") {
				rowCount++
				// Should have all columns including both timestamp columns
				_, hasName := row["name"]
				_, hasDescription := row["description"]
				_, hasCreatedAt := row["created_at"]
				_, hasUpdatedAt := row["updated_at"]

				require.True(t, hasName, "Expected name column to be present")
				require.True(t, hasDescription, "Expected description column to be present")
				require.True(t, hasCreatedAt, "Expected created_at column to be present")
				require.True(t, hasUpdatedAt, "Expected updated_at column to be present")

				// Should have ignore-on-update metadata with both columns
				ignoreOnUpdate, hasIgnoreOnUpdate := row["~ignore_on_update"]
				require.True(t, hasIgnoreOnUpdate, "Expected ~ignore_on_update metadata to be present")
				
				ignoreList, ok := ignoreOnUpdate.([]string)
				require.True(t, ok, "Expected ~ignore_on_update to be a slice of strings")
				require.ElementsMatch(t, []string{"created_at", "updated_at"}, ignoreList, "Expected both timestamp columns to be in ignore list")
			}
		}
		require.Equal(t, 2, rowCount, "Expected 2 rows from test_table")
	})

	// Test 3: No ignore-on-update metadata when no columns specified
	t.Run("No ignore metadata when no columns specified", func(t *testing.T) {
		ripoffFile, err := ExportToRipoff(ctx, tx, []string{}, []string{})
		require.NoError(t, err)

		// Verify that ripoffFile.Rows contains rows without ignore-on-update metadata
		rowCount := 0
		for rowId, row := range ripoffFile.Rows {
			if strings.HasPrefix(rowId, "test_table:") {
				rowCount++
				// Should NOT have ignore-on-update metadata
				_, hasIgnoreOnUpdate := row["~ignore_on_update"]
				require.False(t, hasIgnoreOnUpdate, "Expected no ~ignore_on_update metadata when no columns specified")
			}
		}
		require.Equal(t, 2, rowCount, "Expected 2 rows from test_table")
	})

	// Test 4: Verify ignore-on-update preserves existing values during re-export
	t.Run("Preserve existing values on re-export", func(t *testing.T) {
		// First export with ignore-on-update flags
		ripoffFile1, err := ExportToRipoff(ctx, tx, []string{}, []string{"created_at", "updated_at"})
		require.NoError(t, err)

		// Get the original timestamp values from first export
		var originalCreatedAt, originalUpdatedAt string
		for rowId, row := range ripoffFile1.Rows {
			if strings.HasPrefix(rowId, "test_table:") {
				originalCreatedAt = row["created_at"].(string)
				originalUpdatedAt = row["updated_at"].(string)
				break
			}
		}

		// Simulate time passing and database changes
		// Add a small delay to ensure different timestamps
		time.Sleep(10 * time.Millisecond)
		// Update the database with new timestamp values
		_, err = tx.Exec(ctx, "UPDATE test_table SET updated_at = NOW() + INTERVAL '1 second' WHERE id = 1")
		require.NoError(t, err)

		// Second export with same ignore-on-update flags
		// This should preserve the original timestamp values, not use the new database values
		ripoffFile2, err := ExportToRipoffWithExisting(ctx, tx, []string{}, []string{"created_at", "updated_at"}, &ripoffFile1)
		require.NoError(t, err)

		// Check that the timestamp values are preserved from first export
		for rowId, row := range ripoffFile2.Rows {
			if strings.HasPrefix(rowId, "test_table:") {
				currentCreatedAt := row["created_at"].(string)
				currentUpdatedAt := row["updated_at"].(string)
				
				t.Logf("Original created_at: %s, Current created_at: %s", originalCreatedAt, currentCreatedAt)
				t.Logf("Original updated_at: %s, Current updated_at: %s", originalUpdatedAt, currentUpdatedAt)
				
				// These assertions should pass with the new logic
				require.Equal(t, originalCreatedAt, currentCreatedAt, "created_at should be preserved from original export")
				require.Equal(t, originalUpdatedAt, currentUpdatedAt, "updated_at should be preserved from original export")
				
				// Verify the metadata is still present
				ignoreOnUpdate, hasIgnoreOnUpdate := row["~ignore_on_update"]
				require.True(t, hasIgnoreOnUpdate, "Expected ~ignore_on_update metadata to be present")
				ignoreList, ok := ignoreOnUpdate.([]string)
				require.True(t, ok, "Expected ~ignore_on_update to be a slice of strings")
				require.ElementsMatch(t, []string{"created_at", "updated_at"}, ignoreList, "Expected both timestamps in ignore list")
				break
			}
		}
	})
}
