package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/wippyai/go-lua/analysis/lua/internal/grammarproof/requirements/programsupply"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(arguments []string) error {
	flags := flag.NewFlagSet("program-supply", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	out := flags.String("out", "", "generated evidence output")
	check := flags.Bool("check", false, "require checked-in evidence to be current")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if *out == "" {
		return fmt.Errorf("program supply: -out is required")
	}
	return programsupply.Generate(*out, *check)
}
