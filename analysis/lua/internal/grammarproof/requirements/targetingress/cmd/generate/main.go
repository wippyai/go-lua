package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/wippyai/go-lua/analysis/lua/internal/grammarproof/requirements/targetingress"
)

func main() {
	out := flag.String("out", "", "generated evidence output")
	check := flag.Bool("check", false, "require checked-in evidence to be current")
	flag.Parse()
	if *out == "" {
		fmt.Fprintln(os.Stderr, "Target ingress requirements: -out is required")
		os.Exit(1)
	}
	if err := targetingress.Generate(*out, *check); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
