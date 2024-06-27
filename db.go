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
	"slices"
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

// Copied from gofakeit
func fakerFuncs(f *gofakeit.Faker) map[string]func() string {
	funcs := map[string]func() string{}

	v := reflect.ValueOf(f)

	templateExclusion := []string{
		"RandomMapKey",
		"SQL",
		"Template",
	}

	for i := 0; i < v.NumMethod(); i++ {
		if slices.Index(templateExclusion, v.Type().Method(i).Name) != -1 {
			continue
		}

		// Verify that this is a method that takes a string and returns a string.
		if v.Type().Method(i).Type.NumOut() != 1 || v.Type().Method(i).Type.NumIn() != 1 || v.Type().Method(i).Type.Out(0).String() != "string" {
			continue
		}

		funcs[v.Type().Method(i).Name] = v.Method(i).Interface().(func() string)
	}

	return funcs
}

func prepareValue(rawValue string) (string, bool, error) {
	valueFuncMatches := valueFuncRegex.FindStringSubmatch(rawValue)
	if len(valueFuncMatches) != 3 {
		return rawValue, false, nil
	}
	addEdge := referenceRegex.MatchString(rawValue)
	kind := valueFuncMatches[1]
	value := valueFuncMatches[2]
	h := sha256.New()
	h.Write([]byte(value))
	hashBytes := h.Sum(nil)
	randSeed := rand.New(rand.NewSource(int64(binary.BigEndian.Uint64(hashBytes))))
	// It's one of ours.
	switch kind {
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

	faker := gofakeit.NewFaker(randSeed, true)
	funcs := fakerFuncs(faker)
	fakerFunc, funcExists := funcs[strings.ToUpper(kind[:1])+kind[1:]]
	if funcExists {
		return fakerFunc(), addEdge, nil
	}

	return "", false, fmt.Errorf("Magic ID kind does not exist: %s(%s)", kind, value)
}

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
			// Rows can explicitly mark what columns they should conflict with, in (hopefully rare) cases.
			if column == "~conflict" {
				columnParts := strings.Split(value, ",")
				for i, columnPart := range columnParts {
					columnParts[i] = pq.QuoteIdentifier(strings.TrimSpace(columnPart))
				}
				onConflictColumn = strings.Join(columnParts, ", ")
				continue
			}
			columns = append(columns, pq.QuoteIdentifier(column))
			valuePrepared, addEdge, err := prepareValue(value)
			if err != nil {
				return []string{}, err
			}
			if addEdge && rowId != value {
				err = dependencyGraph.AddEdge(rowId, value)
				if err != nil {
					return []string{}, err
				}
			}
			if rowId == value {
				onConflictColumn = pq.QuoteIdentifier(column)
			}
			values = append(values, pq.QuoteLiteral(valuePrepared))
			setStatements = append(setStatements, fmt.Sprintf("%s = %s", pq.QuoteIdentifier(column), pq.QuoteLiteral(valuePrepared)))
		}

		if onConflictColumn == "" {
			return []string{}, fmt.Errorf("cannot determine column to conflict with for: %s", rowId)
		}

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
