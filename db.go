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
	queries, err := buildQueriesForRipoff(totalRipoff)
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

var valueFuncRegex = regexp.MustCompile(`([a-zA-Z]+)\((\S+)\)$`)
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

func buildQueryForRow(rowId string, row Row, dependencyGraph graph.Graph[string, string]) (string, error) {
	parts := strings.Split(rowId, ":")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid id: %s", rowId)
	}
	table := parts[0]

	columns := []string{}
	values := []string{}
	setStatements := []string{}
	onConflictColumn := ""
	for column, value := range row {
		// Rows can explicitly mark what columns they should conflict with, in cases like composite primary keys.
		if column == "~conflict" {
			// Really novice way of escaping these.
			columnParts := strings.Split(value, ",")
			for i, columnPart := range columnParts {
				columnParts[i] = pq.QuoteIdentifier(strings.TrimSpace(columnPart))
			}
			onConflictColumn = strings.Join(columnParts, ", ")
			continue
		}

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
		if rowId == value {
			onConflictColumn = pq.QuoteIdentifier(column)
		}
		values = append(values, pq.QuoteLiteral(valuePrepared))
		setStatements = append(setStatements, fmt.Sprintf("%s = %s", pq.QuoteIdentifier(column), pq.QuoteLiteral(valuePrepared)))
	}

	if onConflictColumn == "" {
		return "", fmt.Errorf("cannot determine column to conflict with for: %s", rowId)
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
func buildQueriesForRipoff(totalRipoff RipoffFile) ([]string, error) {
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
		query, err := buildQueryForRow(rowId, row, dependencyGraph)
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
