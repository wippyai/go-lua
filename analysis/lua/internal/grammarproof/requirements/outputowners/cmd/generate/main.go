package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/wippyai/go-lua/analysis/lua/internal/grammarproof/requirements/outputowners"
)

func main() {
	out := flag.String("out", "", "generated evidence output")
	check := flag.Bool("check", false, "require checked-in evidence to be current")
	flag.Parse()
	if *out == "" {
		fmt.Fprintln(os.Stderr, "Program output owners: -out is required")
		os.Exit(1)
	}
	if err := outputowners.Generate(*out, *check); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
