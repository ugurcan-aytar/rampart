// Package commands hosts the rampart CLI subcommand implementations.
// Dispatch picks one based on args[0]; each subcommand lives in its own
// file so they're individually readable (scan.go is the only shipping
// subcommand today; future subcommands plug into the same dispatch).
package commands

import (
	"context"
	"fmt"
	"io"
)

// Dispatch routes args[0] to its subcommand. Unknown or missing subcommand
// prints usage to stderr and returns an error.
func Dispatch(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		printUsage(stderr)
		return fmt.Errorf("missing subcommand")
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "scan":
		return Scan(ctx, rest, stdout)
	case "help", "-h", "--help":
		printUsage(stdout)
		return nil
	default:
		printUsage(stderr)
		return fmt.Errorf("unknown subcommand: %s", sub)
	}
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "usage: rampart <subcommand> [args]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "subcommands:")
	fmt.Fprintln(w, "  scan    Parse a lockfile and print the SBOM (text | json | sarif)")
	fmt.Fprintln(w, "  help    Print this message")
}
