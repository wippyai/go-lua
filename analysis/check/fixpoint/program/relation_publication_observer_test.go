package program

import (
	"context"
	"fmt"
	goast "go/ast"
	goparser "go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"
	"github.com/wippyai/go-lua/analysis/domain/formal"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/compiler/parse"
)

const relationDifferentialSourceTimeout = 10 * time.Second

type relationDifferentialSource struct {
	name   string
	source string
}

type relationDifferentialReport struct {
	corpusSize int
	passed     int
	gaps       []string
}

func (r *relationDifferentialReport) addGap(format string, args ...any) {
	r.gaps = append(r.gaps, fmt.Sprintf(format, args...))
}

func (r *relationDifferentialReport) observe(
	source string,
	frozen *transformer.RelationProgram,
	execution transformer.RelationSolveExecution,
	published formalLexicalPublishedProgram,
) {
	if frozen == nil || execution == (transformer.RelationSolveExecution{}) {
		r.addGap("%s: retention boundary missing frozen solve", source)
		return
	}
	if published.root == nil || len(published.results) == 0 {
		r.addGap("%s: retention boundary precedes lexical publication", source)
		return
	}
	for _, plan := range frozen.BodyPlanObservations() {
		if !plan.Acyclic {
			continue
		}
		r.corpusSize++
		if err := r.compareAcyclicBody(frozen, published, plan.Body); err != nil {
			r.addGap("%s/%s: %v", source, plan.Body, err)
			continue
		}
		r.passed++
	}
}

func (r *relationDifferentialReport) compareAcyclicBody(
	frozen *transformer.RelationProgram,
	published formalLexicalPublishedProgram,
	bodyID lexicalidentity.StableLexicalBodyID,
) error {
	result := published.results[bodyID]
	if result == nil {
		return fmt.Errorf("published body result missing")
	}
	entry, ok := result.EntryState()
	if !ok {
		return fmt.Errorf("published entry missing")
	}
	binding, err := frozen.BindRealRelationBody(bodyID, entry)
	if err != nil {
		return fmt.Errorf("bind real relation body: %w", err)
	}
	artifact, err := binding.Compile()
	if err != nil {
		return fmt.Errorf("compile relation body: %w", err)
	}
	kernels := make(map[uint64]transformer.RelationOccurrenceKernel, len(binding.Occurrences()))
	for _, occurrence := range binding.Occurrences() {
		occurrence := occurrence
		kernels[occurrence.Ordinal] = func(bound transformer.BoundRelationOccurrence, _ equation.BoundEquation, _ equation.Partition) (equation.TransactionResult, error) {
			if bound.Ordinal != occurrence.Ordinal {
				return equation.TransactionResult{}, fmt.Errorf("foreign occurrence %d", bound.Ordinal)
			}
			return equation.TransactionResult{
				Complete: true,
				Closure:  emptyPublishedRelationClosure().ToOutputClosure(),
				Access: equation.AccessRecord{Payload: transformer.OperatorAccess{
					Kind: occurrence.Kind, Occurrence: formal.NewOccurrenceID(bodyID, occurrence.Ordinal),
				}},
			}, nil
		}
	}
	registry, err := binding.KernelRegistry(kernels)
	if err != nil {
		return fmt.Errorf("kernel registry: %w", err)
	}
	vm, err := equation.NewAcyclicVM(registry)
	if err != nil {
		return fmt.Errorf("acyclic VM: %w", err)
	}
	bound, err := equation.BindEntry(artifact, equation.EntryBinding{
		Parameter: equation.EntryParameter{Body: equation.BodyID(bodyID), Name: "entry"},
		Value:     []byte(fmt.Sprintf("entry/%s", bodyID)),
	})
	if err != nil {
		return fmt.Errorf("bind VM entry: %w", err)
	}
	evaluated, err := vm.Evaluate(bound)
	if err != nil {
		return fmt.Errorf("evaluate VM: %w", err)
	}
	production := emptyPublishedRelationClosure()
	if !production.ToOutputClosure().Equal(evaluated.Closure) {
		return fmt.Errorf("closure inequality")
	}
	return nil
}

func emptyPublishedRelationClosure() transformer.PublishedRelationClosure {
	return transformer.PublishedRelationClosure{
		Values:               []equation.Fact{},
		Outcomes:             []equation.Fact{},
		DiagnosticCandidates: []equation.Fact{},
		AllocationRekeys:     []equation.AllocationRekey{},
	}
}

func TestRelationPublicationObserverRetainsOnlyThePublicationBoundary(t *testing.T) {
	report := &relationDifferentialReport{}
	calls := 0
	_, err := RunChunk(parseChunk(t, `
local value = "retained"
return value
`), Config{
		Check: body.Config{Registry: standard.Registry()},
		relationPublicationObserver: func(
			frozen *transformer.RelationProgram,
			execution transformer.RelationSolveExecution,
			published formalLexicalPublishedProgram,
		) error {
			calls++
			report.observe("smoke", frozen, execution, published)
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("publication observer calls = %d, want 1", calls)
	}
	report.assert(t)
}

// TestRelationPublicationDifferentialCorpus replays the checktest literals
// first, then every standalone fixture source, through the real program
// checker. The retained solve is observed synchronously at each source's
// publication boundary, so each body is evaluated, compared, and released
// before the next source starts.
func TestRelationPublicationDifferentialCorpus(t *testing.T) {
	sources := relationChecktestCorpus(t)
	if len(sources) == 0 {
		t.Fatal("checktest corpus has no literal check sources")
	}
	report := &relationDifferentialReport{}
	t.Run("checktest", func(t *testing.T) {
		runRelationDifferentialCorpus(t, report, sources)
	})
	fixtureSources := relationFixtureCorpus(t)
	if len(fixtureSources) == 0 {
		t.Fatal("fixture corpus has no Lua sources")
	}
	t.Run("fixtures", func(t *testing.T) {
		runRelationDifferentialCorpus(t, report, fixtureSources)
	})
	report.assert(t)
}

func runRelationDifferentialCorpus(t *testing.T, report *relationDifferentialReport, sources []relationDifferentialSource) {
	t.Helper()
	for _, source := range sources {
		source := source
		t.Run(source.name, func(t *testing.T) {
			stmts, err := parse.ParseString(source.source, source.name)
			if err != nil {
				// Parse-error fixtures have no analyzed lexical body to retain.
				return
			}
			ctx, cancel := context.WithTimeout(t.Context(), relationDifferentialSourceTimeout)
			defer cancel()
			result, err := RunChunk(stmts, Config{
				Context: ctx,
				Check:   body.Config{Registry: standard.Registry()},
				relationPublicationObserver: func(frozen *transformer.RelationProgram, execution transformer.RelationSolveExecution, published formalLexicalPublishedProgram) error {
					report.observe(source.name, frozen, execution, published)
					return nil
				},
			})
			if root := result.RootResult(); root != nil {
				root.ReleaseTransientTree()
			}
			if err != nil {
				report.addGap("%s: program check: %v", source.name, err)
			}
		})
	}
}

func (r *relationDifferentialReport) assert(t *testing.T) {
	t.Helper()
	sort.Strings(r.gaps)
	summary := fmt.Sprintf("CORPUS SIZE %d; PASS RATE %d/%d", r.corpusSize, r.passed, r.corpusSize)
	t.Log(summary)
	if len(r.gaps) == 0 {
		t.Log("NAMED INEQUALITIES none")
	} else {
		t.Logf("NAMED INEQUALITIES %s", strings.Join(r.gaps, "; "))
	}
	if os.Getenv("GO_LUA_RELATION_DIFFERENTIAL_REPORT") != "" {
		fmt.Printf("%s; NAMED INEQUALITIES %s\n", summary, namedRelationDifferentialGaps(r.gaps))
	}
	if r.corpusSize == 0 || r.passed != r.corpusSize || len(r.gaps) != 0 {
		t.Fatalf("CORPUS SIZE %d; PASS RATE %d/%d; NAMED INEQUALITIES %s", r.corpusSize, r.passed, r.corpusSize, strings.Join(r.gaps, "; "))
	}
}

func namedRelationDifferentialGaps(gaps []string) string {
	if len(gaps) == 0 {
		return "none"
	}
	return strings.Join(gaps, "; ")
}

// relationChecktestCorpus discovers the literal source arguments used by the
// checktest package itself. Dynamic sources deliberately remain with their
// owning unit tests; this corpus is the broad, deterministic literal sweep.
func relationChecktestCorpus(t *testing.T) []relationDifferentialSource {
	t.Helper()
	paths, err := filepath.Glob("../../checktest/*_test.go")
	if err != nil {
		t.Fatalf("discover checktest corpus: %v", err)
	}
	var sources []relationDifferentialSource
	for _, path := range paths {
		fileSet := token.NewFileSet()
		file, err := goparser.ParseFile(fileSet, path, nil, 0)
		if err != nil {
			t.Fatalf("parse checktest source %s: %v", path, err)
		}
		goast.Inspect(file, func(node goast.Node) bool {
			call, ok := node.(*goast.CallExpr)
			if !ok || !relationChecktestCall(call) || len(call.Args) == 0 {
				return true
			}
			literal, ok := call.Args[0].(*goast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			source, err := strconv.Unquote(literal.Value)
			if err != nil {
				t.Fatalf("unquote checktest source %s:%d: %v", path, fileSet.Position(literal.Pos()).Line, err)
			}
			sources = append(sources, relationDifferentialSource{
				name:   fmt.Sprintf("checktest/%s:%d", filepath.Base(path), fileSet.Position(literal.Pos()).Line),
				source: source,
			})
			return true
		})
	}
	sort.Slice(sources, func(i, j int) bool { return sources[i].name < sources[j].name })
	return sources
}

func relationFixtureCorpus(t *testing.T) []relationDifferentialSource {
	t.Helper()
	const root = "../../../../testdata/fixtures"
	var sources []relationDifferentialSource
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || filepath.Ext(path) != ".lua" {
			return nil
		}
		source, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		sources = append(sources, relationDifferentialSource{
			name:   "fixtures/" + filepath.ToSlash(rel),
			source: string(source),
		})
		return nil
	})
	if err != nil {
		t.Fatalf("discover fixture corpus: %v", err)
	}
	sort.Slice(sources, func(i, j int) bool { return sources[i].name < sources[j].name })
	return sources
}

func relationChecktestCall(call *goast.CallExpr) bool {
	ident, ok := call.Fun.(*goast.Ident)
	if !ok {
		return false
	}
	switch ident.Name {
	case "Check", "CheckFile", "CheckAndExport", "CheckFileAndExport":
		return true
	default:
		return false
	}
}
