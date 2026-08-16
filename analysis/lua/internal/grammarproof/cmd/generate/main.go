package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/wippyai/go-lua/analysis/lua/internal/grammarproof"
)

func main() {
	root := flag.String("root", ".", "module root")
	out := flag.String("out", "", "generated evidence output")
	check := flag.Bool("check", false, "require checked-in evidence to be current")
	flag.Parse()
	if *out == "" {
		fail("-out is required")
	}
	absoluteRoot, err := filepath.Abs(*root)
	if err != nil {
		fail("resolve root: %v", err)
	}
	if err := grammarproof.Generate(absoluteRoot, *out, *check); err != nil {
		fail("%v", err)
	}
}

func fail(format string, arguments ...any) {
	fmt.Fprintf(os.Stderr, "grammarproof: "+format+"\n", arguments...)
	os.Exit(1)
}
