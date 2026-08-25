// Command relbind emits the two artifacts the semantic ABI admits as
// irreducible generated code, or proves the checked-in ones are already the
// emission.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen/relbind"
)

func main() {
	var (
		root  = flag.String("root", ".", "module root the axis packages live under")
		check = flag.Bool("check", false, "report drift instead of writing")
	)
	flag.Parse()

	if *check {
		drifts, err := relbind.Check(*root)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		for _, drift := range drifts {
			fmt.Fprintln(os.Stderr, drift)
		}
		if len(drifts) != 0 {
			os.Exit(1)
		}
		return
	}
	if err := relbind.Write(*root); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}
