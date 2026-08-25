// Command relcut reads the Wave 5 deletion manifest: it validates the manifest
// against the repository and prints the entries, or the bare paths, a lane
// executes from.
//
//	go run ./cmd/relcut                                   validate and summarise
//	go run ./cmd/relcut -format paths -step 4             the step's paths, one per line
//	go run ./cmd/relcut -format paths -layer L2.2         one dependency stratum
//	go run ./cmd/relcut -format entries -disposition keep-if-generic
//
// It never writes to the repository. The manifest is read-only until the cut.
//
// Exit status: 0 the manifest is executable, 1 it is not.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/wippyai/go-lua/internal/relcut"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("relcut", flag.ContinueOnError)
	flags.SetOutput(stderr)
	format := flags.String("format", "summary", "summary, entries, or paths")
	layer := flags.String("layer", "", "restrict to one dependency layer, e.g. L2.2")
	step := flags.Int("step", 0, "restrict to one Wave 5 cut step, 1..8")
	disposition := flags.String("disposition", "", "restrict to delete, restate, or keep-if-generic")
	if err := flags.Parse(args); err != nil {
		return 1
	}

	manifest, err := relcut.Load()
	if err != nil {
		fmt.Fprintf(stderr, "relcut: %v\n", err)
		return 1
	}
	root, err := relcut.RepositoryRoot(".")
	if err != nil {
		fmt.Fprintf(stderr, "relcut: %v\n", err)
		return 1
	}
	findings := relcut.Validate(manifest, root)

	selected := manifest.Select(relcut.Select{
		Layer:       *layer,
		Step:        *step,
		Disposition: relcut.Disposition(*disposition),
	})

	switch *format {
	case "paths":
		for _, entry := range selected {
			for _, path := range entry.Paths {
				fmt.Fprintln(stdout, path)
			}
		}
	case "entries":
		for _, entry := range selected {
			printEntry(stdout, entry)
		}
	case "summary":
		printSummary(stdout, manifest, selected)
	default:
		fmt.Fprintf(stderr, "relcut: unknown format %q\n", *format)
		return 1
	}

	if len(findings) > 0 {
		fmt.Fprintln(stdout, "\n== VALIDATION ==")
		for _, finding := range findings {
			fmt.Fprintln(stdout, finding)
		}
	}
	if relcut.Refused(findings) {
		fmt.Fprintln(stdout, "\nMANIFEST REFUSED")
		return 1
	}
	return 0
}

func printEntry(stdout io.Writer, entry relcut.Entry) {
	fmt.Fprintf(stdout, "%s  [%s step %d layer %s]  %d files / %d loc\n",
		entry.ID, entry.Disposition, entry.CutStep, orDash(entry.ResidueLayer),
		entry.Measured.Files, entry.Measured.NonTestLOC)
	fmt.Fprintf(stdout, "  authority: %s\n", entry.Authority)
	if len(entry.BlockedBy) > 0 {
		fmt.Fprintf(stdout, "  blocked by: %s\n", strings.Join(entry.BlockedBy, ", "))
	}
	for _, law := range entry.LawsDie {
		fmt.Fprintf(stdout, "  law dies: %s\n", law)
	}
	for _, law := range entry.LawsRestated {
		fmt.Fprintf(stdout, "  law restated: %s -> %s\n", law.Law, law.Target)
	}
	if entry.ProofObligation != "" {
		fmt.Fprintf(stdout, "  proof obligation: %s\n", entry.ProofObligation)
	}
	if entry.Note != "" {
		fmt.Fprintf(stdout, "  note: %s\n", entry.Note)
	}
	for _, path := range entry.Paths {
		fmt.Fprintf(stdout, "    %s\n", path)
	}
	fmt.Fprintln(stdout)
}

func printSummary(stdout io.Writer, manifest relcut.Manifest, selected []relcut.Entry) {
	fmt.Fprintf(stdout, "manifest pinned at %s, %d entries\n", manifest.PinnedRef, len(manifest.Entries))
	fmt.Fprintf(stdout, "%s\n\n", manifest.Rule)
	byDisposition := map[relcut.Disposition]relcut.Measurement{}
	for _, entry := range selected {
		total := byDisposition[entry.Disposition]
		total.Files += entry.Measured.Files
		total.NonTestLOC += entry.Measured.NonTestLOC
		byDisposition[entry.Disposition] = total
	}
	for _, disposition := range []relcut.Disposition{
		relcut.DispositionDelete, relcut.DispositionRestate, relcut.DispositionKeepIfGeneric} {
		total := byDisposition[disposition]
		fmt.Fprintf(stdout, "%-16s %4d files  %6d non-test loc\n", disposition, total.Files, total.NonTestLOC)
	}
	fmt.Fprintln(stdout)
	for _, entry := range selected {
		fmt.Fprintf(stdout, "%-8s step %d  %-16s %-34s %4d files %6d loc\n",
			orDash(entry.ResidueLayer), entry.CutStep, entry.Disposition, entry.ID,
			entry.Measured.Files, entry.Measured.NonTestLOC)
	}
}

func orDash(text string) string {
	if text == "" {
		return "-"
	}
	return text
}
