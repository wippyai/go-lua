// go-lua-lint checks a Lua directory through the new analysis engine.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wippyai/go-lua/analysis/check/lint"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

type output struct {
	Diagnostics []diagnostic.Diagnostic `json:"diagnostics"`
	Timings     *lint.PhaseTimings      `json:"timings,omitempty"`
	Entries     int                     `json:"entries"`
}

func main() {
	var jsonOutput, timings bool
	var entries entryFlags
	flag.BoolVar(&jsonOutput, "json", false, "write machine-readable diagnostics")
	flag.BoolVar(&timings, "timings", false, "include structured timing data")
	flag.Var(&entries, "entry", "Lua entry path to check (repeatable); defaults to all files")
	flag.Parse()
	root, positionalEntries := ".", []string(nil)
	if flag.NArg() > 0 {
		root = flag.Arg(0)
		if info, statErr := os.Stat(root); statErr == nil && !info.IsDir() {
			positionalEntries = append(positionalEntries, filepath.Base(root))
			root = filepath.Dir(root)
			positionalEntries = append(positionalEntries, flag.Args()[1:]...)
		} else {
			positionalEntries = append(positionalEntries, flag.Args()[1:]...)
		}
	}
	entries = append(entries, positionalEntries...)
	project, err := lint.LoadDirectory(root, entries)
	exitCode := 0
	if err == nil {
		result, checkErr := lint.CheckProject(context.Background(), project)
		if checkErr != nil {
			err = checkErr
		} else if jsonOutput {
			value := output{Diagnostics: result.Diagnostics, Entries: len(result.Entries)}
			if timings {
				value.Timings = &result.Timings
			}
			var data []byte
			data, err = json.Marshal(value)
			if err == nil {
				_, err = fmt.Fprintln(os.Stdout, string(data))
			}
		} else {
			for _, item := range result.Diagnostics {
				fmt.Fprintln(os.Stdout, lint.RenderDiagnostic(item))
			}
			if timings {
				data, marshalErr := json.Marshal(result.Timings)
				if marshalErr != nil {
					err = marshalErr
				} else {
					_, err = fmt.Fprintln(os.Stdout, string(data))
				}
			}
		}
		if len(result.Diagnostics) != 0 {
			exitCode = 1
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "go-lua-lint:", err)
		exitCode = 2
	}
	if exitCode != 0 {
		os.Exit(exitCode)
	}
}

type entryFlags []string

func (f *entryFlags) String() string { return strings.Join(*f, ",") }
func (f *entryFlags) Set(value string) error {
	*f = append(*f, value)
	return nil
}
