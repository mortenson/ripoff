package ripoff

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/lib/pq"
)

type RowMissingDependency struct {
	Row              Row
	ConstraintMapKey [3]string
}

// Exports all rows in the database to a ripoff file.
func ExportToRipoff(ctx context.Context, tx pgx.Tx) (RipoffFile, error) {
	ripoffFile := RipoffFile{
		Rows: map[string]Row{},
	}

	// We use primary keys to determine what columns to use as row keys.
	primaryKeyResult, err := getPrimaryKeys(ctx, tx)
	if err != nil {
		return ripoffFile, err
	}
	// We use foreign keys to reference other rows using the table_name:literal(...) syntax.
	foreignKeyResult, err := getForeignKeysResult(ctx, tx)
	if err != nil {
		return ripoffFile, err
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
		columns := make([]string, len(foreignKeyResult[table].Columns))
		// Due to yaml limitations, ripoff treats all data as nullable text on import and export.
		for i, column := range foreignKeyResult[table].Columns {
			columns[i] = fmt.Sprintf("CAST(%s AS TEXT)", pq.QuoteIdentifier(column))
		}
		selectQuery := fmt.Sprintf("SELECT %s FROM %s;", strings.Join(columns, ", "), pq.QuoteIdentifier(table))
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
