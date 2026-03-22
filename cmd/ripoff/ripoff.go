package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/mortenson/ripoff"
)

func errAttr(err error) slog.Attr {
	return slog.Any("error", err)
}

func confirmPluginsSafe(plugins map[string]ripoff.RipoffPlugin) {
	baseDir, err := os.UserHomeDir()
	if err != nil {
		baseDir = os.TempDir()
	}
	consentFilePath := path.Join(baseDir, ".ripoff-consent")
	consentFile, err := os.ReadFile(consentFilePath)
	if err != nil && !os.IsNotExist(err) {
		slog.Error("Could not read from consent file", errAttr(err), slog.String("filepath", consentFilePath))
	}
	consentFileLines := strings.Split(string(consentFile), "\n")
	scanner := bufio.NewScanner(os.Stdin)
	newConsentLines := []string{}
	for _, plugin := range plugins {
		cmdJson, err := json.Marshal(plugin)
		if err != nil {
			slog.Error("Could not marshal plugin to JSON for consent file", errAttr(err), slog.Any("plugin", plugin))
			os.Exit(1)
		}
		if !slices.Contains(consentFileLines, string(cmdJson)) {
			newConsentLines = append(newConsentLines, string(cmdJson))
		}
	}
	if len(newConsentLines) > 0 {
		fmt.Printf("You have not run these ripoff plugins before, please confirm that the following plugin configurations/commands are safe to run on your machine: \n")
		fmt.Println()
		for _, consentPrompt := range newConsentLines {
			fmt.Printf("	%s\n", consentPrompt)
		}
		fmt.Println()
		fmt.Println("Run the above? (Y/N)")
		scanner.Scan()
		input := scanner.Text()
		if input == "y" || input == "Y" {
			consentFileLines = append(consentFileLines, newConsentLines...)
			err = os.WriteFile(consentFilePath, []byte(strings.Join(consentFileLines, "\n")), 0644)
			if err != nil {
				slog.Error("Could not append to the consent file", errAttr(err), slog.String("filepath", consentFilePath))
			}
			fmt.Println("Proceeding...")
		} else {
			fmt.Println("ABORT")
			os.Exit(1)
		}
	}
}

func main() {
	verbosePtr := flag.Bool("v", false, "enable verbose output")
	softPtr := flag.Bool("s", false, "do not commit generated queries")
	maxConcurrencyPtr := flag.Int("c", ripoff.DEFAULT_MAX_CONCURRENCY, "maximum number of rows to generate queries for at one time. defaults at 1000")
	unsafePluginPtr := flag.Bool("u", false, "execute new plugin commands without prompting. only for use in CI or trusted environments")
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

	// Start database transaction.
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dburl)
	if err != nil {
		slog.Error("Could not connect to database", errAttr(err))
		os.Exit(1)
	}
	defer func() {
		err := conn.Close(ctx)
		if err != nil {
			slog.Error("Could not close database connection", errAttr(err))
		}
	}()

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

	enums, err := ripoff.GetEnumValues(ctx, tx)
	if err != nil {
		slog.Error("Could not load enums", errAttr(err))
		os.Exit(1)
	}

	rootDirectory := path.Clean(flag.Arg(0))
	totalRipoff, err := ripoff.RipoffFromDirectory(rootDirectory, enums)
	if err != nil {
		slog.Error("Could not load ripoff", errAttr(err))
		os.Exit(1)
	}

	if !*unsafePluginPtr && len(totalRipoff.Plugins) > 0 {
		confirmPluginsSafe(totalRipoff.Plugins)
	}

	err = ripoff.RunRipoff(ctx, tx, totalRipoff, *maxConcurrencyPtr)
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
