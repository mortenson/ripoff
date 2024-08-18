package ripoff

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"log/slog"
	"math/rand"
	"regexp"
	"strings"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/dominikbraun/graph"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/lib/pq"
)

// Runs ripoff from start to finish, without committing the transaction.
func RunRipoff(ctx context.Context, tx pgx.Tx, totalRipoff RipoffFile) error {
	primaryKeys, err := getPrimaryKeys(ctx, tx)
	if err != nil {
		return err
	}

	queries, err := buildQueriesForRipoff(primaryKeys, totalRipoff)
	if err != nil {
		return err
	}

	for _, query := range queries {
		slog.Debug(query)
		_, err = tx.Exec(ctx, query)
		if err != nil {
			return fmt.Errorf("error when running query %s, %w", query, err)
		}
	}

	return nil
}

const primaryKeysQuery = `
SELECT STRING_AGG(c.column_name, '|'), tc.table_name
FROM information_schema.table_constraints tc 
JOIN information_schema.constraint_column_usage AS ccu USING (constraint_schema, constraint_name) 
JOIN information_schema.columns AS c ON c.table_schema = tc.constraint_schema
  AND tc.table_name = c.table_name AND ccu.column_name = c.column_name
WHERE constraint_type = 'PRIMARY KEY'
AND tc.table_schema = 'public'
GROUP BY tc.table_name;
`

type PrimaryKeysResult map[string][]string

func getPrimaryKeys(ctx context.Context, tx pgx.Tx) (PrimaryKeysResult, error) {
	rows, err := tx.Query(ctx, primaryKeysQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	allPrimaryKeys := PrimaryKeysResult{}

	for rows.Next() {
		var primaryKeys string
		var tableName string
		err = rows.Scan(&primaryKeys, &tableName)
		if err != nil {
			return nil, err
		}
		allPrimaryKeys[tableName] = strings.Split(primaryKeys, "|")
	}
	return allPrimaryKeys, nil
}

const enumValuesQuery = `
SELECT STRING_AGG(e.enumlabel, '|'), t.typname
FROM pg_type t
JOIN pg_enum e ON t.oid = e.enumtypid  
JOIN pg_catalog.pg_namespace n ON n.oid = t.typnamespace
WHERE n.nspname = 'public'
GROUP BY t.typname;
`

type EnumValuesResult map[string][]string

func GetEnumValues(ctx context.Context, tx pgx.Tx) (EnumValuesResult, error) {
	rows, err := tx.Query(ctx, enumValuesQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	allEnumValues := EnumValuesResult{}

	for rows.Next() {
		var primaryKeys string
		var tableName string
		err = rows.Scan(&primaryKeys, &tableName)
		if err != nil {
			return nil, err
		}
		allEnumValues[tableName] = strings.Split(primaryKeys, "|")
	}
	return allEnumValues, nil
}

var valueFuncRegex = regexp.MustCompile(`([a-zA-Z]+)\((.*)\)$`)
var referenceRegex = regexp.MustCompile(`^[a-zA-Z0-9_]+:`)

func prepareValue(rawValue string) (string, error) {
	valueFuncMatches := valueFuncRegex.FindStringSubmatch(rawValue)
	if len(valueFuncMatches) != 3 {
		return rawValue, nil
	}
	methodName := valueFuncMatches[1]
	value := valueFuncMatches[2]

	// Create a new random seed based on a sha256 hash of the value.
	h := sha256.New()
	h.Write([]byte(value))
	hashBytes := h.Sum(nil)
	randSeed := rand.New(rand.NewSource(int64(binary.BigEndian.Uint64(hashBytes))))

	// Check for methods provided by ripoff.
	switch methodName {
	case "uuid":
		randomId, err := uuid.NewRandomFromReader(randSeed)
		if err != nil {
			return "", err
		}
		return randomId.String(), nil
	case "int":
		return fmt.Sprint(randSeed.Int()), nil
	case "literal":
		return value, nil
	}

	// Assume the user meant to call a gofakeit.Faker method.
	faker := gofakeit.NewFaker(randSeed, true)
	fakerResult, err := callFakerMethod(methodName, faker)
	if err != nil {
		return "", err
	}

	return fakerResult, nil
}

func buildQueryForRow(primaryKeys PrimaryKeysResult, rowId string, row Row, dependencyGraph graph.Graph[string, string]) (string, error) {
	parts := strings.Split(rowId, ":")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid id: %s", rowId)
	}
	table := parts[0]
	primaryKeysForTable, hasPrimaryKeysForTable := primaryKeys[table]

	columns := []string{}
	values := []string{}
	setStatements := []string{}

	onConflictColumn := ""
	if hasPrimaryKeysForTable {
		quotedKeys := make([]string, len(primaryKeysForTable))
		for i, columnPart := range primaryKeysForTable {
			quotedKeys[i] = pq.QuoteIdentifier(strings.TrimSpace(columnPart))
		}
		onConflictColumn = strings.Join(quotedKeys, ", ")
		// For UX reasons, you don't have to define primary key columns (ex: id), since we have the map key already.
		if len(primaryKeysForTable) == 1 {
			column := primaryKeysForTable[0]
			_, hasPrimaryColumn := row[column]
			if !hasPrimaryColumn {
				row[column] = rowId
			}
		}
	}

	for column, valueRaw := range row {
		// Backwards compatability weirdness.
		if column == "~conflict" {
			continue
		}

		// Technically we allow more than strings in ripoff files for templating purposes,
		// but full support (ex: escaping arrays, what to do with maps, etc.) is quite hard so tabling that for now.
		value := fmt.Sprint(valueRaw)

		// Assume that if a valueFunc is prefixed with a table name, it's a primary/foreign key.
		addEdge := referenceRegex.MatchString(value)
		// Don't add edges to and from the same row.
		if addEdge && rowId != value {
			err := dependencyGraph.AddEdge(rowId, value)
			if err != nil {
				return "", err
			}
		}

		columns = append(columns, pq.QuoteIdentifier(column))
		valuePrepared, err := prepareValue(value)
		if err != nil {
			return "", err
		}
		// Assume this column is the primary key.
		if rowId == value && onConflictColumn == "" {
			onConflictColumn = pq.QuoteIdentifier(column)
		}
		values = append(values, pq.QuoteLiteral(valuePrepared))
		setStatements = append(setStatements, fmt.Sprintf("%s = %s", pq.QuoteIdentifier(column), pq.QuoteLiteral(valuePrepared)))
	}

	if onConflictColumn == "" {
		return "", fmt.Errorf("cannot determine column to conflict with for: %s, saw %s", rowId, row)
	}

	// Extremely smart query builder.
	return fmt.Sprintf(
		`INSERT INTO %s (%s)
	VALUES (%s)
	ON CONFLICT (%s)
	DO UPDATE SET %s;`,
		pq.QuoteIdentifier(table),
		strings.Join(columns, ","),
		strings.Join(values, ","),
		onConflictColumn,
		strings.Join(setStatements, ","),
	), nil
}

// Returns a sorted array of queries to run based on a given ripoff file.
func buildQueriesForRipoff(primaryKeys PrimaryKeysResult, totalRipoff RipoffFile) ([]string, error) {
	dependencyGraph := graph.New(graph.StringHash, graph.Directed(), graph.Acyclic())
	queries := map[string]string{}

	// Add vertexes first, since rows can be in any order.
	for rowId := range totalRipoff.Rows {
		err := dependencyGraph.AddVertex(rowId)
		if err != nil {
			return []string{}, err
		}
	}

	// Build queries.
	for rowId, row := range totalRipoff.Rows {
		query, err := buildQueryForRow(primaryKeys, rowId, row, dependencyGraph)
		if err != nil {
			return []string{}, err
		}
		queries[rowId] = query
	}

	// Sort and reverse the graph, so queries are in order of least (hopefully none) to most dependencies.
	ordered, _ := graph.TopologicalSort(dependencyGraph)
	sortedQueries := []string{}
	for i := len(ordered) - 1; i >= 0; i-- {
		query, ok := queries[ordered[i]]
		if !ok {
			return []string{}, fmt.Errorf("no query found for %s", ordered[i])
		}
		sortedQueries = append(sortedQueries, query)
	}
	return sortedQueries, nil
}
