package ripoff

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/lib/pq"
)

// parseColumnExclusions parses column exclusion specifications and returns
// table-specific exclusions and global column exclusions.
func parseColumnExclusions(excludeColumns []string) (map[string][]string, []string) {
	tableSpecific := make(map[string][]string)
	var globalColumns []string

	for _, spec := range excludeColumns {
		parts := strings.SplitN(spec, ".", 2)
		if len(parts) == 2 {
			// table.column format
			table, column := parts[0], parts[1]
			tableSpecific[table] = append(tableSpecific[table], column)
		} else {
			// column format - applies to all tables
			globalColumns = append(globalColumns, spec)
		}
	}

	return tableSpecific, globalColumns
}

// shouldExcludeColumn checks if a column should be excluded based on exclusion rules.
func shouldExcludeColumn(table, column string, tableSpecific map[string][]string, globalColumns []string) bool {
	// Check global column exclusions
	for _, globalCol := range globalColumns {
		if column == globalCol {
			return true
		}
	}

	// Check table-specific exclusions
	if excludedCols, exists := tableSpecific[table]; exists {
		for _, excludedCol := range excludedCols {
			if column == excludedCol {
				return true
			}
		}
	}

	return false
}

type RowMissingDependency struct {
	Row              Row
	ConstraintMapKey [3]string
}

// Exports all rows in the database to a ripoff file.
// excludeTables is a list of table names to exclude from the export.
// excludeColumns is a list of column specifications to exclude from the export.
// Format: "table.column" (exclude column from specific table) or "column" (exclude column from all tables).
func ExportToRipoff(ctx context.Context, tx pgx.Tx, excludeTables []string, excludeColumns []string) (RipoffFile, error) {
	ripoffFile := RipoffFile{
		Rows: map[string]Row{},
	}

	// Parse column exclusions
	tableSpecificExclusions, globalColumnExclusions := parseColumnExclusions(excludeColumns)

	// We use primary keys to determine what columns to use as row keys.
	primaryKeyResult, err := getPrimaryKeys(ctx, tx)
	if err != nil {
		return ripoffFile, err
	}

	// Remove excluded tables from the primary keys
	for _, table := range excludeTables {
		delete(primaryKeyResult, table)
	}

	// We use foreign keys to reference other rows using the table_name:literal(...) syntax.
	foreignKeyResult, err := getForeignKeysResult(ctx, tx)
	if err != nil {
		return ripoffFile, err
	}

	// Remove excluded tables from foreign key results
	for _, table := range excludeTables {
		delete(foreignKeyResult, table)
	}

	// A map from [table,column] -> ForeignKey for single column foreign keys.
	singleColumnFkeyMap := map[[2]string]*ForeignKey{}
	// A map from [table,constraintName,values] -> rowKey.
	constraintMap := map[[3]string]string{}
	for table, tableInfo := range foreignKeyResult {
		for _, foreignKey := range tableInfo.ForeignKeys {
			if len(foreignKey.ColumnConditions) == 1 {
				singleColumnFkeyMap[[2]string{table, foreignKey.ColumnConditions[0][0]}] = foreignKey
			}
		}
	}

	missingDependencies := []RowMissingDependency{}

	for table, primaryKeys := range primaryKeyResult {
		// Filter out excluded columns from the foreign key result columns
		var filteredColumns []string
		for _, column := range foreignKeyResult[table].Columns {
			if !shouldExcludeColumn(table, column, tableSpecificExclusions, globalColumnExclusions) {
				filteredColumns = append(filteredColumns, fmt.Sprintf("CAST(%s AS TEXT)", pq.QuoteIdentifier(column)))
			}
		}

		// Skip table if no columns remain after filtering
		if len(filteredColumns) == 0 {
			continue
		}

		selectQuery := fmt.Sprintf("SELECT %s FROM %s;", strings.Join(filteredColumns, ", "), pq.QuoteIdentifier(table))
		rows, err := tx.Query(ctx, selectQuery)
		if err != nil {
			return RipoffFile{}, err
		}
		defer rows.Close()
		fields := rows.FieldDescriptions()

		for rows.Next() {
			columnsRaw, err := rows.Values()
			if err != nil {
				return RipoffFile{}, err
			}
			// Convert the columns to nullable strings.
			columns := make([]*string, len(columnsRaw))
			for i, column := range columnsRaw {
				if column == nil {
					columns[i] = nil
				} else {
					str := column.(string)
					columns[i] = &str
				}
			}
			ripoffRow := Row{}
			// A map of fieldName -> tableName to convert values to literal:(...)
			literalFields := map[string]string{}
			ids := []string{}
			for i, field := range fields {
				// Null columns are still exported since we don't know if there is a default or not (at least not at time of writing).
				if columns[i] == nil {
					ripoffRow[field.Name] = nil
					continue
				}
				columnVal := *columns[i]
				// Note: for multi-column primary keys this is ugly.
				if slices.Contains(primaryKeys, field.Name) {
					ids = append(ids, columnVal)
				}
				foreignKey, isFkey := singleColumnFkeyMap[[2]string{table, field.Name}]
				// No need to export primary keys due to inference from schema on import.
				if len(primaryKeys) == 1 && primaryKeys[0] == field.Name {
					// The primary key is a foreign key, we'll need explicit dependencies.
					if isFkey && columnVal != "" {
						dependencies, ok := ripoffRow["~dependencies"].([]string)
						if !ok {
							ripoffRow["~dependencies"] = []string{}
						}
						ripoffRow["~dependencies"] = append(dependencies, fmt.Sprintf("%s:literal(%s)", foreignKey.ToTable, columnVal))
					}
					continue
				}
				// If this is a foreign key to a single-column primary key, we can use literal() instead of ~dependencies.
				if isFkey && columnVal != "" && len(primaryKeyResult[foreignKey.ToTable]) == 1 && primaryKeyResult[foreignKey.ToTable][0] == foreignKey.ColumnConditions[0][1] {
					literalFields[field.Name] = foreignKey.ToTable
				}
				// Normal column.
				ripoffRow[field.Name] = columnVal
			}
			rowKey := fmt.Sprintf("%s:literal(%s)", table, strings.Join(ids, "."))
			// Hash values of this row for dependency lookups in the future.
			for _, fkeys := range foreignKeyResult {
				for constraintName, fkey := range fkeys.ForeignKeys {
					if fkey.ToTable != table {
						continue
					}
					values := []string{}
					abort := false
					for _, conditions := range fkey.ColumnConditions {
						toColumnValue, hasToColumn := ripoffRow[conditions[1]]
						if hasToColumn && toColumnValue.(string) != "" {
							values = append(values, toColumnValue.(string))
						} else {
							abort = true
							break
						}
					}
					if abort {
						continue
					}
					constraintMap[[3]string{table, constraintName, strings.Join(values, ",")}] = rowKey
				}
			}
			// Now register missing dependencies for all our foreign keys.
			for constraintName, fkey := range foreignKeyResult[table].ForeignKeys {
				values := []string{}
				allLiteral := true
				for _, condition := range fkey.ColumnConditions {
					fieldValue, hasField := ripoffRow[condition[0]]
					fieldValueStr, isString := fieldValue.(string)
					if hasField && isString && fieldValue != "" {
						_, isLiteral := literalFields[condition[0]]
						if !isLiteral {
							allLiteral = false
						}
						values = append(values, fieldValueStr)
					}
				}
				// We have enough values to satisfy the column conditions.
				if !allLiteral && len(values) == len(fkey.ColumnConditions) {
					missingDependencies = append(missingDependencies, RowMissingDependency{
						Row:              ripoffRow,
						ConstraintMapKey: [3]string{fkey.ToTable, constraintName, strings.Join(values, ",")},
					})
				}
			}
			// Finally convert some fields to use literal() for UX reasons.
			for fieldName, toTable := range literalFields {
				ripoffRow[fieldName] = fmt.Sprintf("%s:literal(%s)", toTable, ripoffRow[fieldName])
			}
			ripoffFile.Rows[rowKey] = ripoffRow
		}
	}
	// Resolve missing dependencies now that all rows are in memory.
	for _, missingDependency := range missingDependencies {
		rowKey, ok := constraintMap[missingDependency.ConstraintMapKey]
		if !ok {
			return ripoffFile, fmt.Errorf("row has missing dependency on constraint map key %s", missingDependency.ConstraintMapKey)
		}
		dependencies, ok := missingDependency.Row["~dependencies"].([]string)
		if !ok {
			missingDependency.Row["~dependencies"] = []string{}
		}
		missingDependency.Row["~dependencies"] = append(dependencies, rowKey)
	}
	return ripoffFile, nil
}
