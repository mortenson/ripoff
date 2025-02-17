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
	Row         Row
	ToTable     string
	ToColumn    string
	UniqueValue string
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
	// A map from [table,column] -> a map of column values to row keys (ex: users:literal(1)) of the given table.
	uniqueConstraintMap := map[[2]string]map[string]string{}
	// A map from table to a list of columns that need mapped in uniqueConstraintMap.
	hasUniqueConstraintMap := map[string][]string{}
	for table, tableInfo := range foreignKeyResult {
		for _, foreignKey := range tableInfo.ForeignKeys {
			// We could possibly maintain a uniqueConstraintMap map for these as well, but tabling for now.
			if len(foreignKey.ColumnConditions) != 1 {
				continue
			}
			singleColumnFkeyMap[[2]string{table, foreignKey.ColumnConditions[0][0]}] = foreignKey
			// This is a foreign key to a unique index, not a primary key.
			if len(primaryKeyResult[foreignKey.ToTable]) == 1 && primaryKeyResult[foreignKey.ToTable][0] != foreignKey.ColumnConditions[0][1] {
				_, ok := hasUniqueConstraintMap[foreignKey.ToTable]
				if !ok {
					hasUniqueConstraintMap[foreignKey.ToTable] = []string{}
				}
				uniqueConstraintMap[[2]string{foreignKey.ToTable, foreignKey.ColumnConditions[0][1]}] = map[string]string{}
				hasUniqueConstraintMap[foreignKey.ToTable] = append(hasUniqueConstraintMap[foreignKey.ToTable], foreignKey.ColumnConditions[0][1])
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
				// If this is a foreign key, should ensure it uses the table:valueFunc() format.
				if isFkey && columnVal != "" {
					// Does the referenced table have more than one primary key, or does the constraint not point to a primary key?
					// Then is a foreign key to a non-primary key, we need to fill this info in later.
					if len(primaryKeyResult[foreignKey.ToTable]) != 1 || primaryKeyResult[foreignKey.ToTable][0] != foreignKey.ColumnConditions[0][1] {
						missingDependencies = append(missingDependencies, RowMissingDependency{
							Row:         ripoffRow,
							UniqueValue: columnVal,
							ToTable:     foreignKey.ToTable,
							ToColumn:    foreignKey.ColumnConditions[0][1],
						})
					} else {
						ripoffRow[field.Name] = fmt.Sprintf("%s:literal(%s)", foreignKey.ToTable, columnVal)
						continue
					}
				}
				// Normal column.
				ripoffRow[field.Name] = columnVal
			}
			rowKey := fmt.Sprintf("%s:literal(%s)", table, strings.Join(ids, "."))
			// For foreign keys to non-unique fields, we need to maintain our own map of unique values to rowKeys.
			columnsThatNeepMapped, needsMapped := hasUniqueConstraintMap[table]
			if needsMapped {
				for i, field := range fields {
					if columns[i] == nil {
						continue
					}
					columnVal := *columns[i]
					if slices.Contains(columnsThatNeepMapped, field.Name) {
						uniqueConstraintMap[[2]string{table, field.Name}][columnVal] = rowKey
					}
				}
			}
			ripoffFile.Rows[rowKey] = ripoffRow
		}
	}
	// Resolve missing dependencies now that all rows are in memory.
	for _, missingDependency := range missingDependencies {
		valueMap, ok := uniqueConstraintMap[[2]string{missingDependency.ToTable, missingDependency.ToColumn}]
		if !ok {
			return ripoffFile, fmt.Errorf("row has dependency on column %s.%s which is not mapped", missingDependency.ToTable, missingDependency.ToColumn)
		}
		rowKey, ok := valueMap[missingDependency.UniqueValue]
		if !ok {
			return ripoffFile, fmt.Errorf("row has dependency on column %s.%s which does not contain unqiue value %s", missingDependency.ToTable, missingDependency.ToColumn, missingDependency.UniqueValue)
		}
		dependencies, ok := missingDependency.Row["~dependencies"].([]string)
		if !ok {
			missingDependency.Row["~dependencies"] = []string{}
		}
		missingDependency.Row["~dependencies"] = append(dependencies, rowKey)
	}
	return ripoffFile, nil
}
