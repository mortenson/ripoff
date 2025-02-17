package ripoff

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"log/slog"
	"math/rand"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/brianvoe/gofakeit/v7"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/lib/pq"
	"github.com/tj/go-naturaldate"
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
var referenceRegex = regexp.MustCompile(`^[a-zA-Z0-9_]+:[a-zA-Z]+\(`)

func prepareValue(rawValue string) (string, error) {
	valueFuncMatches := valueFuncRegex.FindStringSubmatch(rawValue)
	if len(valueFuncMatches) != 3 {
		return rawValue, nil
	}
	methodName := valueFuncMatches[1]
	value := valueFuncMatches[2]
	valueParts := strings.Split(strings.ReplaceAll(" ", "", valueFuncMatches[2]), ",")

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
	case "naturalDate":
		parsed, err := naturaldate.Parse(value, time.Now())
		return parsed.Format(time.RFC3339), err
	}

	// Assume the user meant to call a gofakeit.Faker method.
	faker := gofakeit.NewFaker(randSeed, true)
	fakerResult, err := callFakerMethod(methodName, faker, valueParts...)
	if err != nil {
		return "", err
	}

	return fakerResult, nil
}

func buildQueryForRow(primaryKeys PrimaryKeysResult, rowId string, row Row, dependencyGraph map[string][]string) (string, error) {
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
		// Explicit dependencies, for foreign keys to non-primary keys.
		if column == "~dependencies" {
			dependencies := []string{}
			switch v := valueRaw.(type) {
			// Coming from yaml
			case []interface{}:
				for _, curr := range v {
					dependencies = append(dependencies, curr.(string))
				}
			// Coming from Go, probably a test
			case []string:
				dependencies = v
			default:
				return "", fmt.Errorf("cannot parse ~dependencies value in row %s", rowId)
			}
			dependencyGraph[rowId] = append(dependencyGraph[rowId], dependencies...)
			dependencyGraph[rowId] = slices.Compact(dependencyGraph[rowId])
			continue
		}

		// Technically we allow more than null strings in ripoff files for templating purposes,
		// but full support (ex: escaping arrays, what to do with maps, etc.) is quite hard so tabling that for now.
		if valueRaw == nil {
			columns = append(columns, pq.QuoteIdentifier(column))
			values = append(values, "NULL")
			setStatements = append(setStatements, fmt.Sprintf("%s = %s", pq.QuoteIdentifier(column), "NULL"))
		} else {
			value := fmt.Sprint(valueRaw)

			// Assume that if a valueFunc is prefixed with a table name, it's a primary/foreign key.
			addEdge := referenceRegex.MatchString(value)
			// Don't add edges to and from the same row.
			if addEdge && rowId != value {
				dependencyGraph[rowId] = append(dependencyGraph[rowId], value)
				dependencyGraph[rowId] = slices.Compact(dependencyGraph[rowId])
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
	dependencyGraph := map[string][]string{}
	queries := map[string]string{}

	// Add vertexes first, since rows can be in any order.
	for rowId := range totalRipoff.Rows {
		dependencyGraph[rowId] = []string{}
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
	ordered, err := topologicalSort(dependencyGraph)
	if err != nil {
		return []string{}, err
	}
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

const columnsWithForeignKeysQuery = `
select col.table_name as table,
       col.column_name,
       COALESCE(rel.table_name, '') as primary_table,
       COALESCE(rel.column_name, '') as primary_column,
			 COALESCE(kcu.constraint_name, '')
from information_schema.columns col
left join (select kcu.constraint_schema, 
                  kcu.constraint_name, 
                  kcu.table_schema,
                  kcu.table_name, 
                  kcu.column_name, 
                  kcu.ordinal_position,
                  kcu.position_in_unique_constraint
           from information_schema.key_column_usage kcu
           join information_schema.table_constraints tco
                on kcu.constraint_schema = tco.constraint_schema
                and kcu.constraint_name = tco.constraint_name
                and tco.constraint_type = 'FOREIGN KEY'
          ) as kcu
          on col.table_schema = kcu.table_schema
          and col.table_name = kcu.table_name
          and col.column_name = kcu.column_name
left join information_schema.referential_constraints rco
          on rco.constraint_name = kcu.constraint_name
          and rco.constraint_schema = kcu.table_schema
left join information_schema.key_column_usage rel
          on rco.unique_constraint_name = rel.constraint_name
          and rco.unique_constraint_schema = rel.constraint_schema
          and rel.ordinal_position = kcu.position_in_unique_constraint
where col.table_schema = 'public';
`

type ForeignKey struct {
	ToTable          string
	ColumnConditions [][2]string
}

type ForeignKeyResultTable struct {
	Columns []string
	// Constraint -> Fkey
	ForeignKeys map[string]*ForeignKey
}

// Map of table name to foreign keys.
type ForeignKeysResult map[string]*ForeignKeyResultTable

func getForeignKeysResult(ctx context.Context, conn pgx.Tx) (ForeignKeysResult, error) {
	rows, err := conn.Query(ctx, columnsWithForeignKeysQuery)
	if err != nil {
		return ForeignKeysResult{}, err
	}
	defer rows.Close()

	result := ForeignKeysResult{}

	for rows.Next() {
		var fromTableName string
		var fromColumnName string
		var toTableName string
		var toColumnName string // Unused
		var constaintName string
		err = rows.Scan(&fromTableName, &fromColumnName, &toTableName, &toColumnName, &constaintName)
		if err != nil {
			return ForeignKeysResult{}, err
		}
		_, tableExists := result[fromTableName]
		if !tableExists {
			result[fromTableName] = &ForeignKeyResultTable{
				Columns:     []string{},
				ForeignKeys: map[string]*ForeignKey{},
			}
		}
		result[fromTableName].Columns = append(result[fromTableName].Columns, fromColumnName)
		if constaintName != "" {
			_, fkeyExists := result[fromTableName].ForeignKeys[constaintName]
			if !fkeyExists {
				result[fromTableName].ForeignKeys[constaintName] = &ForeignKey{
					ToTable:          toTableName,
					ColumnConditions: [][2]string{},
				}
			}
			if fromColumnName != "" && toColumnName != "" {
				result[fromTableName].ForeignKeys[constaintName].ColumnConditions = append(
					result[fromTableName].ForeignKeys[constaintName].ColumnConditions,
					[2]string{fromColumnName, toColumnName},
				)
			}
		}
	}

	return result, nil
}

// Copy of github.com/amwolff/gorder DFS topological sort implementation,
// with the only change being allowing non-acyclic graphs (for better or worse).
func topologicalSort(digraph map[string][]string) ([]string, error) {
	var (
		acyclic       = true
		order         []string
		permanentMark = make(map[string]bool)
		temporaryMark = make(map[string]bool)
		visit         func(string)
	)

	visit = func(u string) {
		if temporaryMark[u] {
			acyclic = false
		} else if !(temporaryMark[u] || permanentMark[u]) {
			temporaryMark[u] = true
			for _, v := range digraph[u] {
				visit(v)
				if !acyclic {
					slog.Debug("Ripoff file appears to have cycle", slog.String("rowId", u))
				}
			}
			delete(temporaryMark, u)
			permanentMark[u] = true
			order = append([]string{u}, order...)
		}
	}

	for u := range digraph {
		if !permanentMark[u] {
			visit(u)
		}
	}
	return order, nil
}
