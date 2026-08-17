// Command generate emits the checked-in relation catalog projections.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/wippyai/go-lua/analysis/schema/denominator/generator"
)

func main() {
	input := flag.String("input", "", "catalog schema input")
	history := flag.String("history", "", "checked-in semantic revision history")
	retired := flag.String("retired", "", "append-only permanently retired relation identities")
	relationOutput := flag.String("relations", "", "relation-schema Go output")
	check := flag.Bool("check", false, "fail when output is not fresh")
	writeHistory := flag.Bool("write-history", false, "write the checked-in semantic revision history")
	flag.Parse()
	if err := generator.Run(*input, *history, *retired, *relationOutput, *check, *writeHistory); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
