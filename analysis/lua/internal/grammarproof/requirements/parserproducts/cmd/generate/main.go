package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/wippyai/go-lua/analysis/lua/internal/grammarproof/requirements/parserproducts"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fail("%v", err)
	}
}

func run(arguments []string) error {
	flags := flag.NewFlagSet("parser-products", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	root := flags.String("root", ".", "module root")
	out := flags.String("out", "", "generated evidence output")
	check := flags.Bool("check", false, "require checked-in evidence to be current")
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
	return parserproducts.Generate(absoluteRoot, *out, *check)
}

func fail(format string, arguments ...any) {
	fmt.Fprintf(os.Stderr, "parser products: "+format+"\n", arguments...)
	os.Exit(1)
}
