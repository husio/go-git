package main

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

// https://wyag.thb.lt

func main() {
	if len(os.Args) == 1 {
		fmt.Fprintf(os.Stderr, "Usage: %s <command> [<flags>]\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\nAvailable commands are:\n\t%s\n", strings.Join(availableCmds(), "\n\t"))
		fmt.Fprintf(os.Stderr, "Run '%s <command> -help' to learn more about each command.\n", os.Args[0])
		os.Exit(2)
	}
	run, ok := commands[os.Args[1]]
	if !ok {
		fmt.Fprintf(os.Stderr, "Unknown command %q\n", os.Args[1])
		fmt.Fprintf(os.Stderr, "\nAvailable commands are:\n\t%s\n", strings.Join(availableCmds(), "\n\t"))
		os.Exit(2)
	}

	// Skip first two arguments. Second argument is the command name that
	// we just consumed.
	if err := run(os.Stdin, os.Stdout, os.Args[2:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var commands = map[string]func(input io.Reader, output io.Writer, args []string) error{
	"cat-file":    cmdCatFile,
	"checkout":    cmdCheckout,
	"hash-object": cmdHashObject,
	"init":        cmdInit,
	"log":         cmdLog,
	"ls-tree":     cmdLsTree,
	"show-ref":    cmdShowRef,
	"tag":         cmdTag,
}

func availableCmds() []string {
	available := make([]string, 0, len(commands))
	for name := range commands {
		available = append(available, name)
	}
	sort.Strings(available)
	return available
}
