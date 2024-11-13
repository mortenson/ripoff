package ripoff

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/lib/pq"
)

func ExportToRipoff(ctx context.Context, conn *pgx.Conn, path string) (RipoffFile, error) {
	ripoffFile := RipoffFile{
		Rows: map[string]Row{},
	}

	tx, err := conn.Begin(ctx)
	if err != nil {
		return ripoffFile, err
	}
	defer tx.Rollback(ctx)

	primaryKeyResult, err := getPrimaryKeys(ctx, tx)
	if err != nil {
		return ripoffFile, err
	}
	foreignKeyResult, err := getForeignKeysResult(ctx, tx)
	if err != nil {
		return ripoffFile, err
	}
	// Assemble an easier to parse [table,column] -> table map for single column foreign keys.
	singleColumnFkeyMap := map[[2]string]string{}
	for table, tableInfo := range foreignKeyResult {
		for _, foreignKey := range tableInfo.ForeignKeys {
			if len(foreignKey.ColumnConditions) == 1 {
				singleColumnFkeyMap[[2]string{table, foreignKey.ColumnConditions[0][0]}] = foreignKey.ToTable
			}
		}
	}
	for table, primaryKeys := range primaryKeyResult {
		if len(primaryKeys) != 1 {
			return RipoffFile{}, fmt.Errorf("multiple primary keys are not supported in exports yet, abort on table: %s", table)
		}
		columns := make([]string, len(foreignKeyResult[table].Columns))
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
			columns, err := rows.Values()
			if err != nil {
				return RipoffFile{}, err
			}
			ripoffRow := Row{}
			var id any
			for i, field := range fields {
				// No need to export primary keys due to inference from schema.
				if primaryKeys[0] == field.Name {
					id = columns[i]
					continue
				}
				// If this is a foreign key, should ensure it uses the table:valueFunc() format.
				toTable, isFkey := singleColumnFkeyMap[[2]string{table, field.Name}]
				if isFkey {
					ripoffRow[field.Name] = fmt.Sprintf("%s:literal(%s)", toTable, columns[i])
					continue
				}
				// Normal column.
				ripoffRow[field.Name] = columns[i]
			}
			ripoffFile.Rows[fmt.Sprintf("%s:literal(%s)", table, id)] = ripoffRow
		}
	}
	return ripoffFile, nil
}
