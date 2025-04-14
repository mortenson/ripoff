package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path"
	"path/filepath"

	"github.com/jackc/pgx/v5"
	"gopkg.in/yaml.v3"

	"github.com/mortenson/ripoff"
)

func errAttr(err error) slog.Attr {
	return slog.Any("error", err)
}

func main() {
	// Define flags
	var excludeTables stringSliceFlag
	flag.Var(&excludeTables, "exclude", "Exclude specific tables from export (can be specified multiple times)")
	
	// Parse flags
	flag.Parse()
	
	dburl := os.Getenv("DATABASE_URL")
	if dburl == "" {
		slog.Error("DATABASE_URL env variable is required")
		os.Exit(1)
	}

	args := flag.Args()
	if len(args) != 1 {
		slog.Error("Path to export directory is required")
		os.Exit(1)
	}

	// Connect to database.
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dburl)
	if err != nil {
		slog.Error("Could not connect to database", errAttr(err))
		os.Exit(1)
	}
	defer conn.Close(ctx)

	exportDirectory := path.Clean(args[0])
	dirInfo, err := os.Stat(exportDirectory)
	if err == nil && !dirInfo.IsDir() {
		slog.Error("Export directory is not a directory")
		os.Exit(1)
	}

	// Directory exists, delete it after verifying that it's safe to do so.
	if err == nil && !os.IsNotExist(err) {
		err = filepath.WalkDir(exportDirectory, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !entry.IsDir() && filepath.Ext(path) != ".yaml" && filepath.Ext(path) != ".yml" {
				return fmt.Errorf("ripoff-export can only safely delete directories that only contain YAML files, found: %s", path)
			}
			return nil
		})
		if err != nil {
			slog.Error("Error verifying test directory", errAttr(err))
			os.Exit(1)
		}
		err = os.RemoveAll(exportDirectory)
		if err != nil {
			slog.Error("Could not read from export directory", errAttr(err))
			os.Exit(1)
		}
	}

	err = os.MkdirAll(exportDirectory, 0755)
	if err != nil {
		slog.Error("Could not re-create export directory", errAttr(err))
		os.Exit(1)
	}

	tx, err := conn.Begin(ctx)
	if err != nil {
		slog.Error("Could not create transaction", errAttr(err))
		os.Exit(1)
	}
	defer func() {
		err = tx.Rollback(ctx)
		if err != nil && err != pgx.ErrTxClosed {
			slog.Error("Could not rollback transaction", errAttr(err))
			os.Exit(1)
		}
	}()

	// Pass the excluded tables to the export function
	ripoffFile, err := ripoff.ExportToRipoff(ctx, tx, excludeTables)
	if err != nil {
		slog.Error("Could not assemble ripoff file from database", errAttr(err))
		os.Exit(1)
	}

	var ripoffFileBuf bytes.Buffer
	yamlEncoder := yaml.NewEncoder(&ripoffFileBuf)
	yamlEncoder.SetIndent(2)
	err = yamlEncoder.Encode(ripoffFile)
	if err != nil {
		slog.Error("Could not marshal yaml from ripoff file", errAttr(err))
		os.Exit(1)
	}

	err = os.WriteFile(path.Join(exportDirectory, "ripoff.yml"), ripoffFileBuf.Bytes(), 0644)
	if err != nil {
		slog.Error("Could not write ripoff file", errAttr(err))
		os.Exit(1)
	}

	slog.Info(fmt.Sprintf("Ripoff export complete, %d rows exported", len(ripoffFile.Rows)))
}

// stringSliceFlag is a custom flag to support multiple --exclude flags
type stringSliceFlag []string

func (s *stringSliceFlag) String() string {
	return fmt.Sprintf("%v", *s)
}

func (s *stringSliceFlag) Set(value string) error {
	*s = append(*s, value)
	return nil
}
