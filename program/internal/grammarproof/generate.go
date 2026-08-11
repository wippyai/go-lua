package grammarproof

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	goast "go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"

	programlower "github.com/wippyai/go-lua/program/lower"
)

// Generate recreates cold grammar-reduction evidence. The parser is copied to
// a temporary directory, generated there, and discarded. No production parser
// source, generated parser, AST, or lowering path is instrumented.
func Generate(root, out string, check bool) error {
	codecPath := filepath.Join(root, astCodecRelativePath)
	codec, err := renderASTCodec(root)
	if err != nil {
		return err
	}
	if check {
		current, readErr := os.ReadFile(codecPath)
		if readErr != nil || !bytes.Equal(current, codec) {
			return fmt.Errorf("grammarproof AST codec is stale: run go generate ./program/internal/grammarproof")
		}
		if err := ValidateGeneratedASTCodec(root); err != nil {
			return err
		}
		return validateGeneratedEvidence(root, out)
	}
	if err := os.WriteFile(codecPath, codec, 0o644); err != nil {
		return fmt.Errorf("write AST codec: %w", err)
	}
	snapshot, err := Collect(root)
	if err != nil {
		return err
	}
	rendered, err := renderEvidence(snapshot.Evidence)
	if err != nil {
		return err
	}
	if err := os.WriteFile(out, rendered, 0o644); err != nil {
		return fmt.Errorf("write evidence: %w", err)
	}
	return nil
}

// validateGeneratedEvidence is the routine freshness proof. The reduction
// probe itself is cold generation work: its complete parser inputs and probe
// protocol are committed by TraceDigest, while this path independently
// revalidates every grammar/corpus denominator and re-runs public ingress for
// every source to verify the sealed Program identities. A stale trace cannot
// pass by resemblance, but an unchanged proof need not rebuild a temporary
// parser on every ordinary test run.
func validateGeneratedEvidence(root, out string) error {
	grammar, err := extractGrammar(filepath.Join(root, "compiler", "parse", "parser.go.y"))
	if err != nil {
		return fmt.Errorf("extract parser grammar: %w", err)
	}
	sources, err := corpus(root)
	if err != nil {
		return err
	}
	if err := validateSources(sources); err != nil {
		return err
	}
	digest := evidenceDigest(grammar, sources)
	traceDigest, err := traceInputDigest(root, sources)
	if err != nil {
		return err
	}
	if err := Generated.Validate(liveFromGrammar(grammar), sources, digest, traceDigest); err != nil {
		return err
	}
	ingress, err := collectIngress(sources)
	if err != nil {
		return err
	}
	if Generated.IngressDigest != ingressDigest(ingress) {
		return fmt.Errorf("grammar ingress evidence is stale: run go generate ./program/internal/grammarproof")
	}
	rendered, err := renderEvidence(Generated)
	if err != nil {
		return err
	}
	current, err := os.ReadFile(out)
	if err != nil || !bytes.Equal(current, rendered) {
		return fmt.Errorf("grammar reduction evidence is stale: run go generate ./program/internal/grammarproof")
	}
	return nil
}

// Collect derives one coherent cold proof snapshot. It is the sole path that
// joins parser reductions, semantic traces, and public ingress results, so a
// downstream matrix cannot accidentally combine evidence from different
// corpus or grammar revisions.
func Collect(root string) (Snapshot, error) {
	grammar, err := extractGrammar(filepath.Join(root, "compiler", "parse", "parser.go.y"))
	if err != nil {
		return Snapshot{}, fmt.Errorf("extract parser grammar: %w", err)
	}
	sources, err := corpus(root)
	if err != nil {
		return Snapshot{}, err
	}
	if err := validateSources(sources); err != nil {
		return Snapshot{}, err
	}
	manifest, err := traceInputManifest(root, sources)
	if err != nil {
		return Snapshot{}, err
	}
	reductions, reductionKeys, rejected, semantics, err := traceReductionsWithManifest(root, manifest)
	if err != nil {
		return Snapshot{}, err
	}
	if rejected := rejectedRequired(sources, rejected); rejected != "" {
		return Snapshot{}, fmt.Errorf("required grammar witness source %s was rejected", rejected)
	}
	if err := validateSemanticTrace(semantics); err != nil {
		return Snapshot{}, err
	}
	ingress, err := collectIngress(sources)
	if err != nil {
		return Snapshot{}, err
	}
	live, err := attachReductions(liveFromGrammar(grammar), allGrammarKeys(grammar), reductionKeys)
	if err != nil {
		return Snapshot{}, err
	}
	traceDigest := traceManifestDigest(manifest)
	evidence, err := buildEvidence(live, evidenceDigest(grammar, sources), traceDigest, reductions, ingress)
	if err != nil {
		return Snapshot{}, err
	}
	traces, err := resolveSemanticTraces(live, semantics)
	if err != nil {
		return Snapshot{}, err
	}
	return Snapshot{Evidence: evidence, Traces: traces, Corpus: corpusSources(sources)}, nil
}

// CorpusSources returns the ordered complete grammar witness corpus as cold
// evidence input. The returned source text must never be used to widen parser
// acceptance or define a Program relation.
func CorpusSources(root string) ([]CorpusSource, error) {
	sources, err := corpus(root)
	if err != nil {
		return nil, err
	}
	if err := validateSources(sources); err != nil {
		return nil, err
	}
	return corpusSources(sources), nil
}

func corpusSources(sources []source) []CorpusSource {
	result := make([]CorpusSource, len(sources))
	for index, source := range sources {
		result[index] = CorpusSource{ID: source.id, Text: source.text}
	}
	return result
}

func validateSources(sources []source) error {
	seen := make(map[string]bool, len(sources))
	for _, source := range sources {
		if source.id == "" {
			return fmt.Errorf("grammar witness corpus has an empty source ID")
		}
		if seen[source.id] {
			return fmt.Errorf("grammar witness corpus duplicates source %s", source.id)
		}
		seen[source.id] = true
	}
	return nil
}

type traceManifestEntry struct {
	name        string
	destination string
	contents    []byte
}

type traceExecutable struct {
	lookupPath   string
	resolvedPath string
	contents     []byte
}

type traceManifest struct {
	entries      []traceManifestEntry
	goExecutable traceExecutable
	goyacc       traceExecutable
	commandEnv   []string
}

// traceInputManifest is the one manifest used both to assemble the isolated
// temporary parser module and to compute TraceDigest. A newly imported local
// parser input therefore either joins this list (and is hashed/copied) or
// makes the temporary build fail closed instead of silently using the shipped
// module copy.
func traceInputManifest(root string, sources []source) (traceManifest, error) {
	var manifest []traceManifestEntry
	appendFile := func(name, relative, destination string) error {
		contents, err := os.ReadFile(filepath.Join(root, relative))
		if err != nil {
			return fmt.Errorf("read grammar proof trace input %s: %w", relative, err)
		}
		manifest = append(manifest, traceManifestEntry{name: name, destination: destination, contents: contents})
		return nil
	}
	appendBytes := func(name, destination string, contents []byte) {
		manifest = append(manifest, traceManifestEntry{name: name, destination: destination, contents: append([]byte(nil), contents...)})
	}
	for _, item := range []struct {
		name, relative, destination string
	}{
		{"module:go.mod", "go.mod", "go.mod"},
		{"module:go.sum", "go.sum", "go.sum"},
		{"parser:grammar", "compiler/parse/parser.go.y", "program/parse/parser.go.y"},
		{"parser:canonical", "compiler/parse/parser.go", "canonical/parse/parser.go"},
		{"parser:lexer", "compiler/parse/lexer.go", "program/parse/lexer.go"},
		{"proof:generate", "program/internal/grammarproof/generate.go", ""},
		{"proof:astcodec-generate", "program/internal/grammarproof/astcodec_generate.go", ""},
		{"proof:astcodec", "program/internal/grammarproof/astcodec/codec.go", "program/internal/grammarproof/astcodec/codec.go"},
		{"proof:astcodec-generated", astCodecRelativePath, "program/internal/grammarproof/astcodec/codec_gen.go"},
		{"proof:model", "program/internal/grammarproof/model.go", ""},
	} {
		if err := appendFile(item.name, item.relative, item.destination); err != nil {
			return traceManifest{}, err
		}
	}
	for _, directory := range []struct {
		name, relative, destination string
	}{
		{"ast", "compiler/ast", "compiler/ast"},
		{"source", "compiler/source", "compiler/source"},
		{"numparse", "compiler/parse/numparse", "compiler/parse/numparse"},
	} {
		entries, err := os.ReadDir(filepath.Join(root, directory.relative))
		if err != nil {
			return traceManifest{}, fmt.Errorf("read grammar proof trace directory %s: %w", directory.relative, err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			relative := filepath.Join(directory.relative, entry.Name())
			name := filepath.ToSlash(relative)
			destination := filepath.Join(directory.destination, entry.Name())
			if err := appendFile(name, relative, destination); err != nil {
				return traceManifest{}, err
			}
		}
	}
	appendBytes("protocol:trace-hook", "program/parse/grammarproof_trace.go", []byte(traceHook))
	appendBytes("protocol:trace-test", "program/parse/grammarproof_trace_test.go", []byte(traceTest))
	corpus, err := json.Marshal(traceSources(sources))
	if err != nil {
		return traceManifest{}, fmt.Errorf("encode grammar proof trace corpus: %w", err)
	}
	appendBytes("corpus", "corpus.json", corpus)
	goExecutable, err := resolveTraceExecutable("go")
	if err != nil {
		return traceManifest{}, err
	}
	goyaccExecutable, err := resolveTraceExecutable("goyacc")
	if err != nil {
		return traceManifest{}, err
	}
	commandEnv := traceCommandEnvironment()
	toolchain := []string{runtime.Version(), runtime.Compiler, runtime.GOOS, runtime.GOARCH}
	for _, key := range []string{"GOWORK", "GOTOOLCHAIN", "GOENV", "GOFLAGS", "GOEXPERIMENT", "CGO_ENABLED"} {
		toolchain = append(toolchain, key+"="+traceEnvironmentValue(commandEnv, key))
	}
	appendBytes("toolchain:go", "", traceExecutableManifest(goExecutable))
	appendBytes("toolchain:goyacc", "", traceExecutableManifest(goyaccExecutable))
	appendBytes("toolchain:environment", "", []byte(strings.Join(toolchain, "\x00")))
	names := make(map[string]bool, len(manifest))
	destinations := make(map[string]bool, len(manifest))
	for _, entry := range manifest {
		if entry.name == "" || names[entry.name] {
			return traceManifest{}, fmt.Errorf("grammar proof trace manifest has duplicate input %q", entry.name)
		}
		names[entry.name] = true
		if entry.destination != "" {
			if destinations[entry.destination] {
				return traceManifest{}, fmt.Errorf("grammar proof trace manifest has duplicate destination %q", entry.destination)
			}
			destinations[entry.destination] = true
		}
	}
	return traceManifest{entries: manifest, goExecutable: goExecutable, goyacc: goyaccExecutable, commandEnv: commandEnv}, nil
}

func resolveTraceExecutable(name string) (traceExecutable, error) {
	lookup, err := exec.LookPath(name)
	if err != nil {
		return traceExecutable{}, fmt.Errorf("locate %s for grammar proof trace: %w", name, err)
	}
	lookup, err = filepath.Abs(lookup)
	if err != nil {
		return traceExecutable{}, fmt.Errorf("resolve %s path for grammar proof trace: %w", name, err)
	}
	resolved, err := filepath.EvalSymlinks(lookup)
	if err != nil {
		return traceExecutable{}, fmt.Errorf("resolve %s executable for grammar proof trace: %w", name, err)
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return traceExecutable{}, fmt.Errorf("absolutize %s executable for grammar proof trace: %w", name, err)
	}
	contents, err := os.ReadFile(resolved)
	if err != nil {
		return traceExecutable{}, fmt.Errorf("read %s executable for grammar proof trace: %w", name, err)
	}
	return traceExecutable{lookupPath: lookup, resolvedPath: resolved, contents: contents}, nil
}

func traceExecutableManifest(executable traceExecutable) []byte {
	contents := make([]byte, 0, len(executable.lookupPath)+len(executable.resolvedPath)+2+len(executable.contents))
	contents = append(contents, filepath.ToSlash(executable.lookupPath)...)
	contents = append(contents, 0)
	contents = append(contents, filepath.ToSlash(executable.resolvedPath)...)
	contents = append(contents, 0)
	contents = append(contents, executable.contents...)
	return contents
}

func traceCommandEnvironment() []string {
	forced := map[string]string{
		"GOWORK":      "off",
		"GOTOOLCHAIN": "local",
		"GOENV":       "off",
	}
	environment := make([]string, 0, len(os.Environ())+len(forced))
	for _, entry := range os.Environ() {
		key, _, ok := strings.Cut(entry, "=")
		if ok {
			if _, replace := forced[key]; replace {
				continue
			}
		}
		environment = append(environment, entry)
	}
	keys := make([]string, 0, len(forced))
	for key := range forced {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		environment = append(environment, key+"="+forced[key])
	}
	return environment
}

func traceEnvironmentValue(environment []string, key string) string {
	prefix := key + "="
	for _, entry := range environment {
		if strings.HasPrefix(entry, prefix) {
			return strings.TrimPrefix(entry, prefix)
		}
	}
	return ""
}

// traceInputDigest commits every entry used to assemble the cold parser and
// semantic probe. It intentionally frames logical source and destination
// names before hashing bytes so two boundaries cannot blur together.
func traceInputDigest(root string, sources []source) (string, error) {
	manifest, err := traceInputManifest(root, sources)
	if err != nil {
		return "", err
	}
	return traceManifestDigest(manifest), nil
}

func traceManifestDigest(manifest traceManifest) string {
	hash := sha256.New()
	for _, entry := range manifest.entries {
		name := entry.name
		if entry.destination != "" {
			name += "=>" + filepath.ToSlash(entry.destination)
		}
		fmt.Fprintf(hash, "%s\x00%d\x00", name, len(entry.contents))
		hash.Write(entry.contents)
		hash.Write([]byte{'\n'})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func traceReductions(root string, sources []source) (map[int][]string, map[int]string, []string, []traceSemantic, error) {
	manifest, err := traceInputManifest(root, sources)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	return traceReductionsWithManifest(root, manifest)
}

func traceReductionsWithManifest(root string, manifest traceManifest) (map[int][]string, map[int]string, []string, []traceSemantic, error) {
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("resolve grammarproof root: %w", err)
	}
	root = absoluteRoot
	// Keep the throw-away module below its program subtree so it may import the
	// cold internal/grammarproof/astcodec package under Go's internal visibility
	// rule. It is still outside every shipped package and is removed before this
	// function returns.
	temporary, err := os.MkdirTemp(filepath.Join(root, "program"), ".grammarproof-")
	if err != nil {
		return nil, nil, nil, nil, err
	}
	defer os.RemoveAll(temporary)
	for _, entry := range manifest.entries {
		if entry.destination == "" {
			continue
		}
		path := filepath.Join(temporary, entry.destination)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, nil, nil, nil, err
		}
		if err := os.WriteFile(path, entry.contents, 0o644); err != nil {
			return nil, nil, nil, nil, err
		}
	}
	parseDirectory := filepath.Join(temporary, "program", "parse")
	if err := os.MkdirAll(parseDirectory, 0o755); err != nil {
		return nil, nil, nil, nil, err
	}
	// The trace invocation differs from the checked-in generation only in its
	// generated banner. Comparing the complete body after one -v generation
	// proves the shipped parser source exactly while also producing y.output;
	// a second identical goyacc run cannot strengthen that equality.
	command := exec.Command(manifest.goyacc.resolvedPath, "-v", "y.output", "-o", "parser.go", "parser.go.y")
	command.Dir = parseDirectory
	command.Env = append([]string(nil), manifest.commandEnv...)
	if output, runErr := command.CombinedOutput(); runErr != nil {
		return nil, nil, nil, nil, fmt.Errorf("generate temporary trace parser: %w: %s", runErr, output)
	}
	traceParser, err := os.ReadFile(filepath.Join(parseDirectory, "parser.go"))
	if err != nil {
		return nil, nil, nil, nil, err
	}
	canonicalParser, err := os.ReadFile(filepath.Join(temporary, "canonical", "parse", "parser.go"))
	if err != nil {
		return nil, nil, nil, nil, err
	}
	if !sameGeneratedBody(canonicalParser, traceParser) {
		return nil, nil, nil, nil, fmt.Errorf("goyacc -v trace parser differs from canonical parser beyond its generated header")
	}
	reductionKeys, err := reductionKeysFromYOutput(filepath.Join(parseDirectory, "y.output"))
	if err != nil {
		return nil, nil, nil, nil, err
	}
	instrumented, targets, err := instrumentTraceParser(traceParser)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	if err := os.WriteFile(filepath.Join(parseDirectory, "parser.go"), instrumented, 0o644); err != nil {
		return nil, nil, nil, nil, err
	}
	typedObserver, err := renderTraceObserver(targets)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	if err := os.WriteFile(filepath.Join(parseDirectory, "grammarproof_typed.go"), typedObserver, 0o644); err != nil {
		return nil, nil, nil, nil, err
	}
	corpusPath := filepath.Join(temporary, "corpus.json")
	resultPath := filepath.Join(temporary, "result.json")
	// This nested go test is part of the cold proof. Generation does not waive
	// repository test-safety; callers must run the outer command through
	// scripts/bounded_test.sh so its complete child process tree is bounded.
	command = exec.Command(manifest.goExecutable.resolvedPath, "test", ".", "-run", "^TestGrammarproofTrace$", "-count=1")
	command.Dir = parseDirectory
	command.Env = append(append([]string(nil), manifest.commandEnv...), "GRAMMARPROOF_CORPUS="+corpusPath, "GRAMMARPROOF_RESULT="+resultPath)
	if output, runErr := command.CombinedOutput(); runErr != nil {
		return nil, nil, nil, nil, fmt.Errorf("run temporary parser evidence: %w: %s", runErr, output)
	}
	contents, err := os.ReadFile(resultPath)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	var result traceResult
	if err := json.Unmarshal(contents, &result); err != nil {
		return nil, nil, nil, nil, err
	}
	return result.Reductions, reductionKeys, result.Rejected, result.Semantics, nil
}

// SemanticTraces returns parser observations only after an independently
// grammar-derived witness corpus has exercised every accepted alternative.
// Callers must never use the returned observations to define their required
// set: a trace is solely evidence that an already-required occurrence ran.
func SemanticTraces(root string) ([]SemanticTrace, error) {
	snapshot, err := Collect(root)
	if err != nil {
		return nil, err
	}
	return append([]SemanticTrace(nil), snapshot.Traces...), nil
}

func resolveSemanticTraces(live []liveProduction, traces []traceSemantic) ([]SemanticTrace, error) {
	byReduction := make(map[int]string, len(live))
	for _, production := range live {
		byReduction[production.reduction] = production.key
	}
	result := make([]SemanticTrace, 0, len(traces))
	for _, trace := range traces {
		key, exists := byReduction[trace.Production]
		if !exists {
			return nil, fmt.Errorf("semantic parser trace has unknown reduction %d", trace.Production)
		}
		if trace.Source == "" || len(trace.Occurrences) == 0 {
			return nil, fmt.Errorf("semantic parser trace has incomplete observed occurrence")
		}
		result = append(result, SemanticTrace{Production: key, Source: trace.Source, Occurrences: trace.Occurrences})
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].Production != result[right].Production {
			return result[left].Production < result[right].Production
		}
		return result[left].Source < result[right].Source
	})
	return result, nil
}

func validateSemanticTrace(traces []traceSemantic) error {
	if len(traces) == 0 {
		return fmt.Errorf("semantic parser trace has no observed AST occurrences")
	}
	for _, trace := range traces {
		if trace.Source == "" || trace.Production <= 0 || len(trace.Occurrences) == 0 {
			return fmt.Errorf("semantic parser trace has incomplete observation")
		}
		for _, occurrence := range trace.Occurrences {
			if occurrence.Type == "" {
				return fmt.Errorf("semantic parser trace has unnamed AST occurrence")
			}
			for _, field := range occurrence.Fields {
				if field.Name == "" || field.State == FieldStateInvalid || field.State > FieldStateNonZero {
					return fmt.Errorf("semantic parser trace has invalid field state for %s", occurrence.Type)
				}
			}
		}
	}
	return nil
}

func sameGeneratedBody(left, right []byte) bool {
	leftBreak := bytes.IndexByte(left, '\n')
	rightBreak := bytes.IndexByte(right, '\n')
	if leftBreak < 0 || rightBreak < 0 {
		return false
	}
	return bytes.Equal(left[leftBreak+1:], right[rightBreak+1:])
}

var yOutputProduction = regexp.MustCompile(`^\s*([A-Za-z_][A-Za-z0-9_]*):.*\s\(([0-9]+)\)\s*$`)

func reductionKeysFromYOutput(path string) (map[int]string, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	nonterminals := make(map[int]string)
	for _, line := range strings.Split(string(contents), "\n") {
		match := yOutputProduction.FindStringSubmatch(line)
		if len(match) == 0 {
			continue
		}
		var number int
		if _, err := fmt.Sscanf(match[2], "%d", &number); err != nil {
			return nil, fmt.Errorf("parse y.output reduction %q: %w", match[2], err)
		}
		if prior, exists := nonterminals[number]; exists {
			if prior != match[1] {
				return nil, fmt.Errorf("y.output reduction %d has conflicting nonterminals %s and %s", number, prior, match[1])
			}
			continue
		}
		nonterminals[number] = match[1]
	}
	if len(nonterminals) == 0 {
		return nil, fmt.Errorf("temporary y.output contains no productions")
	}
	numbers := make([]int, 0, len(nonterminals))
	for number := range nonterminals {
		numbers = append(numbers, number)
	}
	sort.Ints(numbers)
	ordinals := make(map[string]int)
	byNumber := make(map[int]string, len(numbers))
	for _, number := range numbers {
		nonterminal := nonterminals[number]
		ordinals[nonterminal]++
		byNumber[number] = fmt.Sprintf("%s#%d", nonterminal, ordinals[nonterminal])
	}
	return byNumber, nil
}

func attachReductions(live []liveProduction, all map[string]bool, keys map[int]string) ([]liveProduction, error) {
	byKey := make(map[string]int, len(keys))
	for reduction, key := range keys {
		if !all[key] {
			return nil, fmt.Errorf("temporary y.output has unknown grammar production %s", key)
		}
		if _, exists := byKey[key]; exists {
			return nil, fmt.Errorf("temporary y.output maps multiple reductions to %s", key)
		}
		byKey[key] = reduction
	}
	if len(byKey) != len(all) {
		return nil, fmt.Errorf("temporary y.output has %d productions, extracted grammar has %d", len(byKey), len(all))
	}
	for index := range live {
		reduction, exists := byKey[live[index].key]
		if !exists {
			return nil, fmt.Errorf("extracted grammar production %s has no goyacc reduction", live[index].key)
		}
		live[index].reduction = reduction
	}
	return live, nil
}

func rejectedRequired(sources []source, rejected []string) string {
	required := make(map[string]bool, len(sources))
	for _, source := range sources {
		required[source.id] = source.required
	}
	for _, id := range rejected {
		if required[id] {
			return id
		}
	}
	return ""
}

func instrumentTraceParser(contents []byte) ([]byte, map[int][]string, error) {
	const marker = "\tyynt := yyn\n"
	const replacement = "\tyynt := yyn\n\tgrammarproofReduction(yylex, yynt)\n"
	if bytes.Count(contents, []byte(marker)) != 1 {
		return nil, nil, fmt.Errorf("temporary goyacc parser has no unique reduction hook marker")
	}
	contents = bytes.Replace(contents, []byte(marker), []byte(replacement), 1)

	// Observe yyVAL only after its generated semantic action has run. This
	// returns a proof-only parser image; the checked-in parser was verified
	// byte-for-byte before instrumentation and is never modified.
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, "parser.go", contents, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("parse temporary generated parser for semantic hook: %w", err)
	}
	target, err := uniqueYYNTSwitch(file)
	if err != nil {
		return nil, nil, err
	}
	type insertion struct{ offset int }
	insertions := make([]insertion, 0, len(target.Body.List))
	for _, item := range target.Body.List {
		clause, ok := item.(*goast.CaseClause)
		if !ok || len(clause.List) == 0 || len(clause.Body) == 0 {
			continue
		}
		last := clause.Body[len(clause.Body)-1]
		position := fileSet.PositionFor(last.End(), false)
		if position.Offset <= 0 || position.Offset > len(contents) {
			return nil, nil, fmt.Errorf("temporary goyacc parser has invalid semantic-action position")
		}
		insertions = append(insertions, insertion{offset: position.Offset})
	}
	if len(insertions) == 0 {
		return nil, nil, fmt.Errorf("temporary goyacc parser semantic switch has no action cases")
	}
	sort.Slice(insertions, func(left, right int) bool { return insertions[left].offset > insertions[right].offset })
	for _, insertion := range insertions {
		const hook = "\n\t\tgrammarproofSemantic(yynt, yyVAL)"
		contents = append(contents[:insertion.offset], append([]byte(hook), contents[insertion.offset:]...)...)
	}
	targets, err := traceSemanticTargets(target)
	if err != nil {
		return nil, nil, err
	}
	return contents, targets, nil
}

func uniqueYYNTSwitch(file *goast.File) (*goast.SwitchStmt, error) {
	var targets []*goast.SwitchStmt
	goast.Inspect(file, func(node goast.Node) bool {
		statement, ok := node.(*goast.SwitchStmt)
		if !ok {
			return true
		}
		ident, ok := statement.Tag.(*goast.Ident)
		if ok && ident.Name == "yynt" {
			targets = append(targets, statement)
		}
		return true
	})
	if len(targets) != 1 {
		if len(targets) == 0 {
			return nil, fmt.Errorf("temporary goyacc parser has no semantic switch tagged yynt")
		}
		return nil, fmt.Errorf("temporary goyacc parser has %d semantic switches tagged yynt; expected exactly one", len(targets))
	}
	return targets[0], nil
}

func traceSemanticTargets(target *goast.SwitchStmt) (map[int][]string, error) {
	result := make(map[int][]string)
	for _, item := range target.Body.List {
		clause, ok := item.(*goast.CaseClause)
		if !ok || len(clause.List) == 0 {
			continue
		}
		fields := make(map[string]bool)
		var scanErr error
		addField := func(field, form string) bool {
			if field == "" {
				scanErr = fmt.Errorf("temporary parser has unsupported yyVAL mutation or alias form %s", form)
				return false
			}
			if _, supported := traceObserverField(field); !supported {
				scanErr = fmt.Errorf("temporary parser observes unsupported yyVAL field %s via %s", field, form)
				return false
			}
			fields[field] = true
			return true
		}
		goast.Inspect(&goast.BlockStmt{List: clause.Body}, func(node goast.Node) bool {
			if scanErr != nil {
				return false
			}
			var expressions []goast.Expr
			switch statement := node.(type) {
			case *goast.AssignStmt:
				expressions = statement.Lhs
			case *goast.IncDecStmt:
				expressions = []goast.Expr{statement.X}
			case *goast.RangeStmt:
				if statement.Key != nil {
					expressions = append(expressions, statement.Key)
				}
				if statement.Value != nil {
					expressions = append(expressions, statement.Value)
				}
			case *goast.SendStmt:
				expressions = []goast.Expr{statement.Chan}
			case *goast.CallExpr:
				for _, argument := range statement.Args {
					if field, form, rooted := traceYYVALMutationField(argument); rooted {
						if !addField(field, form) {
							return false
						}
						continue
					}
					if _, form, touched := traceYYVALReference(argument); touched {
						scanErr = fmt.Errorf("temporary parser passes an indeterminate yyVAL alias via call argument %s", form)
						return false
					}
				}
				if receiver, ok := traceMethodReceiver(statement.Fun); ok {
					expressions = []goast.Expr{receiver}
				}
			case *goast.UnaryExpr:
				if statement.Op == token.AND {
					if field, form, rooted := traceYYVALMutationField(statement); rooted {
						if !addField(field, form) {
							return false
						}
					} else if _, form, touched := traceYYVALReference(statement.X); touched {
						scanErr = fmt.Errorf("temporary parser takes an indeterminate yyVAL alias via address form %s", form)
						return false
					}
				}
			}
			for _, expression := range expressions {
				field, form, touched := traceYYVALMutationField(expression)
				if !touched {
					continue
				}
				if !addField(field, form) {
					return false
				}
			}
			return true
		})
		if scanErr != nil {
			return nil, scanErr
		}
		if len(fields) == 0 {
			continue
		}
		ordered := make([]string, 0, len(fields))
		for field := range fields {
			ordered = append(ordered, field)
		}
		sort.Strings(ordered)
		for _, expression := range clause.List {
			literal, ok := expression.(*goast.BasicLit)
			if !ok || literal.Kind != token.INT {
				return nil, fmt.Errorf("temporary parser has non-integer reduction case")
			}
			var production int
			if _, err := fmt.Sscanf(literal.Value, "%d", &production); err != nil || production <= 0 {
				return nil, fmt.Errorf("temporary parser has invalid reduction case %q", literal.Value)
			}
			if prior, exists := result[production]; exists && !sameStrings(prior, ordered) {
				return nil, fmt.Errorf("temporary parser assigns conflicting yyVAL fields for reduction %d", production)
			}
			result[production] = ordered
		}
	}
	return result, nil
}

// traceYYVALMutationField classifies every generated-action mutation shape
// rooted at yyVAL. Direct field assignments are the common case; indexed,
// nested, method, and inc/dec forms are still classified by their union-field
// root so no yyVAL mutation can disappear from the observer silently.
func traceYYVALMutationField(expression goast.Expr) (field, form string, touched bool) {
	switch value := expression.(type) {
	case *goast.Ident:
		if value.Name == "yyVAL" {
			return "", "whole-union", true
		}
	case *goast.SelectorExpr:
		if ident, ok := value.X.(*goast.Ident); ok && ident.Name == "yyVAL" {
			return value.Sel.Name, "direct-field", true
		}
		field, _, touched := traceYYVALMutationField(value.X)
		if touched {
			return field, "nested-selector", true
		}
	case *goast.IndexExpr:
		field, _, touched := traceYYVALMutationField(value.X)
		if touched {
			return field, "indexed", true
		}
	case *goast.IndexListExpr:
		field, _, touched := traceYYVALMutationField(value.X)
		if touched {
			return field, "indexed", true
		}
	case *goast.ParenExpr:
		return traceYYVALMutationField(value.X)
	case *goast.StarExpr:
		field, _, touched := traceYYVALMutationField(value.X)
		if touched {
			return field, "dereferenced", true
		}
	case *goast.UnaryExpr:
		if value.Op == token.AND {
			field, _, touched := traceYYVALMutationField(value.X)
			if touched {
				return field, "address-taken", true
			}
		}
	}
	return "", "unrelated", false
}

func traceYYVALReference(expression goast.Expr) (field, form string, touched bool) {
	goast.Inspect(expression, func(node goast.Node) bool {
		candidate, candidateForm, candidateTouched := "", "", false
		if expression, ok := node.(goast.Expr); ok {
			candidate, candidateForm, candidateTouched = traceYYVALMutationField(expression)
		}
		if candidateTouched {
			field, form, touched = candidate, candidateForm, true
			return false
		}
		return true
	})
	return field, form, touched
}

func traceMethodReceiver(expression goast.Expr) (goast.Expr, bool) {
	selector, ok := expression.(*goast.SelectorExpr)
	if !ok {
		return nil, false
	}
	return selector.X, true
}

func renderTraceObserver(targets map[int][]string) ([]byte, error) {
	productions := make([]int, 0, len(targets))
	for production := range targets {
		productions = append(productions, production)
	}
	sort.Ints(productions)
	var out strings.Builder
	out.WriteString("// Code generated by grammarproof; DO NOT EDIT.\npackage parse\n\nfunc grammarproofSemantic(production int, value yySymType) {\n\tswitch production {\n")
	for _, production := range productions {
		out.WriteString("\tcase ")
		out.WriteString(strconv.Itoa(production))
		out.WriteString(":\n")
		for _, field := range targets[production] {
			expression, ok := traceObserverField(field)
			if !ok {
				return nil, fmt.Errorf("temporary parser assigns unsupported semantic field %s", field)
			}
			out.WriteString("\t\t")
			out.WriteString(expression)
			out.WriteByte('\n')
		}
	}
	out.WriteString("\t}\n}\n")
	return format.Source([]byte(out.String()))
}

func traceObserverField(field string) (string, bool) {
	direct := map[string]string{
		"annotation": "grammarproofObserve(production, value.annotation)", "expr": "grammarproofObserve(production, value.expr)",
		"field": "grammarproofObserve(production, value.field)", "funcexpr": "grammarproofObserve(production, value.funcexpr)",
		"funcname": "grammarproofObserve(production, value.funcname)", "funcparam": "grammarproofObserve(production, value.funcparam)",
		"ifacemember": "grammarproofObserve(production, value.ifacemember)", "parlist": "grammarproofObserve(production, value.parlist)",
		"recordfield": "grammarproofObserve(production, value.recordfield)", "stmt": "grammarproofObserve(production, value.stmt)",
		"token": "grammarproofObserve(production, value.token)", "typeparam": "grammarproofObserve(production, value.typeparam)",
		"typeref": "grammarproofObserve(production, value.typeref)", "typeexpr": "grammarproofObserve(production, value.typeexpr)",
	}
	if value, ok := direct[field]; ok {
		return value, true
	}
	tails := map[string]string{
		"annotations": "value.annotations", "exprlist": "value.exprlist", "fieldlist": "value.fieldlist", "recordfields": "value.recordfields", "stmts": "value.stmts", "typeexprlist": "value.typeexprlist", "typeparams": "value.typeparams", "typereflist": "value.typereflist",
	}
	if values, ok := tails[field]; ok {
		return "grammarproofObserveTail(production, " + values + ")", true
	}
	switch field {
	case "callargs":
		return "grammarproofObserveTail(production, value.callargs.values)", true
	case "returntype":
		return "grammarproofObserveTail(production, value.returntype.types)", true
	case "interfacebody":
		return "grammarproofObserveTail(production, value.interfacebody.members)", true
	case "functionparams":
		return "grammarproofObserveTail(production, value.functionparams.Params); grammarproofObserve(production, value.functionparams.Variadic)", true
	case "typedname":
		return "grammarproofObserve(production, value.typedname.Type)", true
	case "typednames":
		return "if len(value.typednames) != 0 { grammarproofObserve(production, value.typednames[len(value.typednames)-1].Type) }", true
	case "fieldsep", "namelist":
		return "", true
	}
	return "", false
}

type traceResult struct {
	Reductions map[int][]string `json:"reductions"`
	Rejected   []string         `json:"rejected"`
	Semantics  []traceSemantic  `json:"semantics"`
}

type traceSemantic struct {
	Source      string          `json:"source"`
	Production  int             `json:"production"`
	Occurrences []ASTOccurrence `json:"occurrences"`
}

type traceSource struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

func traceSources(sources []source) []traceSource {
	result := make([]traceSource, len(sources))
	for index, source := range sources {
		result[index] = traceSource{ID: source.id, Text: source.text}
	}
	return result
}

func collectIngress(sources []source) ([]Ingress, error) {
	result := make([]Ingress, 0, len(sources))
	for _, source := range sources {
		name := source.id + ".lua"
		sealed, err := programlower.Lower(programlower.Source{Name: name, Text: []byte(source.text)})
		if err != nil {
			return nil, fmt.Errorf("grammar ingress lower %s: %w", source.id, err)
		}
		if id := sealed.ContentID(); !id.Available() {
			return nil, fmt.Errorf("grammar ingress seal %s: Program has no content identity", source.id)
		} else {
			result = append(result, Ingress{Source: source.id, ProgramID: id.String()})
		}
	}
	return result, nil
}

func ingressDigest(rows []Ingress) string {
	hash := sha256.New()
	for _, row := range rows {
		fmt.Fprintf(hash, "%s\x00%s\n", row.Source, row.ProgramID)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func buildEvidence(live []liveProduction, digest, traceDigest string, reductions map[int][]string, ingress []Ingress) (Evidence, error) {
	byReduction := make(map[int]liveProduction, len(live))
	for _, production := range live {
		byReduction[production.reduction] = production
	}
	witnesses := make(map[string][]string, len(live))
	for reduction, ids := range reductions {
		production, exists := byReduction[reduction]
		if !exists {
			return Evidence{}, fmt.Errorf("temporary parser reduced unknown production %d", reduction)
		}
		witnesses[production.key] = append(witnesses[production.key], ids...)
	}
	evidence := Evidence{
		Digest:        digest,
		TraceDigest:   traceDigest,
		IngressDigest: ingressDigest(ingress),
		Productions:   make([]Production, 0, len(live)),
		Ingress:       append([]Ingress(nil), ingress...),
	}
	missing := make([]string, 0)
	for _, production := range live {
		ids := uniqueSorted(witnesses[production.key])
		if len(ids) == 0 {
			missing = append(missing, production.key)
			continue
		}
		evidence.Productions = append(evidence.Productions, Production{
			Key:     production.key,
			Witness: ids[0],
		})
	}
	if len(missing) != 0 {
		return Evidence{}, fmt.Errorf("accepted grammar corpus leaves %d uncovered productions: %s", len(missing), strings.Join(missing, ", "))
	}
	return evidence, nil
}

func uniqueSorted(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	sort.Strings(values)
	result := values[:0]
	for _, value := range values {
		if len(result) == 0 || result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	return result
}

func renderEvidence(evidence Evidence) ([]byte, error) {
	var out strings.Builder
	out.WriteString("// Code generated by grammarproof; DO NOT EDIT.\n\npackage grammarproof\n\nvar Generated = Evidence{\n")
	fmt.Fprintf(&out, "\tDigest: %q,\n\tTraceDigest: %q,\n\tProductions: []Production{\n", evidence.Digest, evidence.TraceDigest)
	for _, production := range evidence.Productions {
		fmt.Fprintf(&out, "\t\t{Key: %q, Witness: %q},\n", production.Key, production.Witness)
	}
	out.WriteString("\t},\n\tIngressDigest: ")
	fmt.Fprintf(&out, "%q,\n\tIngress: []Ingress{\n", evidence.IngressDigest)
	for _, ingress := range evidence.Ingress {
		fmt.Fprintf(&out, "\t\t{Source: %q, ProgramID: %q},\n", ingress.Source, ingress.ProgramID)
	}
	out.WriteString("\t},\n}\n")
	return format.Source([]byte(out.String()))
}

const traceHook = `package parse

import "github.com/wippyai/go-lua/program/internal/grammarproof/astcodec"

var grammarproofReductions []int
var grammarproofSemantics []grammarproofSemanticEvent

type grammarproofSemanticField = astcodec.Field
type grammarproofSemanticOccurrence = astcodec.Occurrence

type grammarproofSemanticEvent struct {
	Production int ` + "`json:\"production\"`" + `
	Occurrences []grammarproofSemanticOccurrence ` + "`json:\"occurrences\"`" + `
}

const (
	grammarproofFieldAbsent = iota + 1
	grammarproofFieldPresent
	grammarproofFieldEmpty
	grammarproofFieldNonEmpty
	grammarproofFieldFalse
	grammarproofFieldTrue
	grammarproofFieldZero
	grammarproofFieldNonZero
)

func grammarproofReduction(_ yyLexer, production int) {
	grammarproofReductions = append(grammarproofReductions, production)
}

func grammarproofObserveTail[T any](production int, values []T) {
	if len(values) != 0 { grammarproofObserve(production, values[len(values)-1]) }
}

func grammarproofObserve(production int, value any) {
	occurrence, ok := astcodec.Encode(value)
	if !ok { return }
	grammarproofSemantics = append(grammarproofSemantics, grammarproofSemanticEvent{Production: production, Occurrences: []grammarproofSemanticOccurrence{occurrence}})
}
`

const traceTest = `package parse

import (
	"encoding/json"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
)

type grammarproofSource struct {
	ID string ` + "`json:\"id\"`" + `
	Text string ` + "`json:\"text\"`" + `
}

type grammarproofResult struct {
	Reductions map[int][]string ` + "`json:\"reductions\"`" + `
	Rejected []string ` + "`json:\"rejected\"`" + `
	Semantics []grammarproofSemanticTrace ` + "`json:\"semantics\"`" + `
}

type grammarproofSemanticTrace struct {
	Source string ` + "`json:\"source\"`" + `
	Production int ` + "`json:\"production\"`" + `
	Occurrences []grammarproofSemanticOccurrence ` + "`json:\"occurrences\"`" + `
}

func TestGrammarproofTrace(t *testing.T) {
	contents, err := os.ReadFile(os.Getenv("GRAMMARPROOF_CORPUS"))
	if err != nil { t.Fatal(err) }
	var corpus []grammarproofSource
	if err := json.Unmarshal(contents, &corpus); err != nil { t.Fatal(err) }
	result := grammarproofResult{Reductions: make(map[int][]string)}
	// The trace is a finite witness inventory, not an event log.  Preserve one
	// deterministic source for every exact (reduction, AST product) shape.
	// Retaining every repeat would make a long statement list encode the same
	// semantic product once per element, without adding a denominator row.
	semantic := make(map[string]grammarproofSemanticTrace)
	for _, source := range corpus {
		grammarproofReductions = nil
		grammarproofSemantics = nil
		if !grammarproofAccepted(source.Text, source.ID) {
			result.Rejected = append(result.Rejected, source.ID)
			continue
		}
		for _, reduction := range grammarproofReductions {
			result.Reductions[reduction] = append(result.Reductions[reduction], source.ID)
		}
		for _, event := range grammarproofSemantics {
			for _, occurrence := range event.Occurrences {
				key := grammarproofSemanticKey(event.Production, occurrence)
				if _, exists := semantic[key]; !exists {
					semantic[key] = grammarproofSemanticTrace{
						Source: source.ID, Production: event.Production,
						Occurrences: []grammarproofSemanticOccurrence{occurrence},
					}
				}
			}
		}
	}
	keys := make([]string, 0, len(semantic))
	for key := range semantic { keys = append(keys, key) }
	sort.Strings(keys)
	result.Semantics = make([]grammarproofSemanticTrace, 0, len(keys))
	for _, key := range keys { result.Semantics = append(result.Semantics, semantic[key]) }
	encoded, err := json.Marshal(result)
	if err != nil { t.Fatal(err) }
	if err := os.WriteFile(os.Getenv("GRAMMARPROOF_RESULT"), encoded, 0o644); err != nil { t.Fatal(err) }
}

func grammarproofSemanticKey(production int, occurrence grammarproofSemanticOccurrence) string {
	var out strings.Builder
	out.Grow(len(occurrence.Type) + len(occurrence.Fields)*16)
	out.WriteString(strconv.Itoa(production))
	out.WriteByte(0)
	out.WriteString(occurrence.Type)
	for _, field := range occurrence.Fields {
		out.WriteByte(0)
		out.WriteString(field.Name)
		out.WriteByte(0)
		out.WriteString(strconv.Itoa(int(field.State)))
		out.WriteByte(0)
		out.WriteString(strconv.FormatUint(field.Value, 10))
	}
	return out.String()
}

func grammarproofAccepted(text, name string) (accepted bool) {
	lexer := &Lexer{NewScanner(strings.NewReader(text), name), nil, false, ast.Token{Str: ""}, TNil, nil}
	defer func() {
		if recover() != nil {
			accepted = false
		}
	}()
	return yyParse(lexer) == 0 && lexer.Stmts != nil
}
`
