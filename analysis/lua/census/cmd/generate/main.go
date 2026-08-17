package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/wippyai/go-lua/analysis/lua/census"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fail("%v", err)
	}
}

func run(arguments []string) error {
	flags := flag.NewFlagSet("parser-census", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	root := flags.String("root", ".", "module root")
	out := flags.String("out", "", "generated census output")
	check := flags.Bool("check", false, "require checked-in census to be current")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if *out == "" {
		return fmt.Errorf("-out is required")
	}
	absoluteRoot, err := filepath.Abs(*root)
	if err != nil {
		return fmt.Errorf("resolve root: %w", err)
	}
	return census.Generate(absoluteRoot, *out, *check)
}

func fail(format string, arguments ...any) {
	fmt.Fprintf(os.Stderr, "parser census: "+format+"\n", arguments...)
	os.Exit(1)
}
