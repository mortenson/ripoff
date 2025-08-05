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

// TestExcludeColumnsFlag tests that the exclude-columns flag properly excludes columns from export
func TestExcludeColumnsFlag(t *testing.T) {
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

	// Create test tables with timestamped columns
	_, err = tx.Exec(ctx, `
		CREATE TABLE users (
			id SERIAL PRIMARY KEY,
			name TEXT,
			email TEXT,
			created_at TIMESTAMP DEFAULT NOW(),
			updated_at TIMESTAMP DEFAULT NOW()
		);
		
		CREATE TABLE posts (
			id SERIAL PRIMARY KEY,
			title TEXT,
			content TEXT,
			user_id INTEGER REFERENCES users(id),
			created_at TIMESTAMP DEFAULT NOW(),
			updated_at TIMESTAMP DEFAULT NOW()
		);
		
		INSERT INTO users (name, email) VALUES ('Alice', 'alice@example.com'), ('Bob', 'bob@example.com');
		INSERT INTO posts (title, content, user_id) VALUES 
			('Post 1', 'Content 1', 1), 
			('Post 2', 'Content 2', 1),
			('Post 3', 'Content 3', 2);
	`)
	require.NoError(t, err)

	// Test 1: Exclude global columns (created_at, updated_at)
	t.Run("Global column exclusion", func(t *testing.T) {
		ripoffFile, err := ExportToRipoff(ctx, tx, []string{}, []string{"created_at", "updated_at"})
		require.NoError(t, err)

		// Verify that no row contains created_at or updated_at columns
		for rowId, row := range ripoffFile.Rows {
			_, hasCreatedAt := row["created_at"]
			_, hasUpdatedAt := row["updated_at"]
			require.False(t, hasCreatedAt, "Row %s should not have created_at column", rowId)
			require.False(t, hasUpdatedAt, "Row %s should not have updated_at column", rowId)

			// But should still have other columns
			tableName := strings.Split(rowId, ":")[0]
			switch tableName {
			case "users":
				_, hasName := row["name"]
				_, hasEmail := row["email"]
				require.True(t, hasName, "Row %s should have name column", rowId)
				require.True(t, hasEmail, "Row %s should have email column", rowId)
			case "posts":
				_, hasTitle := row["title"]
				_, hasContent := row["content"]
				require.True(t, hasTitle, "Row %s should have title column", rowId)
				require.True(t, hasContent, "Row %s should have content column", rowId)
			}
		}
	})

	// Test 2: Exclude table-specific column (users.created_at) - shared column name
	t.Run("Table-specific column exclusion", func(t *testing.T) {
		ripoffFile, err := ExportToRipoff(ctx, tx, []string{}, []string{"users.created_at"})
		require.NoError(t, err)

		// Verify that user rows don't have created_at but post rows still have created_at
		for rowId, row := range ripoffFile.Rows {
			tableName := strings.Split(rowId, ":")[0]
			switch tableName {
			case "users":
				_, hasCreatedAt := row["created_at"]
				require.False(t, hasCreatedAt, "User row %s should not have created_at column", rowId)
				// Should still have other columns
				_, hasName := row["name"]
				_, hasEmail := row["email"]
				require.True(t, hasName, "User row %s should have name column", rowId)
				require.True(t, hasEmail, "User row %s should have email column", rowId)
			case "posts":
				// Posts should have created_at since only users.created_at was excluded
				_, hasTitle := row["title"]
				_, hasCreatedAt := row["created_at"]
				require.True(t, hasTitle, "Post row %s should have title column", rowId)
				require.True(t, hasCreatedAt, "Post row %s should have created_at column", rowId)
			}
		}
	})

	// Test 3: Combine both exclusion types
	t.Run("Combined exclusions", func(t *testing.T) {
		ripoffFile, err := ExportToRipoff(ctx, tx, []string{}, []string{"created_at", "users.email"})
		require.NoError(t, err)

		// Verify exclusions are applied correctly
		for rowId, row := range ripoffFile.Rows {
			tableName := strings.Split(rowId, ":")[0]

			// No row should have created_at (global exclusion)
			_, hasCreatedAt := row["created_at"]
			require.False(t, hasCreatedAt, "Row %s should not have created_at column", rowId)

			switch tableName {
			case "users":
				// Users should not have email (table-specific exclusion)
				_, hasEmail := row["email"]
				require.False(t, hasEmail, "User row %s should not have email column", rowId)
				// But should have name and updated_at
				_, hasName := row["name"]
				_, hasUpdatedAt := row["updated_at"]
				require.True(t, hasName, "User row %s should have name column", rowId)
				require.True(t, hasUpdatedAt, "User row %s should have updated_at column", rowId)
			case "posts":
				// Posts should have all columns except created_at
				_, hasTitle := row["title"]
				_, hasUpdatedAt := row["updated_at"]
				require.True(t, hasTitle, "Post row %s should have title column", rowId)
				require.True(t, hasUpdatedAt, "Post row %s should have updated_at column", rowId)
			}
		}
	})
}
