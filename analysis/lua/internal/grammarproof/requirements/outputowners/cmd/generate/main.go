package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/wippyai/go-lua/analysis/lua/internal/grammarproof/requirements/outputowners"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(arguments []string) error {
	flags := flag.NewFlagSet("outputowners", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	out := flags.String("out", "", "generated evidence output")
	check := flags.Bool("check", false, "require checked-in evidence to be current")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if *out == "" {
		return fmt.Errorf("Program output owners: -out is required")
	}
	return outputowners.Generate(*out, *check)
}
