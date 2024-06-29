package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path"

	"github.com/jackc/pgx/v5"

	"github.com/mortenson/ripoff"
)

func errAttr(err error) slog.Attr {
	return slog.Any("error", err)
}

func main() {
	verbosePtr := flag.Bool("v", false, "enable verbose output")
	softPtr := flag.Bool("s", false, "do not commit generated queries")
	flag.Parse()

	if *verbosePtr {
		slog.SetLogLoggerLevel(slog.LevelDebug)
	}

	dburl := os.Getenv("DATABASE_URL")
	if dburl == "" {
		slog.Error("DATABASE_URL env variable is required")
		os.Exit(1)
	}

	if len(flag.Args()) != 1 {
		slog.Error("Path to YAML files required")
		os.Exit(1)
	}
	rootDirectory := path.Clean(flag.Arg(0))
	totalRipoff, err := ripoff.RipoffFromDirectory(rootDirectory)
	if err != nil {
		slog.Error("Could not load ripoff", errAttr(err))
		os.Exit(1)
	}

	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dburl)
	if err != nil {
		slog.Error("Could not connect to database", errAttr(err))
		os.Exit(1)
	}
	defer conn.Close(ctx)

	tx, err := conn.Begin(ctx)
	if err != nil {
		slog.Error("Could not create transaction", errAttr(err))
		os.Exit(1)
	}
	defer func() {
		err = tx.Rollback(ctx)
		if err != pgx.ErrTxClosed {
			slog.Error("Could not rollback transaction", errAttr(err))
			os.Exit(1)
		}
	}()

	err = ripoff.RunRipoff(ctx, tx, totalRipoff)
	if err != nil {
		slog.Error("Could not run ripoff", errAttr(err))
		os.Exit(1)
	}

	if *softPtr {
		slog.Info("Not committing transaction due to -s flag")
	} else {
		err = tx.Commit(ctx)
		if err != nil {
			slog.Error("Could not commit transaction", errAttr(err))
			os.Exit(1)
		}
	}

	slog.Info(fmt.Sprintf("Ripoff complete, %d rows processed", len(totalRipoff.Rows)))
}
