package engine

// This file is the deliberately narrow bridge between lexical evaluation and
// interproc.  In particular it never gives an in-flight table cell to an
// equation kernel: callback execution may only obtain a candidate through
// RecursiveValues.Read.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/front"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/interproc"
)

const lexicalSCCDemand interproc.DemandKey = "engine/lexical-closed-output/v1"

type lexicalSCCAdmission struct {
	compilation front.Compilation
	artifact    interproc.DemandedBodyArtifact
	entry       []byte // private application sidecar, never part of an outcome
}

type lexicalSCCRun struct {
	discovered map[string]interproc.InstanceKey
	values     *interproc.RecursiveValues
}

type lexicalSCCFact struct {
	Key   string `json:"key"`
	Value []byte `json:"value"`
}

// lexicalSCCWire is intentionally smaller than OutputClosure: diagnostics,
// spans, transaction counts, and caller lenses are replay-only information.
// Values and outcomes use body-owned coordinate names and are re-bound only by
// applyKnown after a cell has closed.
type lexicalSCCWire struct {
	Version  uint8            `json:"version"`
	Values   []lexicalSCCFact `json:"values"`
	Outcomes []lexicalSCCFact `json:"outcomes"`
}

func lexicalSCCMapKey(key interproc.InstanceKey) string { return string(key.CanonicalBytes()) }

func (l *lexicalEvaluator) resolveSCCChild(child front.Compilation, rawEntry []byte, seeds []entrySeed, arguments [][]byte, handle closureHandle, operands directCallOperands, target string) (equation.OutputClosure, error) {
	key, admission, err := l.admitSCC(child, rawEntry, seeds, arguments, handle)
	if err != nil {
		return equation.OutputClosure{}, err
	}
	if l.run != nil {
		if l.run.discovered != nil {
			l.run.discovered[lexicalSCCMapKey(key)] = key
			return lexicalSCCTopClosure(operands, target), nil
		}
		if l.run.values == nil {
			return equation.OutputClosure{}, fmt.Errorf("engine: malformed recursive lexical run")
		}
		candidate, err := l.run.values.Read(key)
		if err != nil {
			return equation.OutputClosure{}, err
		}
		return decodeLexicalSCCOutcome(candidate)
	}
	if l.coordinator == nil {
		return equation.OutputClosure{}, fmt.Errorf("engine: lexical SCC coordinator is unavailable")
	}
	ctx := l.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	outcome, err := l.coordinator.Resolve(ctx, key, lexicalSCCEvaluator{lexical: l}, lexicalSCCLattice{height: lexicalSCCHeight(admission.compilation)})
	if err != nil {
		return equation.OutputClosure{}, err
	}
	closed, err := decodeLexicalSCCOutcome(outcome)
	if err != nil {
		return equation.OutputClosure{}, err
	}
	// Diagnostics are deliberately outside the approximation lattice.  Replay
	// only after Resolve has atomically closed the reachable SCC; nested calls
	// are table hits at this point, never partial reads.
	// Replay the closed body only for diagnostics.  Keep nested lexical calls in
	// discovery mode: re-entering Resolve here would recursively trigger another
	// diagnostics replay after the same cell has already closed.
	replay, _, err := l.runSCCBody(ctx, admission, &lexicalSCCRun{discovered: make(map[string]interproc.InstanceKey)})
	if err != nil {
		return equation.OutputClosure{}, err
	}
	closed.Diagnostics = replay.Diagnostics
	return closed, nil
}

// admitSCC constructs the certified semantic entry.  The raw child-entry
// packet is retained only for the VM bind; the key contains boundary ordinals,
// exact value encodings, and alias classes, never caller path spellings.
func (l *lexicalEvaluator) admitSCC(child front.Compilation, rawEntry []byte, seeds []entrySeed, arguments [][]byte, handle closureHandle) (interproc.InstanceKey, lexicalSCCAdmission, error) {
	artifact, err := lexicalSCCArtifact(child)
	if err != nil {
		return interproc.InstanceKey{}, lexicalSCCAdmission{}, err
	}
	if len(seeds) != len(child.Boundary.Parameters)+len(child.Boundary.Captures) {
		return interproc.InstanceKey{}, lexicalSCCAdmission{}, fmt.Errorf("engine: incomplete lexical SCC boundary")
	}
	classes := make(map[string]uint64)
	nextClass := uint64(0)
	values := make([]interproc.EntryValue, 0, len(seeds))
	for index, seed := range seeds {
		lens := ""
		if index < len(arguments) {
			lens = string(arguments[index])
		} else {
			lens = handle.Captures[index-len(arguments)]
		}
		class, ok := classes[lens]
		if !ok {
			class, classes[lens], nextClass = nextClass, nextClass, nextClass+1
		}
		encoded, marshalErr := json.Marshal(struct {
			Value []byte `json:"value"`
			Alias uint64 `json:"alias"`
		}{Value: seed.Value, Alias: class})
		if marshalErr != nil {
			return interproc.InstanceKey{}, lexicalSCCAdmission{}, marshalErr
		}
		values = append(values, interproc.EntryValue{Selector: lexicalSCCSelector(index), Encoding: encoded})
	}
	entry, err := interproc.NewEntryBinding(values)
	if err != nil {
		return interproc.InstanceKey{}, lexicalSCCAdmission{}, err
	}
	key, err := interproc.NewInstanceKey(artifact, entry)
	if err != nil {
		return interproc.InstanceKey{}, lexicalSCCAdmission{}, err
	}
	admission := lexicalSCCAdmission{compilation: child, artifact: artifact, entry: append([]byte(nil), rawEntry...)}
	id := lexicalSCCMapKey(key)
	if prior, exists := l.admissions[id]; exists {
		if string(prior.entry) != string(admission.entry) || prior.compilation.Body != child.Body {
			return interproc.InstanceKey{}, lexicalSCCAdmission{}, fmt.Errorf("engine: non-identical lexical entry collided after certification")
		}
		return key, prior, nil
	}
	l.admissions[id] = admission
	return key, admission, nil
}

func lexicalSCCSelector(index int) interproc.EntrySelector {
	return interproc.EntrySelector(fmt.Sprintf("boundary/%08d", index))
}

func lexicalSCCArtifact(compilation front.Compilation) (interproc.DemandedBodyArtifact, error) {
	if !compilation.Body.Valid() || compilation.Frozen.CanonicalBytes() == nil {
		return interproc.DemandedBodyArtifact{}, fmt.Errorf("engine: lexical body lacks frozen SCC artifact")
	}
	n := len(compilation.Boundary.Parameters) + len(compilation.Boundary.Captures)
	selectors := make([]interproc.EntrySelector, n)
	for index := range selectors {
		selectors[index] = lexicalSCCSelector(index)
	}
	schema, err := interproc.NewParameterSchema("lexical/"+fmt.Sprintf("%x", compilation.Body), selectors)
	if err != nil {
		return interproc.DemandedBodyArtifact{}, err
	}
	certificate, err := interproc.NewReadProjectionCertificate(lexicalSCCDemand, interproc.ReadCertificateInputs{Semantic: selectors, EntrySeeding: selectors, CallEntry: selectors})
	if err != nil {
		return interproc.DemandedBodyArtifact{}, err
	}
	bodyID := interproc.ContentIDFromCanonicalBytes(compilation.Frozen.CanonicalBytes())
	manifest, err := interproc.NewDependencyManifest([]interproc.Dependency{{Kind: "lexical-frozen-body", ID: bodyID}})
	if err != nil {
		return interproc.DemandedBodyArtifact{}, err
	}
	solver := interproc.ContentIDFromCanonicalBytes([]byte("engine/lexical-scc-lattice/v1"))
	return interproc.NewDemandedBodyArtifact(compilation.Frozen, schema, lexicalSCCDemand, certificate, solver, manifest, nil)
}

type lexicalSCCEvaluator struct{ lexical *lexicalEvaluator }

func (e lexicalSCCEvaluator) Discover(ctx context.Context, key interproc.InstanceKey) ([]interproc.InstanceKey, error) {
	admission, ok := e.lexical.admissions[lexicalSCCMapKey(key)]
	if !ok {
		return nil, fmt.Errorf("engine: recursive lexical admission is unavailable")
	}
	run := &lexicalSCCRun{discovered: make(map[string]interproc.InstanceKey)}
	if _, _, err := e.lexical.runSCCBody(ctx, admission, run); err != nil {
		return nil, err
	}
	delete(run.discovered, lexicalSCCMapKey(key))
	out := make([]interproc.InstanceKey, 0, len(run.discovered))
	for _, callee := range run.discovered {
		out = append(out, callee)
	}
	sort.Slice(out, func(i, j int) bool { return string(out[i].CanonicalBytes()) < string(out[j].CanonicalBytes()) })
	return out, nil
}

func (e lexicalSCCEvaluator) Evaluate(ctx context.Context, key interproc.InstanceKey, values interproc.RecursiveValues) (interproc.ClosedOutcome, error) {
	admission, ok := e.lexical.admissions[lexicalSCCMapKey(key)]
	if !ok {
		return interproc.ClosedOutcome{}, fmt.Errorf("engine: recursive lexical admission is unavailable")
	}
	closure, _, err := e.lexical.runSCCBody(ctx, admission, &lexicalSCCRun{values: &values})
	if err != nil {
		return interproc.ClosedOutcome{}, err
	}
	return encodeLexicalSCCOutcome(admission.compilation, closure)
}

func (l *lexicalEvaluator) runSCCBody(ctx context.Context, admission lexicalSCCAdmission, run *lexicalSCCRun) (equation.OutputClosure, int, error) {
	priorRun, priorCtx := l.run, l.ctx
	l.run, l.ctx = run, ctx
	defer func() { l.run, l.ctx = priorRun, priorCtx }()
	return l.evaluate(admission.compilation, admission.entry)
}

func lexicalSCCTopClosure(operands directCallOperands, target string) equation.OutputClosure {
	closure := equation.OutputClosure{}
	for index := 0; index < operands.resultArity; index++ {
		closure.Values = append(closure.Values, equation.Fact{Key: fmt.Sprintf("call-result/%s/%08d", target, index), Value: []byte("scalar/top")})
	}
	return closure
}

func encodeLexicalSCCOutcome(compilation front.Compilation, closure equation.OutputClosure) (interproc.ClosedOutcome, error) {
	if compilation.Body.Valid() {
		closure = lexicalSCCSummary(compilation, closure)
	}
	wire := lexicalSCCWire{Version: 1, Values: lexicalSCCFacts(closure.Values), Outcomes: lexicalSCCFacts(closure.Outcomes)}
	bytes, err := json.Marshal(wire)
	if err != nil {
		return interproc.ClosedOutcome{}, err
	}
	return interproc.NewClosedOutcome(bytes)
}

// lexicalSCCSummary removes body-local VM state before it reaches the table.
// A recursive re-evaluation consumes its declared boundary, the closed heap
// graph reachable from that boundary, and return-owned capabilities.  Those
// are all existing publications; allocation and diagnostic detail remains
// replay-only and therefore cannot make a recursive coordinate grow with
// caller history.
func lexicalSCCSummary(compilation front.Compilation, closure equation.OutputClosure) equation.OutputClosure {
	boundary := make([]string, 0, len(compilation.Boundary.Parameters)+len(compilation.Boundary.Captures))
	for _, parameter := range compilation.Boundary.Parameters {
		boundary = append(boundary, boundaryTerm(parameter.Symbol))
	}
	for _, capture := range compilation.Boundary.Captures {
		boundary = append(boundary, boundaryTerm(capture.Symbol))
	}
	matchBoundary := func(key string) bool {
		for _, term := range boundary {
			for _, prefix := range []string{"value/", "closure/", "declared-type/", "epoch/", heapTableIdentityPrefix} {
				if strings.HasPrefix(key, prefix+term+"/") {
					return true
				}
			}
		}
		return false
	}
	values := make([]equation.Fact, 0, len(boundary)*4)
	kept := make(map[string]bool)
	identities := make(map[string][]byte)
	keep := func(fact equation.Fact) {
		if kept[fact.Key] {
			return
		}
		kept[fact.Key] = true
		values = append(values, fact)
	}
	for _, fact := range closure.Values {
		if matchBoundary(fact.Key) || strings.HasPrefix(fact.Key, "effect.lifecycle.channel/") ||
			strings.HasPrefix(fact.Key, "effect.lifecycle.resource/") {
			keep(fact)
		}
		if strings.HasPrefix(fact.Key, heapTableIdentityPrefix) && matchBoundary(fact.Key) && len(fact.Value) != 0 {
			identities[string(fact.Value)] = append([]byte(nil), fact.Value...)
		}
	}
	visited := make(map[string]bool)
	for len(identities) != 0 {
		pending := identities
		identities = make(map[string][]byte)
		for key := range pending {
			visited[key] = true
		}
		for _, fact := range closure.Values {
			identity, ownsHeapFact := lexicalSCCHeapFactIdentity(fact.Key)
			if !ownsHeapFact || pending[string(identity)] == nil {
				continue
			}
			keep(fact)
			if next, found := lexicalSCCHeapChildIdentity(fact); found && !visited[string(next)] && identities[string(next)] == nil {
				identities[string(next)] = next
			}
		}
	}
	outcomes := make([]equation.Fact, 0, len(closure.Outcomes))
	for _, fact := range closure.Outcomes {
		if strings.HasPrefix(fact.Key, "return-candidate/") {
			outcomes = append(outcomes, fact)
		}
	}
	for _, fact := range closure.Values {
		if strings.HasPrefix(fact.Key, "return-member-closure/") {
			keep(fact)
		}
	}
	return equation.OutputClosure{Values: values, Outcomes: outcomes}
}

// lexicalSCCHeapFactIdentity decodes the owning allocation identity from the
// closed heap fact schemas.  It is deliberately schema-driven: a source path
// or a declared type cannot enter a recursive summary as a heap capability.
func lexicalSCCHeapFactIdentity(key string) ([]byte, bool) {
	for _, prefix := range []string{heapTableClosedPrefix, heapMemberPrefix, heapMemberIdentityPrefix, memberCellPrefix, heapMetaAttachedPrefix, heapMetaNewIndexPrefix} {
		rest, found := strings.CutPrefix(key, prefix)
		if !found {
			continue
		}
		encoded, _, found := strings.Cut(rest, "/")
		if !found || encoded == "" {
			return nil, false
		}
		identity, err := base64.RawURLEncoding.DecodeString(encoded)
		return identity, err == nil && len(identity) != 0
	}
	return nil, false
}

// lexicalSCCHeapChildIdentity follows only explicit member-identity and
// member-cell publications.  The worklist is finite because every identity
// is frozen by the compiled body or supplied through the certified entry.
func lexicalSCCHeapChildIdentity(fact equation.Fact) ([]byte, bool) {
	if strings.HasPrefix(fact.Key, heapMemberIdentityPrefix) && len(fact.Value) != 0 {
		return append([]byte(nil), fact.Value...), true
	}
	if !strings.HasPrefix(fact.Key, memberCellPrefix) {
		return nil, false
	}
	var cell memberCellWire
	if json.Unmarshal(fact.Value, &cell) != nil || len(cell.MemberIdentity) == 0 {
		return nil, false
	}
	return append([]byte(nil), cell.MemberIdentity...), true
}

func decodeLexicalSCCOutcome(outcome interproc.ClosedOutcome) (equation.OutputClosure, error) {
	var wire lexicalSCCWire
	if !outcome.Valid() || json.Unmarshal(outcome.CanonicalBytes(), &wire) != nil || wire.Version != 1 {
		return equation.OutputClosure{}, fmt.Errorf("engine: malformed recursive lexical summary")
	}
	values, err := lexicalSCCDecodeFacts(wire.Values)
	if err != nil {
		return equation.OutputClosure{}, err
	}
	outcomes, err := lexicalSCCDecodeFacts(wire.Outcomes)
	if err != nil {
		return equation.OutputClosure{}, err
	}
	return equation.OutputClosure{Values: values, Outcomes: outcomes}, nil
}

func lexicalSCCFacts(facts []equation.Fact) []lexicalSCCFact {
	out := make([]lexicalSCCFact, len(facts))
	for index, fact := range facts {
		out[index] = lexicalSCCFact{Key: fact.Key, Value: append([]byte(nil), fact.Value...)}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

func lexicalSCCDecodeFacts(facts []lexicalSCCFact) ([]equation.Fact, error) {
	out := make([]equation.Fact, len(facts))
	for index, fact := range facts {
		if fact.Key == "" || fact.Value == nil || index > 0 && facts[index-1].Key >= fact.Key {
			return nil, fmt.Errorf("engine: malformed recursive lexical summary fact")
		}
		out[index] = equation.Fact{Key: fact.Key, Value: append([]byte(nil), fact.Value...)}
	}
	return out, nil
}

type lexicalSCCLattice struct{ height uint64 }

func lexicalSCCHeight(compilation front.Compilation) uint64 {
	// The frozen body supplies a finite coordinate inventory.  Each coordinate
	// may move bottom -> exact -> top, hence two strict ascents.
	n := len(compilation.Frozen.Artifact.Equations)*8 + len(compilation.Boundary.Parameters) + len(compilation.Boundary.Captures) + 1
	return uint64(2 * n)
}
func (l lexicalSCCLattice) Height() uint64 { return l.height }
func (l lexicalSCCLattice) Bottom(key interproc.InstanceKey) (interproc.ClosedOutcome, error) {
	return encodeLexicalSCCOutcome(front.Compilation{}, equation.OutputClosure{})
}
func (l lexicalSCCLattice) Join(key interproc.InstanceKey, previous, candidate interproc.ClosedOutcome) (interproc.ClosedOutcome, bool, error) {
	left, err := decodeLexicalSCCOutcome(previous)
	if err != nil {
		return interproc.ClosedOutcome{}, false, err
	}
	right, err := decodeLexicalSCCOutcome(candidate)
	if err != nil {
		return interproc.ClosedOutcome{}, false, err
	}
	values, changed, err := lexicalSCCJoinFacts(left.Values, right.Values, false)
	if err != nil {
		return interproc.ClosedOutcome{}, false, err
	}
	outcomes, outcomeChanged, err := lexicalSCCJoinFacts(left.Outcomes, right.Outcomes, true)
	if err != nil {
		return interproc.ClosedOutcome{}, false, err
	}
	next, err := encodeLexicalSCCOutcome(front.Compilation{}, equation.OutputClosure{Values: values, Outcomes: outcomes})
	return next, changed || outcomeChanged, err
}

func lexicalSCCJoinFacts(previous, candidate []equation.Fact, strict bool) ([]equation.Fact, bool, error) {
	all := make(map[string][]byte, len(previous)+len(candidate))
	for _, fact := range previous {
		if fact.Key == "" || fact.Value == nil {
			return nil, false, fmt.Errorf("engine: malformed recursive fact")
		}
		all[fact.Key] = append([]byte(nil), fact.Value...)
	}
	changed := false
	for _, fact := range candidate {
		if fact.Key == "" || fact.Value == nil {
			return nil, false, fmt.Errorf("engine: malformed recursive fact")
		}
		prior, exists := all[fact.Key]
		if !exists {
			all[fact.Key] = append([]byte(nil), fact.Value...)
			changed = true
			continue
		}
		if string(prior) == string(fact.Value) || string(prior) == "scalar/top" {
			continue
		}
		if strict || len(prior) == 0 {
			return nil, false, fmt.Errorf("engine: incompatible recursive outcome coordinate %q", fact.Key)
		}
		all[fact.Key] = []byte("scalar/top")
		changed = true
	}
	keys := make([]string, 0, len(all))
	for key := range all {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]equation.Fact, 0, len(keys))
	for _, key := range keys {
		out = append(out, equation.Fact{Key: key, Value: all[key]})
	}
	return out, changed, nil
}
