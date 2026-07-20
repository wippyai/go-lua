package body

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding"
	"errors"
	"fmt"
	"hash"
	"hash/fnv"
	"sort"
	"strconv"
	"strings"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/solve"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	internalhash "github.com/wippyai/go-lua/analysis/internal/hash"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// ResultVersion returns the content digest of the body inputs used to produce
// this solved result. It is stable for identical body inputs across independent
// solves and changes when the body's parsed form, consumed type/state inputs, or
// tracked summary dependencies change.
func (r *Result) ResultVersion() uint64 {
	if r == nil {
		return 0
	}
	return r.resultVersion
}

func computeResultVersion(s *Static, config SolveConfig, entry state.State, initial transfer.InitialState) (uint64, error) {
	lineage, err := computeResultVersionLineage(s, config, entry, initial)
	return lineage.ResultVersion(), err
}

func computeResultVersionLineage(s *Static, config SolveConfig, entry state.State, initial transfer.InitialState) (ResultVersionLineage, error) {
	return computeResultVersionLineageWithApplications(s, config, entry, initial, nil)
}

func computeResultVersionLineageWithApplications(s *Static, config SolveConfig, entry state.State, initial transfer.InitialState, applicationDependencies []ApplicationDependency) (ResultVersionLineage, error) {
	if s == nil {
		return ResultVersionLineage{}, nil
	}
	if config.SummaryInputs != nil && config.SummaryInputDigests != nil {
		return ResultVersionLineage{}, ErrConflictingSummaryInputProviders
	}
	prefix, wide, err := staticResultVersionPrefix(s, config.Context)
	if err != nil {
		return ResultVersionLineage{}, err
	}
	inputs, err := canonicalSummaryInputs(config.Context, config.SummaryInputs)
	if err != nil {
		if config.Context != nil && config.Context.Err() != nil {
			return ResultVersionLineage{}, errors.Join(solve.ErrCanceled, err)
		}
		return ResultVersionLineage{}, err
	}
	dependencies, err := canonicalApplicationDependencies(applicationDependencies)
	if err != nil {
		return ResultVersionLineage{}, err
	}
	// Summary payloads, product values, entry/initial states, and several type
	// lanes currently enter the canonical stream through uint64 semantic hashes.
	// Exact dependency capture is useful lineage, but cannot authorize a
	// collision-safe full digest until those existing owners expose canonical
	// bytes/full-width identities. The marker is intentionally outside digest
	// semantics so enabling that producer later does not change ResultVersion.
	complete := false
	w := &bodyDigestWriter{
		h:          prefix,
		wide:       wide,
		static:     s,
		symbols:    make(map[symbol.ID]string),
		ctx:        config.Context,
		stateLanes: state.CloneLanes(config.StateLanes),
	}
	w.writeClosedDynamicInvariants(config.ClosedDynamicAllValues)
	w.writeWidening(config)
	w.writeState("entry", entry)
	w.writeInitialStates(initial, s.cfg.Graph)
	if config.SummaryInputs != nil {
		w.writeSummaryInputs(inputs)
	} else {
		w.writeSummaryInputDigests(config.SummaryInputDigests)
	}
	w.writeApplicationDependencies(dependencies)
	if err := w.err(); err != nil {
		return ResultVersionLineage{}, errors.Join(solve.ErrCanceled, err)
	}
	var digest ResultVersionDigest
	_ = w.wide.Sum(digest[:0])
	return ResultVersionLineage{
		legacy:   w.sum64(),
		digest:   digest,
		inputs:   inputs,
		complete: complete,
	}, nil
}

func canonicalApplicationDependencies(in []ApplicationDependency) ([]ApplicationDependency, error) {
	out := append([]ApplicationDependency(nil), in...)
	sort.Slice(out, func(i, j int) bool {
		left, right := out[i], out[j]
		if left.CallPoint != right.CallPoint {
			return left.CallPoint < right.CallPoint
		}
		if left.CallOccurrence != right.CallOccurrence {
			return left.CallOccurrence < right.CallOccurrence
		}
		if order := bytes.Compare(left.Target[:], right.Target[:]); order != 0 {
			return order < 0
		}
		return left.SemanticVersion < right.SemanticVersion
	})
	write := 0
	for _, dependency := range out {
		if dependency.Target == (lexicalidentity.StableLexicalBodyID{}) || dependency.SemanticVersion == 0 {
			return nil, fmt.Errorf("body: invalid application dependency")
		}
		if write != 0 && sameApplicationDependencyEdge(out[write-1], dependency) {
			if out[write-1].SemanticVersion != dependency.SemanticVersion {
				return nil, fmt.Errorf("body: conflicting application dependency at call point %d occurrence %d", dependency.CallPoint, dependency.CallOccurrence)
			}
			continue
		}
		out[write] = dependency
		write++
	}
	return out[:write], nil
}

func sameApplicationDependencyEdge(left, right ApplicationDependency) bool {
	return left.Target == right.Target && left.CallPoint == right.CallPoint && left.CallOccurrence == right.CallOccurrence
}

func (w *bodyDigestWriter) writeApplicationDependencies(dependencies []ApplicationDependency) {
	w.writeInt("application-dependency-count", len(dependencies))
	for _, dependency := range dependencies {
		w.writeInt("application-dependency-call-point", int(dependency.CallPoint))
		w.writeUint64("application-dependency-call-occurrence", uint64(dependency.CallOccurrence))
		w.writeBytes("application-dependency-target", dependency.Target[:])
		w.writeUint64("application-dependency-semantic-version", dependency.SemanticVersion)
	}
}

func (w *bodyDigestWriter) writeWidening(config SolveConfig) {
	points := graphRPO(w.static.cfg.Graph)
	w.writeBool("has-widen-at", config.WidenAt != nil)
	if config.WidenAt != nil {
		for ordinal, point := range points {
			w.writeInt("widen-at-point", ordinal)
			w.writeBool("widen-at", config.WidenAt(point))
		}
	}
	w.writeBool("has-widen-delay", config.WidenDelay != nil)
	if config.WidenDelay != nil {
		for ordinal, point := range points {
			w.writeInt("widen-delay-point", ordinal)
			w.writeInt("widen-delay", config.WidenDelay(point))
		}
	}
}

func staticResultVersionPrefix(s *Static, ctx context.Context) (internalhash.Writer, hash.Hash, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return internalhash.Writer{}, nil, errors.Join(solve.ErrCanceled, err)
		}
	}
	s.immutableDigestMu.Lock()
	if s.resultVersionPrefixReady {
		prefix := s.resultVersionPrefix
		wide := sha256.New()
		if err := wide.(encoding.BinaryUnmarshaler).UnmarshalBinary(s.resultVersionWidePrefix); err != nil {
			s.immutableDigestMu.Unlock()
			return internalhash.Writer{}, nil, err
		}
		s.immutableDigestMu.Unlock()
		return prefix, wide, nil
	}
	s.immutableDigestMu.Unlock()

	// Do not hold the cache lock while encoding: a canceled solve must remain
	// independently cancelable even if another solve is computing the prefix.
	// Concurrent first solves may duplicate this one-time work, then publish the
	// same deterministic writer state below.
	w := newBodyDigestWriter(s, ctx)
	w.wide = sha256.New()
	w.label("body-inputs-v2")
	w.writeWIR(s.wir, s.cfg.Graph)
	w.writeSymbolTypes(s.symbolTypes)
	w.writeBoundaryEnvironment()
	if err := w.err(); err != nil {
		return internalhash.Writer{}, nil, errors.Join(solve.ErrCanceled, err)
	}
	wideState, err := w.wide.(encoding.BinaryMarshaler).MarshalBinary()
	if err != nil {
		return internalhash.Writer{}, nil, err
	}
	s.immutableDigestMu.Lock()
	defer s.immutableDigestMu.Unlock()
	if s.resultVersionPrefixReady {
		wide := sha256.New()
		if err := wide.(encoding.BinaryUnmarshaler).UnmarshalBinary(s.resultVersionWidePrefix); err != nil {
			return internalhash.Writer{}, nil, err
		}
		return s.resultVersionPrefix, wide, nil
	}
	s.resultVersionPrefix = w.h
	s.resultVersionWidePrefix = append([]byte(nil), wideState...)
	s.resultVersionPrefixReady = true
	wide := sha256.New()
	if err := wide.(encoding.BinaryUnmarshaler).UnmarshalBinary(wideState); err != nil {
		return internalhash.Writer{}, nil, err
	}
	return s.resultVersionPrefix, wide, nil
}

// BoundaryEnvironmentDigest is the immutable semantic environment consumed at
// a prepared body boundary. It deliberately excludes the body's WIR and all
// per-solve inputs. The digest mirrors the exact symbol/signature/module/global
// encoding used by ResultVersion, so transformer
// identities cannot drift onto a parallel notion of environment equality.
type BoundaryEnvironmentDigest [sha256.Size]byte

// BoundaryEnvironmentDigest returns the prepared body's canonical boundary
// environment identity. It is stable across distinct bodies prepared against
// the same environment. Callers needing cancellation should use the Context
// form.
func (s *Static) BoundaryEnvironmentDigest() BoundaryEnvironmentDigest {
	digest, _ := s.BoundaryEnvironmentDigestContext(context.Background())
	return digest
}

// BoundaryEnvironmentDigestContext returns BoundaryEnvironmentDigest while
// observing ctx during potentially large type and manifest encodings.
func (s *Static) BoundaryEnvironmentDigestContext(ctx context.Context) (BoundaryEnvironmentDigest, error) {
	if s == nil {
		return BoundaryEnvironmentDigest{}, nil
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return BoundaryEnvironmentDigest{}, errors.Join(solve.ErrCanceled, err)
		}
	}
	s.immutableDigestMu.Lock()
	if s.boundaryEnvironmentDigestReady {
		digest := s.boundaryEnvironmentDigest
		s.immutableDigestMu.Unlock()
		return digest, nil
	}
	s.immutableDigestMu.Unlock()

	w := newBodyDigestWriter(s, ctx)
	w.wide = sha256.New()
	w.label("body-boundary-environment-v1")
	w.writeBoundaryEnvironment()
	if err := w.err(); err != nil {
		return BoundaryEnvironmentDigest{}, errors.Join(solve.ErrCanceled, err)
	}
	var digest BoundaryEnvironmentDigest
	copy(digest[:], w.wide.Sum(nil))

	s.immutableDigestMu.Lock()
	defer s.immutableDigestMu.Unlock()
	if s.boundaryEnvironmentDigestReady {
		return s.boundaryEnvironmentDigest, nil
	}
	s.boundaryEnvironmentDigest = digest
	s.boundaryEnvironmentDigestReady = true
	return digest, nil
}

// writeBoundaryEnvironment is the single semantic encoder shared by
// ResultVersion and BoundaryEnvironmentDigest. Keep additions here rather than
// teaching transformer identity about body configuration independently.
func (w *bodyDigestWriter) writeBoundaryEnvironment() {
	w.writeGlobals(w.static.globals, w.static.globalTypes)
	w.writeManifestSource("signatures", w.static.signatures.Manifests)
	w.writeManifestSource("module-types", w.static.moduleTypes.Manifests)
	w.writeManifestSource("module-loads", w.static.moduleLoads.Manifests)
	w.writeBool("signatures-stdlib", w.static.signatures.IncludeStdlib)
}

type bodyDigestWriter struct {
	h           internalhash.Writer
	wide        hash.Hash
	static      *Static
	symbols     map[symbol.ID]string
	ctx         context.Context
	stateLanes  []state.LaneID
	wideScratch [128]byte
	steps       uint64
	errVal      error
}

func newBodyDigestWriter(s *Static, ctx ...context.Context) *bodyDigestWriter {
	var solveCtx context.Context
	if len(ctx) != 0 {
		solveCtx = ctx[0]
	}
	return &bodyDigestWriter{
		h:       internalhash.NewWriter(),
		static:  s,
		symbols: make(map[symbol.ID]string),
		ctx:     solveCtx,
	}
}

func (w *bodyDigestWriter) sum64() uint64 {
	if w == nil {
		return 0
	}
	return w.h.Sum64()
}

func (w *bodyDigestWriter) label(label string) {
	w.writeString("#", label)
}

func (w *bodyDigestWriter) writeRaw(value string) {
	if !w.checkpoint() {
		return
	}
	_, _ = w.h.WriteString(value)
	if w.wide != nil {
		w.writeWideString(value)
	}
}

func (w *bodyDigestWriter) writeWideString(value string) {
	for len(value) != 0 {
		count := min(len(value), len(w.wideScratch))
		copy(w.wideScratch[:count], value[:count])
		_, _ = w.wide.Write(w.wideScratch[:count])
		value = value[count:]
	}
}

func (w *bodyDigestWriter) writeByte(value byte) {
	if !w.checkpoint() {
		return
	}
	_ = w.h.WriteByte(value)
	if w.wide != nil {
		w.wideScratch[0] = value
		_, _ = w.wide.Write(w.wideScratch[:1])
	}
}

func (w *bodyDigestWriter) checkpoint() bool {
	if w == nil || w.errVal != nil {
		return false
	}
	w.steps++
	if w.steps%64 != 0 || w.ctx == nil {
		return true
	}
	if err := w.ctx.Err(); err != nil {
		w.errVal = err
		return false
	}
	return true
}

func (w *bodyDigestWriter) err() error {
	if w == nil {
		return nil
	}
	if w.errVal != nil {
		return w.errVal
	}
	if w.ctx != nil {
		return w.ctx.Err()
	}
	return nil
}

func (w *bodyDigestWriter) writeRawInt(value int) {
	w.h.WriteIntDecimal(int64(value))
	if w.wide != nil {
		encoded := strconv.AppendInt(w.wideScratch[:0], int64(value), 10)
		_, _ = w.wide.Write(encoded)
	}
}

func (w *bodyDigestWriter) writeRawUint64(value uint64) {
	w.h.WriteUintDecimal(value)
	if w.wide != nil {
		encoded := strconv.AppendUint(w.wideScratch[:0], value, 10)
		_, _ = w.wide.Write(encoded)
	}
}

func (w *bodyDigestWriter) writeString(label, value string) {
	w.writeRaw(label)
	w.writeRaw(":s:")
	w.writeRawInt(len(value))
	w.writeByte(':')
	w.writeRaw(value)
	w.writeByte(';')
}

func (w *bodyDigestWriter) writeBytes(label string, value []byte) {
	w.writeRaw(label)
	w.writeRaw(":x:")
	w.writeRawInt(len(value))
	w.writeByte(':')
	_, _ = w.h.Write(value)
	if w.wide != nil {
		_, _ = w.wide.Write(value)
	}
	w.writeByte(';')
}

func (w *bodyDigestWriter) writeBool(label string, value bool) {
	w.writeRaw(label)
	w.writeRaw(":b:")
	w.h.WriteBool(value)
	if w.wide != nil {
		if value {
			w.writeWideString("true")
		} else {
			w.writeWideString("false")
		}
	}
	w.writeByte(';')
}

// standaloneLexicalUnitNamespace reuses the exact semantic WIR/CFG encoding
// used by ResultVersion, mirrored into SHA-256. The traversal descends into
// every lexical child body, so a standalone body's namespace covers its whole
// semantic subtree. The encoding deliberately excludes process-local
// expression identities and presentation-only spans.
func standaloneLexicalUnitNamespace(bindings *bind.Result, built *cfgbuild.Result, body *wir.Body) lexicalidentity.UnitNamespace {
	if built == nil || built.Graph == nil || body == nil {
		return lexicalidentity.UnitNamespace{}
	}
	partial := &Static{bindings: bindings, cfg: built, wir: body}
	w := newBodyDigestWriter(partial)
	w.wide = sha256.New()
	w.writeString("#", "standalone-lexical-wir-v1")
	w.writeWIRTree(body, built.Graph)
	if w.err() != nil {
		return lexicalidentity.UnitNamespace{}
	}
	var digest [sha256.Size]byte
	copy(digest[:], w.wide.Sum(nil))
	return lexicalidentity.UnitNamespaceFromDigest(digest)
}

// writeWIRTree is the recursive form of the canonical per-body encoder. It is
// used only to derive a standalone compilation-unit namespace; ResultVersion
// deliberately remains scoped to one prepared body and calls writeWIR directly.
func (w *bodyDigestWriter) writeWIRTree(body *wir.Body, graph cfg.Graph) {
	w.writeWIR(body, graph)
	if body == nil {
		return
	}
	protos := body.Protos()
	w.writeInt("child-body-count", len(protos))
	for index, proto := range protos {
		w.writeInt("child-body-index", index)
		w.writeWIRTree(proto.Body, proto.Graph)
	}
}

func (w *bodyDigestWriter) writeInt(label string, value int) {
	w.writeRaw(label)
	w.writeRaw(":i:")
	w.writeRawInt(value)
	w.writeByte(';')
}

func (w *bodyDigestWriter) writeUint64(label string, value uint64) {
	w.writeRaw(label)
	w.writeRaw(":u:")
	w.writeRawUint64(value)
	w.writeByte(';')
}

func (w *bodyDigestWriter) writeType(label string, t typ.Type) {
	if t == nil {
		w.writeString(label, "<nil>")
		return
	}
	if !w.checkpoint() {
		return
	}
	h, err := typ.EqualityHashContext(w.ctx, t)
	if err != nil {
		w.errVal = err
		return
	}
	w.writeUint64(label+":hash", h)
	w.writeString(label+":display", t.String())
}

func (w *bodyDigestWriter) writeProduct(label string, value product.Value) {
	w.writeUint64(label, w.stableProductHash(value))
}

func (w *bodyDigestWriter) stableProductHash(value product.Value) uint64 {
	h := internalhash.NewWriter()
	_, _ = h.WriteString("product:")
	// product.Hash is the canonical product encoding used by product.Equal. It
	// covers every semantic axis, including identity, escape, evidence, and
	// refinements, so ResultVersion cannot project away consumed facts.
	h.WriteUintDecimal(product.Hash(w.static.registry, value))
	_ = h.WriteByte(':')
	h.WriteIntDecimal(int64(product.ShapeOf(value)))
	_ = h.WriteByte(';')
	return h.Sum64()
}

func (w *bodyDigestWriter) writeWIR(body *wir.Body, graph cfg.Graph) {
	w.label("wir")
	if body == nil {
		w.writeString("name", "<nil>")
		return
	}
	w.writeString("name", body.Name)
	w.writeGraph(graph)
	w.writeDeclaredReturns(body)
	w.writeRootTypes(body)
	points := graphRPO(graph)
	w.writeInt("point-count", len(points))
	for ordinal, point := range points {
		w.writeInt("point", ordinal)
		node := graph.Node(point)
		if node != nil {
			w.writeInt("node-kind", int(node.Kind))
		}
		instructions := body.PointInstructions(point)
		w.writeInt("instr-count", len(instructions))
		for _, inst := range instructions {
			w.writeInstruction(body, inst)
		}
	}
	w.writeProtos(body)
}

func (w *bodyDigestWriter) writeGraph(graph cfg.Graph) {
	w.label("graph")
	if graph == nil {
		w.writeString("graph", "<nil>")
		return
	}
	points := graphRPO(graph)
	ord := make(map[cfg.Point]int, len(points))
	for i, point := range points {
		ord[point] = i
	}
	w.writeInt("size", graph.Size())
	w.writeInt("entry", ord[graph.Entry()])
	w.writeInt("exit", ord[graph.Exit()])
	for _, point := range points {
		w.writeInt("graph-point", ord[point])
		type successorEdge struct {
			to      cfg.Point
			cond    bool
			hasCond bool
		}
		successors := cfg.SuccessorsReadOnly(graph, point)
		conditions := cfg.SuccessorConditionsReadOnly(graph, point)
		edges := make([]successorEdge, len(successors))
		for index, successor := range successors {
			edges[index].to = successor
			if len(conditions) == len(successors) {
				edges[index].cond, edges[index].hasCond = conditions[index], true
			}
		}
		sort.Slice(edges, func(i, j int) bool {
			if edges[i].hasCond != edges[j].hasCond {
				return !edges[i].hasCond && edges[j].hasCond
			}
			if edges[i].cond != edges[j].cond {
				return !edges[i].cond && edges[j].cond
			}
			return ord[edges[i].to] < ord[edges[j].to]
		})
		w.writeInt("succ-count", len(edges))
		for _, edge := range edges {
			w.writeInt("succ", ord[edge.to])
			w.writeBool("succ-has-cond", edge.hasCond)
			w.writeBool("succ-cond", edge.cond)
		}
	}
}

func graphRPO(graph cfg.Graph) []cfg.Point {
	if graph == nil {
		return nil
	}
	return graph.RPO()
}

func (w *bodyDigestWriter) writeInstruction(body *wir.Body, inst wir.Instruction) {
	w.writeInstructionDescriptor(inst)
	w.writeOperand(body, "dst", inst.Dst)
	w.writeOperand(body, "a", inst.A)
	w.writeOperand(body, "b", inst.B)
	w.writeOperands(body, "list", inst.List)
	w.writeOperands(body, "results", inst.Results)
	w.writeTableEntries(body, inst.TableEntries)
	w.writeSegments("dynamic-suffix", body.Segments(inst.DynamicSuffix))
	w.writeCheck("check", body.Check(inst.Check))
	for _, check := range body.ImpliedChecks(inst.ImpliedChecks) {
		w.writeBool("implied-edge", check.Edge)
		w.writeBool("implied-polarity", check.Polarity)
		w.writeCheck("implied", check.Check)
	}
	for _, check := range body.SufficientChecks(inst.SufficientChecks) {
		w.writeBool("sufficient-edge", check.Edge)
		w.writeBool("sufficient-polarity", check.Polarity)
		w.writeCheck("sufficient", check.Check)
	}
	for _, diff := range body.BranchDiffConstraints(inst.DiffConstraints) {
		w.writeBranchDiff(diff)
	}
	for _, ref := range body.TypeRefs(inst.CallTypeArgs) {
		w.writeType("call-type-arg", body.Type(ref))
	}
	if inst.Func != 0 {
		w.writeProtoRef(body.Proto(inst.Func))
	}
	if inst.Call.Method != 0 {
		w.writeConst("method", body.Const(inst.Call.Method))
	}
	w.writeOperand(body, "call-callee", inst.Call.Callee)
	w.writeOperand(body, "call-receiver", inst.Call.Receiver)
}

func (w *bodyDigestWriter) writeInstructionDescriptor(inst wir.Instruction) {
	w.label("instruction")
	w.writeInt("op", int(inst.Op))
	w.writeOperandRange("list-range", inst.List)
	w.writeTableEntryRange("table-entry-range", inst.TableEntries)
	w.writeBool("static-string-keys-complete", inst.StaticStringKeysComplete)
	w.writeSegmentRange("dynamic-suffix-range", inst.DynamicSuffix)
	w.writeImpliedCheckRange("implied-check-range", inst.ImpliedChecks)
	w.writeImpliedCheckRange("sufficient-check-range", inst.SufficientChecks)
	w.writeBranchDiffRange("diff-constraint-range", inst.DiffConstraints)
	w.writeOperandRange("result-range", inst.Results)
	w.writeInt("operator", int(inst.Operator))
	w.writeInt("iter", int(inst.Iter))
	w.writeInt("claim", int(inst.Claim))
	w.writeInt("assign", int(inst.Assign))
	w.writeUint64("type-ref", uint64(inst.Type))
	w.writeUint64("check-ref", uint64(inst.Check))
	w.writeUint64("func-ref", uint64(inst.Func))
	w.writeInt("call-context", int(inst.CallContext))
	w.writeBool("call-final", inst.CallFinal)
	w.writeBool("call-expanded", inst.CallExpanded)
	w.writeBool("call-adjusted", inst.CallAdjusted)
	w.writeBool("call-open-tail", inst.CallOpenTail)
	w.writeBool("call-condition-negated", inst.CallConditionNegated)
	w.writeCallArgumentMetaRange("call-argument-range", inst.CallArgs)
	w.writeTypeRefRange("call-type-argument-range", inst.CallTypeArgs)
	w.writeReturnValueMetaRange("return-value-range", inst.ReturnValues)
	w.writeBool("select-default", inst.SelectDefault)
	w.writeBool("list-spread", inst.ListSpread)
	w.writeBool("result-spread", inst.ResultSpread)
}

func (w *bodyDigestWriter) writeOperandRange(label string, value wir.OperandRange) {
	w.writeUint64(label+":start", uint64(value.Start))
	w.writeUint64(label+":len", uint64(value.Len))
}

func (w *bodyDigestWriter) writeTableEntryRange(label string, value wir.TableEntryRange) {
	w.writeUint64(label+":start", uint64(value.Start))
	w.writeUint64(label+":len", uint64(value.Len))
}

func (w *bodyDigestWriter) writeSegmentRange(label string, value wir.SegmentRange) {
	w.writeUint64(label+":start", uint64(value.Start))
	w.writeUint64(label+":len", uint64(value.Len))
}

func (w *bodyDigestWriter) writeImpliedCheckRange(label string, value wir.ImpliedCheckRange) {
	w.writeUint64(label+":start", uint64(value.Start))
	w.writeUint64(label+":len", uint64(value.Len))
}

func (w *bodyDigestWriter) writeBranchDiffRange(label string, value wir.BranchDiffConstraintRange) {
	w.writeUint64(label+":start", uint64(value.Start))
	w.writeUint64(label+":len", uint64(value.Len))
}

func (w *bodyDigestWriter) writeCallArgumentMetaRange(label string, value wir.CallArgumentMetaRange) {
	w.writeUint64(label+":start", uint64(value.Start))
	w.writeUint64(label+":len", uint64(value.Len))
}

func (w *bodyDigestWriter) writeTypeRefRange(label string, value wir.TypeRefRange) {
	w.writeUint64(label+":start", uint64(value.Start))
	w.writeUint64(label+":len", uint64(value.Len))
}

func (w *bodyDigestWriter) writeReturnValueMetaRange(label string, value wir.ReturnValueMetaRange) {
	w.writeUint64(label+":start", uint64(value.Start))
	w.writeUint64(label+":len", uint64(value.Len))
}

func (w *bodyDigestWriter) writeOperand(body *wir.Body, label string, op wir.Operand) {
	w.writeInt(label+":kind", int(op.Kind))
	w.writeUint64(label+":ref", uint64(op.Ref))
	switch op.Kind {
	case wir.OperandPath:
		w.writePath(label+":path", body.Path(wir.PathRef(op.Ref)))
	case wir.OperandConst:
		w.writeConst(label+":const", body.Const(wir.ConstRef(op.Ref)))
	case wir.OperandType:
		w.writeType(label+":type", body.Type(wir.TypeRef(op.Ref)))
	}
}

func (w *bodyDigestWriter) writeOperands(body *wir.Body, label string, r wir.OperandRange) {
	ops := body.Operands(r)
	w.writeInt(label+":len", len(ops))
	for _, op := range ops {
		w.writeOperand(body, label, op)
	}
}

func (w *bodyDigestWriter) writeConst(label string, c wir.Const) {
	w.writeInt(label+":kind", int(c.Kind))
	w.writeBool(label+":bool", c.Bool)
	w.writeString(label+":number", c.Number)
	w.writeString(label+":string", c.Str)
}

func (w *bodyDigestWriter) writeTableEntries(body *wir.Body, r wir.TableEntryRange) {
	entries := body.TableEntries(r)
	w.writeInt("table-entry-count", len(entries))
	for _, entry := range entries {
		w.writePath("table-entry-suffix", entry.Suffix)
		w.writeOperand(body, "table-entry-value", entry.Value)
		w.writeString("table-entry-label", entry.ValueLabel)
	}
}

func (w *bodyDigestWriter) writeCheck(label string, check wir.Check) {
	w.writeInt(label+":kind", int(check.Kind))
	w.writePath(label+":path", check.Path)
	w.writePath(label+":other", check.OtherPath)
	w.writeString(label+":type-name", check.TypeName)
	w.writeType(label+":literal", check.Literal)
	w.writeString(label+":literal-string", check.LiteralString)
	w.writeUint64(label+":len-floor", uint64(check.LenFloor))
	w.writeUint64(label+":num-floor", uint64(check.NumFloor))
	w.writeUint64(label+":num-ceil", uint64(check.NumCeil))
	w.writeBool(label+":has-num-ceil", check.HasNumCeil)
	w.writeBool(label+":num-ceil-negated", check.NumCeilNegated)
	w.writeBool(label+":negated", check.Negated)
}

func (w *bodyDigestWriter) writeBranchDiff(diff wir.BranchDiffConstraint) {
	w.label("branch-diff")
	w.writeUint64("co-hi", uint64(diff.CoHi))
	w.writePath("hi", diff.HiPath)
	w.writeBool("hi-len", diff.HiIsLen)
	w.writeUint64("co-hi2", uint64(diff.CoHi2))
	w.writePath("hi2", diff.Hi2Path)
	w.writeBool("hi2-len", diff.Hi2IsLen)
	w.writeBool("has-hi2", diff.HasHi2)
	w.writePath("lo", diff.LoPath)
	w.writeBool("lo-len", diff.LoIsLen)
	w.writeUint64("c", uint64(diff.C))
	w.writeBool("edge", diff.Edge)
}

func (w *bodyDigestWriter) writeDeclaredReturns(body *wir.Body) {
	returns := body.DeclaredReturnTypes()
	w.writeInt("decl-return-count", len(returns))
	for _, t := range returns {
		w.writeType("decl-return", t)
	}
}

func (w *bodyDigestWriter) writeRootTypes(body *wir.Body) {
	roots := body.RootTypes()
	sort.Slice(roots, func(i, j int) bool {
		return w.pathString(roots[i].Path) < w.pathString(roots[j].Path)
	})
	w.writeInt("root-type-count", len(roots))
	for _, root := range roots {
		w.writePath("root-type-path", root.Path)
		w.writeType("root-type", body.Type(root.Type))
	}
}

func (w *bodyDigestWriter) writeProtos(body *wir.Body) {
	protos := body.Protos()
	w.writeInt("proto-count", len(protos))
	for _, proto := range protos {
		w.writeProtoRef(proto)
	}
}

func (w *bodyDigestWriter) writeProtoRef(proto wir.FuncProto) {
	w.writeString("proto-name", proto.Name)
	w.writeType("proto-type", proto.Type)
	// The child body owns its own ResultVersion. Parent input identity records
	// the closure value and captured operands, but deliberately does not recurse
	// into the nested body instruction stream.
}

func (w *bodyDigestWriter) writeSymbolTypes(types map[symbol.ID]typ.Type) {
	if len(types) == 0 {
		w.writeInt("symbol-type-count", 0)
		return
	}
	type entry struct {
		key string
		typ typ.Type
	}
	entries := make([]entry, 0, len(types))
	for id, t := range types {
		entries = append(entries, entry{key: w.symbolString(id), typ: t})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].key < entries[j].key })
	w.writeInt("symbol-type-count", len(entries))
	for _, entry := range entries {
		w.writeString("symbol-type-key", entry.key)
		w.writeType("symbol-type", entry.typ)
	}
}

func (w *bodyDigestWriter) writeGlobals(globals []string, globalTypes map[string]typ.Type) {
	copiedGlobals := append([]string(nil), globals...)
	sort.Strings(copiedGlobals)
	w.writeInt("global-count", len(copiedGlobals))
	for _, name := range copiedGlobals {
		w.writeString("global", name)
	}
	names := make([]string, 0, len(globalTypes))
	for name := range globalTypes {
		names = append(names, name)
	}
	sort.Strings(names)
	w.writeInt("global-type-count", len(names))
	for _, name := range names {
		w.writeString("global-type-name", name)
		w.writeType("global-type", globalTypes[name])
	}
}

func (w *bodyDigestWriter) writeManifestSource(label string, manifests []*manifest.Manifest) {
	w.writeInt(label+":manifest-count", len(manifests))
	for i, m := range manifests {
		w.writeInt(label+":manifest-index", i)
		w.writeManifest(label, m)
	}
}

func (w *bodyDigestWriter) writeManifest(label string, m *manifest.Manifest) {
	if m == nil {
		w.writeString(label+":manifest", "<nil>")
		return
	}
	w.writeString(label+":path", m.Path)
	w.writeString(label+":version", m.Version)
	w.writeTypeIdentity(label+":export", m.Export)
	w.writeTypeIdentity(label+":error-type", m.ErrorType)
	w.writeStringSlice(label+":globals", m.Globals)
	w.writeTypeMap(label+":types", m.Types)
	w.writeTypeMap(label+":global-types", m.GlobalTypes)
	signatureNames := make([]string, 0, len(m.FunctionSignatures))
	for name := range m.FunctionSignatures {
		signatureNames = append(signatureNames, name)
	}
	sort.Strings(signatureNames)
	w.writeInt(label+":signature-count", len(signatureNames))
	for _, name := range signatureNames {
		sig := m.FunctionSignatures[name]
		w.writeString(label+":signature-name", name)
		w.writeTypeIdentity(label+":signature-type", sig.Type)
		w.writeString(label+":signature-effect", sig.Effect.String())
		w.writeCanonicalOperationalEffects(label+":signature-operational-effects", sig.OperationalEffects)
	}
	protocols := make([]typestateProtocolEntry, 0, len(m.TypestateProtocols))
	for protocol, definition := range m.TypestateProtocols {
		protocols = append(protocols, typestateProtocolEntry{name: string(protocol), definition: definition})
	}
	sort.Slice(protocols, func(i, j int) bool { return protocols[i].name < protocols[j].name })
	w.writeInt(label+":typestate-count", len(protocols))
	for _, protocol := range protocols {
		w.writeString(label+":typestate-protocol", protocol.name)
		w.writeCanonicalTypestateDefinition(label+":typestate-definition", protocol.definition)
	}
	registrations := append([]manifest.CallbackPhaseRegistration(nil), m.CallbackPhaseRegistrations...)
	sort.Slice(registrations, func(i, j int) bool {
		if registrations[i].Function != registrations[j].Function {
			return registrations[i].Function < registrations[j].Function
		}
		if registrations[i].CallbackParam != registrations[j].CallbackParam {
			return registrations[i].CallbackParam < registrations[j].CallbackParam
		}
		return registrations[i].Phase < registrations[j].Phase
	})
	w.writeInt(label+":callback-registration-count", len(registrations))
	for _, registration := range registrations {
		w.writeString(label+":callback-registration-function", registration.Function)
		w.writeInt(label+":callback-registration-param", registration.CallbackParam)
		w.writeString(label+":callback-registration-phase", registration.Phase)
	}
	invocations := append([]manifest.CallbackPhaseInvocation(nil), m.CallbackPhaseInvocations...)
	sort.Slice(invocations, func(i, j int) bool {
		if invocations[i].Function != invocations[j].Function {
			return invocations[i].Function < invocations[j].Function
		}
		return invocations[i].CallbackParam < invocations[j].CallbackParam
	})
	w.writeInt(label+":callback-invocation-count", len(invocations))
	for _, invocation := range invocations {
		w.writeString(label+":callback-invocation-function", invocation.Function)
		w.writeInt(label+":callback-invocation-param", invocation.CallbackParam)
		w.writeStringSlice(label+":callback-invocation-before", invocation.Before)
		w.writeStringSlice(label+":callback-invocation-after", invocation.After)
	}
}

type typestateProtocolEntry struct {
	name       string
	definition typestate.Definition
}

func (w *bodyDigestWriter) writeCanonicalOperationalEffects(label string, effects *signature.OperationalEffects) {
	if err := w.err(); err != nil {
		return
	}
	digest, err := manifest.CanonicalOperationalEffectsDigest(w.ctx, effects)
	if err != nil {
		if w.ctx != nil && w.ctx.Err() != nil {
			w.errVal = w.ctx.Err()
			return
		}
		w.writeString(label, "<invalid-operational-effects>")
		return
	}
	w.writeUint64(label, digest)
}

func (w *bodyDigestWriter) writeCanonicalTypestateDefinition(label string, definition typestate.Definition) {
	encoded, err := manifest.CanonicalTypestateDefinitionBytes(definition)
	if err != nil {
		w.writeString(label, "<invalid-typestate-definition>")
		return
	}
	w.writeBytes(label, encoded)
}

func (w *bodyDigestWriter) writeTypeIdentity(label string, t typ.Type) {
	if t == nil {
		w.writeString(label, "<nil>")
		return
	}
	if !w.checkpoint() {
		return
	}
	h, err := typ.EqualityHashContext(w.ctx, t)
	if err != nil {
		w.errVal = err
		return
	}
	w.writeUint64(label, h)
}

func (w *bodyDigestWriter) writeTypeMap(label string, values map[string]typ.Type) {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	w.writeInt(label+":count", len(names))
	for _, name := range names {
		w.writeString(label+":name", name)
		w.writeTypeIdentity(label+":type", values[name])
	}
}

func (w *bodyDigestWriter) writeStringSlice(label string, values []string) {
	copied := append([]string(nil), values...)
	sort.Strings(copied)
	w.writeInt(label+":count", len(copied))
	for _, value := range copied {
		w.writeString(label, value)
	}
}

func (w *bodyDigestWriter) writeClosedDynamicInvariants(in []factapply.ClosedDynamicAllValueInvariant) {
	entries := make([]string, 0, len(in))
	for _, invariant := range in {
		entries = append(entries, w.pathString(invariant.Container)+"=>"+w.pathString(invariant.Table)+"@"+string(invariant.Site))
	}
	sort.Strings(entries)
	w.writeInt("closed-dynamic-count", len(entries))
	for _, entry := range entries {
		w.writeString("closed-dynamic", entry)
	}
}

func (w *bodyDigestWriter) writeState(label string, st state.State) {
	w.label(label + ":state")
	digest, err := state.SemanticFingerprint(state.FingerprintConfig{
		Context:  w.ctx,
		Registry: w.static.registry,
		KeySpace: w.static.KeySpace(),
		Lanes:    w.stateLanes,
	}, st)
	if err != nil {
		w.errVal = err
		return
	}
	w.writeUint64("semantic", digest)
}

func (w *bodyDigestWriter) writeInitialStates(initial transfer.InitialState, graph cfg.Graph) {
	if initial == nil || graph == nil {
		w.writeInt("initial-count", 0)
		return
	}
	points := graphRPO(graph)
	w.writeInt("initial-point-scan", len(points))
	count := 0
	for i, point := range points {
		st, ok := initial(point)
		if !ok {
			continue
		}
		count++
		w.writeInt("initial-point", i)
		w.writeState("initial", st)
	}
	w.writeInt("initial-count", count)
}

func (w *bodyDigestWriter) writeSummaryInputDigests(provider func() []uint64) {
	if provider == nil {
		w.writeInt("summary-input-count", 0)
		return
	}
	digests := append([]uint64(nil), provider()...)
	sort.Slice(digests, func(i, j int) bool { return digests[i] < digests[j] })
	w.writeInt("summary-input-count", len(digests))
	for _, digest := range digests {
		w.writeUint64("summary-input", digest)
	}
}

func (w *bodyDigestWriter) writeSummaryInputs(inputs []SummaryInput) {
	w.writeSummaryInputDigests(func() []uint64 { return legacySummaryInputDigests(inputs) })
	// Typed key records are future full-width lineage material only. The legacy
	// FNV ResultVersion stream above must remain byte-for-byte compatible.
	w.writeWideInt("summary-input-record-count", len(inputs))
	for _, input := range inputs {
		w.writeWideUint64("summary-input-ref-kind", uint64(input.Key.RefKind))
		w.writeWideUint64("summary-input-ref-id", input.Key.RefID)
		w.writeWideUint64("summary-input-entry-values", input.Key.Values)
		w.writeWideUint64("summary-input-entry-facts", input.Key.Facts)
		w.writeWideUint64("summary-input-entry-references", input.Key.References)
		w.writeWideBool("summary-input-present", input.Present)
		if input.Present {
			w.writeWideUint64("summary-input-payload", input.PayloadDigest)
		}
	}
}

func legacySummaryInputDigests(inputs []SummaryInput) []uint64 {
	digests := make([]uint64, 0, len(inputs))
	for _, input := range inputs {
		h := fnv.New64a()
		if input.Present {
			_, _ = h.Write([]byte("present:"))
			fmt.Fprintf(h, "%d;", input.PayloadDigest)
		} else {
			_, _ = h.Write([]byte("missing"))
		}
		digests = append(digests, h.Sum64())
	}
	return digests
}

func (w *bodyDigestWriter) writeWideRaw(value string) {
	if w.wide == nil || !w.checkpoint() {
		return
	}
	w.writeWideString(value)
}

func (w *bodyDigestWriter) writeWideInt(label string, value int) {
	w.writeWideRaw(label)
	w.writeWideRaw(":i:")
	encoded := strconv.AppendInt(w.wideScratch[:0], int64(value), 10)
	_, _ = w.wide.Write(encoded)
	w.writeWideRaw(";")
}

func (w *bodyDigestWriter) writeWideUint64(label string, value uint64) {
	w.writeWideRaw(label)
	w.writeWideRaw(":u:")
	encoded := strconv.AppendUint(w.wideScratch[:0], value, 10)
	_, _ = w.wide.Write(encoded)
	w.writeWideRaw(";")
}

func (w *bodyDigestWriter) writeWideBool(label string, value bool) {
	w.writeWideRaw(label)
	w.writeWideRaw(":b:")
	if value {
		w.writeWideRaw("true")
	} else {
		w.writeWideRaw("false")
	}
	w.writeWideRaw(";")
}

func (w *bodyDigestWriter) valueSlotString(slot statekey.Value) string {
	if index, ok := statekey.ParseReturnSlot(slot); ok {
		return fmt.Sprintf("ret:%d", index)
	}
	if sym, ok := statekey.ParseSymbolValue(slot); ok {
		return "sym:" + w.symbolString(sym)
	}
	return fmt.Sprintf("slot:%d", uint64(slot))
}

func (w *bodyDigestWriter) keyspaceKeyString(key keyspace.Key) string {
	ks := w.static.KeySpace()
	if p, ok := ks.StatePath(key); ok {
		return w.pathString(p)
	}
	if key.Kind == keyspace.KindStableSym || key.Kind == keyspace.KindResolverSym || key.Kind == keyspace.KindUnversionedSym {
		segments, _ := ks.SegmentsView(key)
		return fmt.Sprintf("ks:%d:%s@%d%s:canon=%t", key.Kind, w.symbolString(key.Sym), key.Ver, segment.FormatSegments(segments), key.Canon)
	}
	if segments, ok := ks.SuffixSegmentsView(key); ok {
		return "suffix:" + segment.FormatSegments(segments)
	}
	segments, _ := ks.SegmentsView(key)
	return fmt.Sprintf("ks:%d:%d:%d%s:canon=%t", key.Kind, key.Root, key.Ver, segment.FormatSegments(segments), key.Canon)
}

func (w *bodyDigestWriter) writePath(label string, p pathdom.Path) {
	w.writeString(label, w.pathString(p))
}

func (w *bodyDigestWriter) pathString(p pathdom.Path) string {
	var b strings.Builder
	if p.Symbol != 0 {
		b.WriteString("sym:")
		b.WriteString(w.symbolString(p.Symbol))
	} else {
		b.WriteString("root:")
		b.WriteString(p.Root)
	}
	if p.Version != 0 {
		fmt.Fprintf(&b, "@%d", p.Version)
	}
	segment.WriteFormattedSegments(&b, p.Segments)
	return b.String()
}

func (w *bodyDigestWriter) writeSegments(label string, segments []segment.Segment) {
	w.writeString(label, segment.FormatSegments(segments))
}

func (w *bodyDigestWriter) symbolString(id symbol.ID) string {
	if id == 0 {
		return "<none>"
	}
	if value, ok := w.symbols[id]; ok {
		return value
	}
	info, ok := w.static.wir.SymbolInfo(pathdom.SymbolID(id))
	if !ok {
		name := ""
		if w.static.bindings != nil {
			name = w.static.bindings.Name(id)
		}
		value := "unknown:" + name
		w.symbols[id] = value
		return value
	}
	name := w.static.wir.SymbolName(pathdom.SymbolID(id))
	requireModule, _ := w.static.wir.SymbolRequireModulePath(pathdom.SymbolID(id))
	value := fmt.Sprintf("kind:%d:name:%s:req:%s:write:%t:implicit:%t",
		info.Kind, name, requireModule, info.HasWrite, info.ImplicitGlobal)
	w.symbols[id] = value
	return value
}
