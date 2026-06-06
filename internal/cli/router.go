package cli

import (
	"flag"
	"fmt"
	"os"

	"github.com/qiezi999/sql-cli/internal/config"
	"github.com/qiezi999/sql-cli/internal/output"
)

// Run is the main CLI router that parses global flags and dispatches to subcommands.
// It returns an exit code (0 for success, non-zero for errors).
// Exit codes:
//   - 0: Success
//   - 1: QUERY_ERROR or unknown subcommand
//   - 2: CONFIG_ERROR
//   - 3: CONNECTION_ERROR
//   - 4: AUTH_ERROR
//   - 5: SAFETY_BLOCKED
//   - 6: TIMEOUT
//   - 7: PERMISSION_DENIED
//   - 99: INTERNAL_ERROR
func Run(args []string) int {
	// Parse global flags
	fs := flag.NewFlagSet("sql-cli", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	dsn := fs.String("dsn", "", "Database connection string (mysql://user:pass@host:port/db)")
	version := fs.Bool("version", false, "Show version information")

	// Custom usage function to show help
	fs.Usage = printHelp

	// Parse flags
	if err := fs.Parse(args[1:]); err != nil {
		// flag.Parse returns error on -h or -help, which we want to handle specially
		if err == flag.ErrHelp {
			return 0
		}
		// Other parsing errors
		return output.ErrorCodeInternalError.ExitCode()
	}

	// Handle --version flag
	if *version {
		printVersion()
		return 0
	}

	// Get remaining arguments (subcommand + subcommand args)
	remaining := fs.Args()

	// If no subcommand, show help
	if len(remaining) == 0 {
		printHelp()
		return 0
	}

	// Check for help/version subcommands before DSN resolution
	if remaining[0] == "help" || remaining[0] == "--help" || remaining[0] == "-h" {
		printHelp()
		return 0
	}

	if remaining[0] == "version" {
		printVersion()
		return 0
	}

	// Resolve DSN from flag or environment
	resolvedDSN, err := config.ResolveDSN(*dsn)
	if err != nil {
		// DSN resolution failed - emit error and exit
		envelope := output.NewErrorEnvelope(
			output.ErrorCodeConfigError,
			err.Error(),
			nil,
		)
		output.WriteError(envelope)
		return output.ErrorCodeConfigError.ExitCode()
	}

	// Extract subcommand
	subcommand := remaining[0]
	subcommandArgs := remaining[1:]

	// Dispatch to subcommand handler
	return dispatch(subcommand, resolvedDSN, subcommandArgs)
}

// dispatch routes to the appropriate subcommand handler.
// It returns the exit code from the handler.
func dispatch(subcommand, dsn string, args []string) int {
	switch subcommand {
	case "databases":
		return handleDatabases(dsn, args)
	case "tables":
		return handleTables(dsn, args)
	case "describe":
		return handleDescribe(dsn, args)
	case "query":
		return handleQuery(dsn, args)
	case "--help", "-h":
		printHelp()
		return 0
	default:
		// Unknown subcommand
		envelope := output.NewErrorEnvelope(
			output.ErrorCodeQueryError,
			fmt.Sprintf("unknown subcommand: %s", subcommand),
			nil,
		)
		output.WriteError(envelope)
		return output.ErrorCodeQueryError.ExitCode()
	}
}

// Placeholder handlers - will be implemented in Tasks 22-25

func handleDatabases(dsn string, args []string) int {
	// TODO: Implement in Task 22
	envelope := output.NewErrorEnvelope(
		output.ErrorCodeInternalError,
		"databases subcommand not yet implemented",
		nil,
	)
	output.WriteError(envelope)
	return output.ErrorCodeInternalError.ExitCode()
}

func handleTables(dsn string, args []string) int {
	// TODO: Implement in Task 23
	envelope := output.NewErrorEnvelope(
		output.ErrorCodeInternalError,
		"tables subcommand not yet implemented",
		nil,
	)
	output.WriteError(envelope)
	return output.ErrorCodeInternalError.ExitCode()
}

func handleDescribe(dsn string, args []string) int {
	// TODO: Implement in Task 24
	envelope := output.NewErrorEnvelope(
		output.ErrorCodeInternalError,
		"describe subcommand not yet implemented",
		nil,
	)
	output.WriteError(envelope)
	return output.ErrorCodeInternalError.ExitCode()
}

func handleQuery(dsn string, args []string) int {
	// TODO: Implement in Task 25
	envelope := output.NewErrorEnvelope(
		output.ErrorCodeInternalError,
		"query subcommand not yet implemented",
		nil,
	)
	output.WriteError(envelope)
	return output.ErrorCodeInternalError.ExitCode()
}

// printHelp displays usage information to stdout.
// Help goes to stdout for grep/piping compatibility.
// stderr is reserved for debug/diagnostics only.
func printHelp() {
	fmt.Fprint(os.Stdout, `sql-cli - MySQL command-line interface for agents

Usage:
  sql-cli --dsn <dsn> <subcommand> [args]

Global Flags:
  --dsn string
        Database connection string (mysql://user:pass@host:port/db)
        Can also be set via SQL_CLI_DSN environment variable

Subcommands:
  databases              List all databases
  tables <database>      List tables in database
  describe <database.table>
                        Show table structure
  query <sql>           Execute SQL query

Examples:
  sql-cli --dsn mysql://root:pass@localhost:3306/ databases
  sql-cli --dsn mysql://root:pass@localhost:3306/mydb tables
  sql-cli --dsn mysql://root:pass@localhost:3306/mydb describe users
  sql-cli --dsn mysql://root:pass@localhost:3306/mydb query "SELECT * FROM users LIMIT 10"

Exit Codes:
  0   Success
  1   Query error
  2   Configuration error
  3   Connection error
  4   Authentication error
  5   Safety blocked
  6   Timeout
  7   Permission denied
  99  Internal error
`)
}

// printVersion displays version information to stdout.
// Version goes to stdout for grep/piping compatibility.
// stderr is reserved for debug/diagnostics only.
func printVersion() {
	// Version will be set during build via ldflags
	fmt.Fprintln(os.Stdout, "sql-cli version 0.1.0")
}