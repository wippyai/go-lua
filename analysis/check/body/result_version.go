package body

import (
	"fmt"
	"sort"
	"strings"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	internalhash "github.com/wippyai/go-lua/analysis/internal/hash"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
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

func computeResultVersion(s *Static, config SolveConfig, entry state.State, initial transfer.InitialState) uint64 {
	if s == nil {
		return 0
	}
	w := newBodyDigestWriter(s)
	w.label("body-inputs-v1")
	w.writeWIR(s.wir, s.cfg.Graph)
	w.writeSymbolTypes(s.symbolTypes)
	w.writeGlobals(s.globals, s.globalTypes)
	w.writeManifestSource("signatures", s.signatures.Manifests)
	w.writeManifestSource("module-types", s.moduleTypes.Manifests)
	w.writeManifestSource("module-loads", s.moduleLoads.Manifests)
	w.writeBool("signatures-stdlib", s.signatures.IncludeStdlib)
	w.writeClosedDynamicInvariants(config.ClosedDynamicAllValues)
	w.writeStateLanes(config.StateLanes)
	w.writeState("entry", entry)
	w.writeInitialStates(initial, s.cfg.Graph)
	w.writeSummaryInputDigests(config.SummaryInputDigests)
	return w.sum64()
}

type bodyDigestWriter struct {
	h       internalhash.Writer
	static  *Static
	symbols map[symbol.ID]string
}

func newBodyDigestWriter(s *Static) *bodyDigestWriter {
	return &bodyDigestWriter{
		h:       internalhash.NewWriter(),
		static:  s,
		symbols: make(map[symbol.ID]string),
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
	_, _ = w.h.WriteString(value)
}

func (w *bodyDigestWriter) writeByte(value byte) {
	_ = w.h.WriteByte(value)
}

func (w *bodyDigestWriter) writeRawInt(value int) {
	w.h.WriteIntDecimal(int64(value))
}

func (w *bodyDigestWriter) writeRawUint64(value uint64) {
	w.h.WriteUintDecimal(value)
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
	w.writeByte(';')
}

func (w *bodyDigestWriter) writeBool(label string, value bool) {
	w.writeRaw(label)
	w.writeRaw(":b:")
	w.h.WriteBool(value)
	w.writeByte(';')
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
	w.writeUint64(label+":hash", typ.EqualityHash(t))
	w.writeString(label+":display", t.String())
}

func (w *bodyDigestWriter) writeProduct(label string, value product.Value) {
	w.writeUint64(label, w.stableProductHash(value))
}

func (w *bodyDigestWriter) stableProductHash(value product.Value) uint64 {
	h := internalhash.NewWriter()
	_, _ = h.WriteString("shape:")
	h.WriteIntDecimal(int64(product.ShapeOf(value)))
	_, _ = h.WriteString(";presence:")
	h.WriteIntDecimal(int64(product.PresenceOf(value)))
	_ = h.WriteByte(';')
	reg := w.static.registry
	if reg == nil {
		return h.Sum64()
	}
	if t, ok := typevalue.TypeOf(reg, value); ok {
		_, _ = h.WriteString("type:")
		h.WriteUintDecimal(typ.EqualityHash(t))
		_ = h.WriteByte(':')
		_, _ = h.WriteString(t.String())
		_ = h.WriteByte(';')
		return h.Sum64()
	}
	kind := product.Get(reg, value, runtimekind.Key)
	_, _ = h.WriteString("runtimekind:")
	h.WriteUintDecimal(kind.Hash())
	_ = h.WriteByte(':')
	_, _ = h.WriteString(kind.String())
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
		succs := append([]cfg.Point(nil), cfg.SuccessorsReadOnly(graph, point)...)
		sort.Slice(succs, func(i, j int) bool {
			leftCond, leftHas := graph.EdgeCond(point, succs[i])
			rightCond, rightHas := graph.EdgeCond(point, succs[j])
			if leftHas != rightHas {
				return !leftHas && rightHas
			}
			if leftCond != rightCond {
				return !leftCond && rightCond
			}
			return ord[succs[i]] < ord[succs[j]]
		})
		w.writeInt("succ-count", len(succs))
		for _, succ := range succs {
			cond, hasCond := graph.EdgeCond(point, succ)
			w.writeInt("succ", ord[succ])
			w.writeBool("succ-has-cond", hasCond)
			w.writeBool("succ-cond", cond)
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
	encoded, err := manifest.CanonicalOperationalEffectsDigestBytes(effects)
	if err != nil {
		w.writeString(label, "<invalid-operational-effects>")
		return
	}
	w.writeBytes(label, encoded)
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
	w.writeUint64(label, typ.EqualityHash(t))
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
		entries = append(entries, w.pathString(invariant.Container)+"=>"+w.pathString(invariant.Table))
	}
	sort.Strings(entries)
	w.writeInt("closed-dynamic-count", len(entries))
	for _, entry := range entries {
		w.writeString("closed-dynamic", entry)
	}
}

func (w *bodyDigestWriter) writeStateLanes(lanes []state.LaneID) {
	w.writeInt("lane-count", len(lanes))
	for _, lane := range lanes {
		w.writeString("lane", string(lane))
	}
}

func (w *bodyDigestWriter) writeState(label string, st state.State) {
	w.label(label + ":state")
	values := st.ValuesSnapshot()
	w.writeBool("values-top", values.Top)
	valueEntries := make([]string, 0, len(values.Values))
	for slot, value := range values.Values {
		valueEntries = append(valueEntries, w.valueSlotString(slot)+"="+fmt.Sprint(w.stableProductHash(value)))
	}
	sort.Strings(valueEntries)
	w.writeInt("value-count", len(valueEntries))
	for _, entry := range valueEntries {
		w.writeString("value", entry)
	}
	w.writePathEvidence("path-refinement", st.ForEachPathRefinement)
	w.writePathEvidence("path-static-member", st.ForEachPathStaticMember)
}

func (w *bodyDigestWriter) writePathEvidence(label string, visit func(func(keyspace.Key, product.Value) bool)) {
	var entries []string
	visit(func(key keyspace.Key, value product.Value) bool {
		entries = append(entries, w.keyspaceKeyString(key)+"="+fmt.Sprint(w.stableProductHash(value)))
		return true
	})
	sort.Strings(entries)
	w.writeInt(label+":count", len(entries))
	for _, entry := range entries {
		w.writeString(label, entry)
	}
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
