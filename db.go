package ripoff

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"log/slog"
	"math/rand"
	"reflect"
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
			return fmt.Errorf("error when running query %s, %v", query, err)
		}
	}

	return nil
}

var valueFuncRegex = regexp.MustCompile(`([a-zA-Z]+)\((\S+)\)$`)
var referenceRegex = regexp.MustCompile(`^[a-zA-Z0-9_]+:`)

// Creates a map of functions from gofakeit using reflection.
// Fairly gross, but the alternative is listing every "useful" function from them which seems worse.
func fakerFuncs(f *gofakeit.Faker) map[string]func() string {
	funcs := map[string]func() string{}

	v := reflect.ValueOf(f)
	for i := 0; i < v.NumMethod(); i++ {
		methodType := v.Type().Method(i).Type
		// Verify that this is a method that takes no args and returns a single string.
		if methodType.NumOut() != 1 || methodType.NumIn() != 1 || methodType.Out(0).String() != "string" {
			continue
		}

		funcs[v.Type().Method(i).Name] = v.Method(i).Interface().(func() string)
	}

	return funcs
}

func uppercase(s string) string {
	if len(s) < 2 {
		return strings.ToUpper(s)
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func prepareValue(rawValue string) (string, bool, error) {
	valueFuncMatches := valueFuncRegex.FindStringSubmatch(rawValue)
	if len(valueFuncMatches) != 3 {
		return rawValue, false, nil
	}
	// Assume that if a valueFunc is prefixed with a table name, it's a primary/foreign key.
	addEdge := referenceRegex.MatchString(rawValue)
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
			return "", false, err
		}
		return randomId.String(), addEdge, nil
	case "int":
		return fmt.Sprint(randSeed.Int()), addEdge, nil
	case "literal":
		return value, addEdge, nil
	}

	// Check for methods provided by gofakeit.
	faker := gofakeit.NewFaker(randSeed, true)
	funcs := fakerFuncs(faker)
	fakerFunc, funcExists := funcs[uppercase(methodName)]
	if funcExists {
		return fakerFunc(), addEdge, nil
	}

	return "", false, fmt.Errorf("valueFunc does not exist: %s(%s)", methodName, value)
}

// Returns a sorted array of queries to run based on a given ripoff file.
func buildQueriesForRipoff(totalRipoff RipoffFile) ([]string, error) {
	dependencyGraph := graph.New(graph.StringHash, graph.Directed(), graph.Acyclic())
	queries := map[string]string{}

	// Add vertexes first.
	for rowId := range totalRipoff.Rows {
		err := dependencyGraph.AddVertex(rowId)
		if err != nil {
			return []string{}, err
		}
	}

	for rowId, row := range totalRipoff.Rows {
		parts := strings.Split(rowId, ":")
		if len(parts) < 2 {
			return []string{}, fmt.Errorf("invalid id: %s", rowId)
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
			// This is a normal/real column.
			columns = append(columns, pq.QuoteIdentifier(column))
			valuePrepared, addEdge, err := prepareValue(value)
			if err != nil {
				return []string{}, err
			}
			// Don't add edges to and from the same row.
			if addEdge && rowId != value {
				err = dependencyGraph.AddEdge(rowId, value)
				if err != nil {
					return []string{}, err
				}
			}
			// Assume this column is the primary key.
			if rowId == value {
				onConflictColumn = pq.QuoteIdentifier(column)
			}
			values = append(values, pq.QuoteLiteral(valuePrepared))
			setStatements = append(setStatements, fmt.Sprintf("%s = %s", pq.QuoteIdentifier(column), pq.QuoteLiteral(valuePrepared)))
		}

		if onConflictColumn == "" {
			return []string{}, fmt.Errorf("cannot determine column to conflict with for: %s", rowId)
		}

		// Extremely smart query builder.
		query := fmt.Sprintf(
			`INSERT INTO %s (%s)
	VALUES (%s)
	ON CONFLICT (%s)
	DO UPDATE SET %s;`,
			pq.QuoteIdentifier(table),
			strings.Join(columns, ","),
			strings.Join(values, ","),
			onConflictColumn,
			strings.Join(setStatements, ","),
		)
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
