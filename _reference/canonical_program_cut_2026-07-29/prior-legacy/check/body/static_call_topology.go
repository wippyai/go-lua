package body

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"

	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func (f *StaticForest) CallTopology() operationplan.CallTopology {
	if f == nil {
		return operationplan.CallTopology{}
	}
	return f.callTopology
}

type staticCallLocation struct {
	root   string
	suffix string
}

type staticCallCopy struct {
	from staticCallLocation
	to   staticCallLocation
}

type staticCallSiteBuilder struct {
	owner        int
	point        cfg.Point
	source       staticCallLocation
	publish      bool
	exactTarget  int
	refine       bool
	candidates   map[int]struct{}
	args         []staticCallLocation
	results      []staticCallLocation
	argSpread    bool
	resultSpread bool
}

type staticCallOpenSource struct {
	index  int
	source staticCallLocation
}

type staticCallPair struct {
	location staticCallLocation
	function int
}

type staticCallTopologyBuilder struct {
	statics       []*Static
	indexByBody   map[lexicalidentity.StableLexicalBodyID]int
	indexByProto  map[wir.FunctionSymbolID]int
	locations     map[staticCallLocation][]segment.Segment
	suffixes      map[string][]segment.Segment
	copyByRoot    map[string][]staticCallCopy
	copySeen      map[staticCallCopy]struct{}
	values        map[staticCallLocation]map[int]struct{}
	watchers      map[staticCallLocation][]int
	sites         []staticCallSiteBuilder
	returnSources [][][]staticCallLocation
	returnOpen    [][]staticCallOpenSource
	adjacency     []map[int]struct{}
	queue         []staticCallPair
	queueHead     int
}

func (f *StaticForest) sealCallTopology(bindings *bind.Result) error {
	if f == nil || bindings == nil {
		return fmt.Errorf("prepare lexical forest: call topology has no forest or bindings")
	}
	statics := make([]*Static, 0, len(f.functions)+1)
	if f.root != nil {
		statics = append(statics, f.root)
	}
	for _, static := range f.functions {
		if static == nil {
			return fmt.Errorf("prepare lexical forest: call topology has a nil body")
		}
		statics = append(statics, static)
	}
	sort.Slice(statics, func(i, j int) bool {
		return bytes.Compare(statics[i].lexicalBodyID[:], statics[j].lexicalBodyID[:]) < 0
	})
	b := &staticCallTopologyBuilder{
		statics: statics, indexByBody: make(map[lexicalidentity.StableLexicalBodyID]int, len(statics)),
		indexByProto: make(map[wir.FunctionSymbolID]int, len(f.functions)), locations: make(map[staticCallLocation][]segment.Segment),
		suffixes: make(map[string][]segment.Segment), copyByRoot: make(map[string][]staticCallCopy), copySeen: make(map[staticCallCopy]struct{}), values: make(map[staticCallLocation]map[int]struct{}),
		watchers: make(map[staticCallLocation][]int), returnSources: make([][][]staticCallLocation, len(statics)), returnOpen: make([][]staticCallOpenSource, len(statics)), adjacency: make([]map[int]struct{}, len(statics)),
	}
	for index, static := range statics {
		if static == nil || static.wir == nil || static.operationPlan == nil || static.lexicalBodyID == (lexicalidentity.StableLexicalBodyID{}) {
			return fmt.Errorf("prepare lexical forest: call topology body is incomplete")
		}
		if _, duplicate := b.indexByBody[static.lexicalBodyID]; duplicate {
			return fmt.Errorf("prepare lexical forest: call topology has duplicate body")
		}
		b.indexByBody[static.lexicalBodyID] = index
		b.adjacency[index] = make(map[int]struct{})
	}
	for fn, static := range f.functions {
		fnSymbol, ok := bindings.FunctionSymbol(fn)
		if !ok || fnSymbol == 0 {
			return fmt.Errorf("prepare lexical forest: call topology has unbound function")
		}
		index, ok := b.indexByBody[static.lexicalBodyID]
		if !ok {
			return fmt.Errorf("prepare lexical forest: call topology function body is absent")
		}
		b.indexByProto[wir.FunctionSymbolID(fnSymbol)] = index
	}
	needsPointsTo, err := b.census()
	if err != nil {
		return err
	}
	if needsPointsTo {
		if err := b.collect(); err != nil {
			return err
		}
		if err := b.solve(); err != nil {
			return err
		}
	}
	topology, err := b.freeze()
	if err != nil || !topology.Complete() {
		if err == nil {
			err = fmt.Errorf("prepare lexical forest: call topology seal is incomplete")
		}
		return err
	}
	f.callTopology = topology
	return nil
}

// census builds exact lexical adjacency without allocating the points-to
// relation. The overwhelmingly common all-exact/external forest therefore
// pays only one indexed CallSurface walk plus Tarjan.
func (b *staticCallTopologyBuilder) census() (bool, error) {
	needsPointsTo := false
	for owner, static := range b.statics {
		surface, exact := static.operationPlan.CallSurface()
		if !exact || !surface.Complete() {
			return false, fmt.Errorf("prepare lexical forest: call topology body has no complete call surface")
		}
		var censusErr error
		static.wir.ForEachCall(func(instruction wir.Instruction) bool {
			site, found := surface.Site(instruction.Point)
			if !found {
				censusErr = fmt.Errorf("prepare lexical forest: call topology point is absent from surface")
				return false
			}
			if targetBody, lexical := site.Target.LexicalBody(); lexical {
				target, present := b.indexByBody[targetBody]
				if !present {
					censusErr = fmt.Errorf("prepare lexical forest: exact call target is absent from forest")
					return false
				}
				b.adjacency[owner][target] = struct{}{}
				return true
			}
			if site.Target.Kind() != operationplan.CallSurfaceTargetRejected {
				return true
			}
			if instruction.Call.Method != 0 {
				// A temporary method receiver has no exact member-read source yet.
				needsPointsTo = needsPointsTo || instruction.Call.Receiver.Kind == wir.OperandPath
				return true
			}
			needsPointsTo = needsPointsTo || instruction.Call.Callee.Kind == wir.OperandPath || instruction.Call.Callee.Kind == wir.OperandTemp
			return true
		})
		if censusErr != nil {
			return false, censusErr
		}
	}
	return needsPointsTo, nil
}

func (b *staticCallTopologyBuilder) collect() error {
	for bodyIndex, static := range b.statics {
		body := static.wir
		for raw := 0; raw < body.Len(); raw++ {
			instruction := body.Instr(raw)
			b.observeOperand(bodyIndex, body, instruction.Dst)
			b.observeOperand(bodyIndex, body, instruction.A)
			b.observeOperand(bodyIndex, body, instruction.B)
			for _, operand := range body.Operands(instruction.List) {
				b.observeOperand(bodyIndex, body, operand)
			}
			for _, operand := range body.Operands(instruction.Results) {
				b.observeOperand(bodyIndex, body, operand)
			}
			if source, ok := instruction.AssignmentSourceOperand(); ok && instruction.Op != wir.OpDynamicIndexWrite {
				b.addCopySpec(b.operandLocation(bodyIndex, body, source), b.operandLocation(bodyIndex, body, instruction.Dst))
			}
			switch instruction.Op {
			case wir.OpClaim:
				b.addCopySpec(b.operandLocation(bodyIndex, body, instruction.A), b.operandLocation(bodyIndex, body, instruction.Dst))
			case wir.OpLogical:
				b.addCopySpec(b.operandLocation(bodyIndex, body, instruction.A), b.operandLocation(bodyIndex, body, instruction.Dst))
				b.addCopySpec(b.operandLocation(bodyIndex, body, instruction.B), b.operandLocation(bodyIndex, body, instruction.Dst))
			case wir.OpSelect:
				for _, source := range body.Operands(instruction.List) {
					b.addCopySpec(b.operandLocation(bodyIndex, body, source), b.operandLocation(bodyIndex, body, instruction.Dst))
				}
			case wir.OpMakeTable:
				root := b.operandLocation(bodyIndex, body, instruction.Dst)
				for _, entry := range body.TableEntries(instruction.TableEntries) {
					target := b.observeSuffix(root, entry.Suffix.Segments)
					b.addCopySpec(b.operandLocation(bodyIndex, body, entry.Value), target)
				}
			case wir.OpClosure:
				proto := body.Proto(instruction.Func)
				target, ok := b.indexByProto[proto.Symbol]
				if !ok {
					return fmt.Errorf("prepare lexical forest: closure references an absent function proto")
				}
				b.addValue(b.operandLocation(bodyIndex, body, instruction.Dst), target)
			case wir.OpReturn:
				returnOperands := body.Operands(instruction.List)
				for index, source := range returnOperands {
					for len(b.returnSources[bodyIndex]) <= index {
						b.returnSources[bodyIndex] = append(b.returnSources[bodyIndex], nil)
					}
					b.returnSources[bodyIndex][index] = append(b.returnSources[bodyIndex][index], b.operandLocation(bodyIndex, body, source))
				}
				if instruction.ListSpread && len(returnOperands) != 0 {
					index := len(returnOperands) - 1
					b.returnOpen[bodyIndex] = append(b.returnOpen[bodyIndex], staticCallOpenSource{index: index, source: b.operandLocation(bodyIndex, body, returnOperands[index])})
				}
			}
		}
		if err := b.collectCalls(bodyIndex, static); err != nil {
			return err
		}
	}
	return nil
}

func (b *staticCallTopologyBuilder) collectCalls(owner int, static *Static) error {
	surface, exact := static.operationPlan.CallSurface()
	if !exact || !surface.Complete() {
		return fmt.Errorf("prepare lexical forest: call topology body has no complete call surface")
	}
	body := static.wir
	var collectErr error
	body.ForEachCall(func(instruction wir.Instruction) bool {
		classified, ok := surface.Site(instruction.Point)
		if !ok {
			collectErr = fmt.Errorf("prepare lexical forest: call topology point is absent from surface")
			return false
		}
		source, publish, ok := b.callSource(owner, body, instruction)
		if !ok {
			return true
		}
		site := staticCallSiteBuilder{owner: owner, point: instruction.Point, source: source, publish: publish, exactTarget: -1, refine: classified.Target.Kind() == operationplan.CallSurfaceTargetRejected, candidates: make(map[int]struct{})}
		if instruction.Call.Method != 0 {
			site.args = append(site.args, b.operandLocation(owner, body, instruction.Call.Receiver))
		}
		for _, operand := range body.Operands(instruction.List) {
			site.args = append(site.args, b.operandLocation(owner, body, operand))
		}
		for _, operand := range body.Operands(instruction.Results) {
			site.results = append(site.results, b.operandLocation(owner, body, operand))
		}
		site.argSpread = instruction.ListSpread
		site.resultSpread = instruction.ResultSpread
		if targetBody, lexical := classified.Target.LexicalBody(); lexical {
			if target, present := b.indexByBody[targetBody]; present {
				site.exactTarget = target
			}
		}
		index := len(b.sites)
		b.sites = append(b.sites, site)
		if site.refine {
			b.watchers[source] = append(b.watchers[source], index)
		}
		return true
	})
	return collectErr
}

func (b *staticCallTopologyBuilder) observeOperand(owner int, body *wir.Body, operand wir.Operand) staticCallLocation {
	location := b.operandLocation(owner, body, operand)
	if location.root != "" {
		if _, present := b.locations[location]; !present {
			b.locations[location] = nil
		}
	}
	return location
}

func (b *staticCallTopologyBuilder) operandLocation(owner int, body *wir.Body, operand wir.Operand) staticCallLocation {
	switch operand.Kind {
	case wir.OperandPath:
		path := body.Path(wir.PathRef(operand.Ref))
		if path.Symbol == 0 || path.Version != 0 {
			return staticCallLocation{}
		}
		return b.observeLocation(staticCallLocation{root: "s:" + strconv.FormatUint(uint64(path.Symbol), 10), suffix: segment.FormatSegments(path.Segments)}, path.Segments)
	case wir.OperandTemp:
		return b.observeLocation(staticCallLocation{root: "t:" + b.statics[owner].lexicalBodyID.String() + ":" + strconv.FormatUint(uint64(operand.Ref), 10)}, nil)
	case wir.OperandVararg:
		function := b.statics[owner].function
		if function == nil {
			return staticCallLocation{}
		}
		vararg, ok := b.statics[owner].bindings.VarargSymbol(function)
		if !ok {
			return staticCallLocation{}
		}
		return b.observeLocation(staticCallLocation{root: "s:" + strconv.FormatUint(uint64(vararg), 10)}, nil)
	default:
		return staticCallLocation{}
	}
}

func (b *staticCallTopologyBuilder) observeLocation(location staticCallLocation, suffix []segment.Segment) staticCallLocation {
	if location.root == "" {
		return staticCallLocation{}
	}
	if _, present := b.locations[location]; !present {
		b.locations[location] = append([]segment.Segment(nil), suffix...)
	}
	if _, present := b.suffixes[location.suffix]; !present {
		b.suffixes[location.suffix] = append([]segment.Segment(nil), suffix...)
	}
	return location
}

func (b *staticCallTopologyBuilder) observeSuffix(root staticCallLocation, suffix []segment.Segment) staticCallLocation {
	if root.root == "" {
		return staticCallLocation{}
	}
	segments := append([]segment.Segment(nil), b.locations[root]...)
	segments = append(segments, suffix...)
	return b.observeLocation(staticCallLocation{root: root.root, suffix: segment.FormatSegments(segments)}, segments)
}

func (b *staticCallTopologyBuilder) addCopySpec(from, to staticCallLocation) {
	if from.root != "" && to.root != "" {
		spec := staticCallCopy{from: from, to: to}
		if _, present := b.copySeen[spec]; present {
			return
		}
		b.copySeen[spec] = struct{}{}
		b.copyByRoot[from.root] = append(b.copyByRoot[from.root], spec)
	}
}

func staticCallSegmentsPrefix(value, prefix []segment.Segment) bool {
	if len(prefix) > len(value) {
		return false
	}
	for i := range prefix {
		if value[i] != prefix[i] {
			return false
		}
	}
	return true
}

func (b *staticCallTopologyBuilder) addValue(location staticCallLocation, function int) {
	if location.root == "" || function < 0 || function >= len(b.statics) {
		return
	}
	set := b.values[location]
	if set == nil {
		set = make(map[int]struct{})
		b.values[location] = set
	}
	if _, present := set[function]; present {
		return
	}
	set[function] = struct{}{}
	b.queue = append(b.queue, staticCallPair{location: location, function: function})
}

func (b *staticCallTopologyBuilder) solve() error {
	for index := range b.sites {
		if b.sites[index].exactTarget >= 0 {
			if err := b.activate(index, b.sites[index].exactTarget); err != nil {
				return err
			}
		}
	}
	for b.queueHead < len(b.queue) {
		pair := b.queue[b.queueHead]
		b.queueHead++
		for _, copySpec := range b.copyByRoot[pair.location.root] {
			if target, ok := b.applyCopyLocation(copySpec, pair.location); ok {
				b.addValue(target, pair.function)
			}
		}
		for _, site := range b.watchers[pair.location] {
			if b.sites[site].exactTarget < 0 {
				if err := b.activate(site, pair.function); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (b *staticCallTopologyBuilder) activate(siteIndex, target int) error {
	site := &b.sites[siteIndex]
	if _, present := site.candidates[target]; present {
		return nil
	}
	site.candidates[target] = struct{}{}
	b.adjacency[site.owner][target] = struct{}{}
	params := b.statics[target].operationPlan.BoundaryParams()
	for index := 0; index < len(site.args) && index < len(params); index++ {
		param := b.observeLocation(staticCallLocation{root: "s:" + strconv.FormatUint(uint64(params[index]), 10)}, nil)
		b.addLateCopy(site.args[index], param)
	}
	if site.argSpread && len(site.args) != 0 {
		tailIndex := len(site.args) - 1
		for index := tailIndex + 1; index < len(params); index++ {
			param := b.observeLocation(staticCallLocation{root: "s:" + strconv.FormatUint(uint64(params[index]), 10)}, nil)
			b.addLateCopy(site.args[tailIndex], param)
		}
	}
	if function := b.statics[target].function; function != nil {
		if vararg, ok := b.statics[target].bindings.VarargSymbol(function); ok {
			varargLocation := b.observeLocation(staticCallLocation{root: "s:" + strconv.FormatUint(uint64(vararg), 10)}, nil)
			for index := len(params); index < len(site.args); index++ {
				b.addLateCopy(site.args[index], varargLocation)
			}
			if site.argSpread && len(site.args) != 0 && len(site.args)-1 < len(params) {
				b.addLateCopy(site.args[len(site.args)-1], varargLocation)
			}
		}
	}
	for index := 0; index < len(site.results) && index < len(b.returnSources[target]); index++ {
		for _, source := range b.returnSources[target][index] {
			b.addLateCopy(source, site.results[index])
		}
	}
	for _, open := range b.returnOpen[target] {
		for index := open.index; index < len(site.results); index++ {
			b.addLateCopy(open.source, site.results[index])
		}
	}
	if site.resultSpread && len(site.results) != 0 {
		tailIndex := len(site.results) - 1
		for returnIndex := tailIndex; returnIndex < len(b.returnSources[target]); returnIndex++ {
			for _, source := range b.returnSources[target][returnIndex] {
				b.addLateCopy(source, site.results[tailIndex])
			}
		}
		for _, open := range b.returnOpen[target] {
			if open.index >= tailIndex {
				b.addLateCopy(open.source, site.results[tailIndex])
			}
		}
	}
	return nil
}

func (b *staticCallTopologyBuilder) addLateCopy(from, to staticCallLocation) {
	if from.root == "" || to.root == "" {
		return
	}
	spec := staticCallCopy{from: from, to: to}
	if _, present := b.copySeen[spec]; present {
		return
	}
	b.copySeen[spec] = struct{}{}
	b.copyByRoot[from.root] = append(b.copyByRoot[from.root], spec)
	locations := make([]staticCallLocation, 0)
	for location := range b.values {
		if location.root == from.root {
			locations = append(locations, location)
		}
	}
	sort.Slice(locations, func(i, j int) bool { return staticCallLocationLess(locations[i], locations[j]) })
	for _, location := range locations {
		target, ok := b.applyCopyLocation(spec, location)
		if !ok {
			continue
		}
		functions := make([]int, 0, len(b.values[location]))
		for function := range b.values[location] {
			functions = append(functions, function)
		}
		sort.Ints(functions)
		for _, function := range functions {
			b.addValue(target, function)
		}
	}
}

func (b *staticCallTopologyBuilder) applyCopyLocation(spec staticCallCopy, source staticCallLocation) (staticCallLocation, bool) {
	fromSuffix, sourceSuffix := b.locations[spec.from], b.locations[source]
	if source.root != spec.from.root || !staticCallSegmentsPrefix(sourceSuffix, fromSuffix) {
		return staticCallLocation{}, false
	}
	targetSegments := append([]segment.Segment(nil), b.locations[spec.to]...)
	targetSegments = append(targetSegments, sourceSuffix[len(fromSuffix):]...)
	suffix := segment.FormatSegments(targetSegments)
	canonical, allowed := b.suffixes[suffix]
	if !allowed {
		return staticCallLocation{}, false
	}
	target := staticCallLocation{root: spec.to.root, suffix: suffix}
	return b.observeLocation(target, canonical), true
}

func staticCallLocationLess(left, right staticCallLocation) bool {
	if left.root != right.root {
		return left.root < right.root
	}
	return left.suffix < right.suffix
}

func (b *staticCallTopologyBuilder) callSource(owner int, body *wir.Body, instruction wir.Instruction) (staticCallLocation, bool, bool) {
	if instruction.Call.Method != 0 {
		method := body.Const(instruction.Call.Method)
		if method.Kind != wir.ConstString || method.Str == "" {
			return staticCallLocation{}, false, false
		}
		receiver := b.operandLocation(owner, body, instruction.Call.Receiver)
		location := b.observeSuffix(receiver, []segment.Segment{{Kind: segment.SegmentField, Name: method.Str}})
		switch instruction.Call.Receiver.Kind {
		case wir.OperandPath:
			path := body.Path(wir.PathRef(instruction.Call.Receiver.Ref)).Field(method.Str)
			return location, true, !path.IsEmpty()
		case wir.OperandTemp:
			return location, false, true
		}
		return staticCallLocation{}, false, false
	}
	location := b.operandLocation(owner, body, instruction.Call.Callee)
	switch instruction.Call.Callee.Kind {
	case wir.OperandPath:
		path := body.Path(wir.PathRef(instruction.Call.Callee.Ref))
		return location, true, !path.IsEmpty()
	case wir.OperandTemp:
		return location, true, true
	default:
		return staticCallLocation{}, false, false
	}
}

func (b *staticCallTopologyBuilder) freeze() (operationplan.CallTopology, error) {
	var sites []operationplan.CallTopologySiteInput
	for _, site := range b.sites {
		if site.exactTarget >= 0 || !site.refine || !site.publish || len(site.candidates) == 0 {
			continue
		}
		frozen := operationplan.CallTopologySiteInput{Owner: b.statics[site.owner].lexicalBodyID, Point: site.point}
		for target := range site.candidates {
			fn := b.statics[target].function
			if fn == nil {
				return operationplan.CallTopology{}, fmt.Errorf("prepare lexical forest: finite call target has no function owner")
			}
			functionSymbol, ok := b.statics[target].bindings.FunctionSymbol(fn)
			if !ok {
				return operationplan.CallTopology{}, fmt.Errorf("prepare lexical forest: finite call target has no binder identity")
			}
			frozen.Candidates = append(frozen.Candidates, operationplan.CallTopologyCandidate{Identity: identity.LuaFunction(uint64(functionSymbol)), Target: b.statics[target].lexicalBodyID})
		}
		sort.Slice(frozen.Candidates, func(i, j int) bool { return frozen.Candidates[i].Identity.Index < frozen.Candidates[j].Identity.Index })
		sites = append(sites, frozen)
	}
	components := staticCallSCCs(b.adjacency)
	var componentInputs []operationplan.CallTopologyComponentInput
	var componentID uint32
	for _, members := range components {
		recursive := len(members) > 1
		if len(members) == 1 {
			_, recursive = b.adjacency[members[0]][members[0]]
		}
		if !recursive {
			continue
		}
		componentID++
		for _, member := range members {
			componentInputs = append(componentInputs, operationplan.CallTopologyComponentInput{Body: b.statics[member].lexicalBodyID, Component: componentID})
		}
	}
	bodies := make([]lexicalidentity.StableLexicalBodyID, len(b.statics))
	for index := range b.statics {
		bodies[index] = b.statics[index].lexicalBodyID
	}
	boundaries, err := b.freezeBoundaries()
	if err != nil {
		return operationplan.CallTopology{}, err
	}
	out, err := operationplan.SealCallTopology(bodies, sites, componentInputs, boundaries)
	if err != nil {
		return operationplan.CallTopology{}, err
	}
	return out, nil
}

type staticBoundaryCarrierKind uint8

const (
	staticCaptureCarrier staticBoundaryCarrierKind = iota + 1
	staticGlobalCarrier
)

type staticBoundaryCarrierKey struct {
	kind   staticBoundaryCarrierKind
	symbol symbol.ID
}
type staticBoundaryCarrier struct {
	key      staticBoundaryCarrierKey
	owner    int
	contract product.Value
}
type staticBoundaryPair struct {
	body int
	key  staticBoundaryCarrierKey
}

func (b *staticCallTopologyBuilder) freezeBoundaries() ([]operationplan.CallTopologyBoundaryInput, error) {
	values := make([]map[staticBoundaryCarrierKey]staticBoundaryCarrier, len(b.statics))
	directOrder := make([][]symbol.ID, len(b.statics))
	reverse := make([][]int, len(b.statics))
	for caller, targets := range b.adjacency {
		for target := range targets {
			reverse[target] = append(reverse[target], caller)
		}
	}
	for target := range reverse {
		sort.Ints(reverse[target])
	}
	var queue []staticBoundaryPair
	for index, static := range b.statics {
		plan := static.operationPlan
		if plan == nil || !plan.BoundaryCapturesValid() || !plan.BoundaryGlobalsValid() {
			return nil, fmt.Errorf("prepare lexical forest: call topology boundary census is incomplete")
		}
		values[index] = make(map[staticBoundaryCarrierKey]staticBoundaryCarrier)
		directOrder[index] = plan.BoundaryCaptures()
		for _, captured := range directOrder[index] {
			declaring, inFunction := static.bindings.DeclaringFunction(captured)
			owner := -1
			if inFunction {
				functionSymbol, ok := static.bindings.FunctionSymbol(declaring)
				if !ok {
					return nil, fmt.Errorf("prepare lexical forest: capture has no declaration identity")
				}
				owner, ok = b.indexByProto[wir.FunctionSymbolID(functionSymbol)]
				if !ok {
					return nil, fmt.Errorf("prepare lexical forest: capture declaration body is absent")
				}
			} else {
				for candidate, ownerStatic := range b.statics {
					if ownerStatic.function == nil {
						owner = candidate
						break
					}
				}
			}
			if owner < 0 || owner == index {
				return nil, fmt.Errorf("prepare lexical forest: capture has invalid declaration owner")
			}
			key := staticBoundaryCarrierKey{kind: staticCaptureCarrier, symbol: captured}
			values[index][key] = staticBoundaryCarrier{key: key, owner: owner}
			queue = append(queue, staticBoundaryPair{body: index, key: key})
		}
		globals, contracts := plan.BoundaryGlobals(), plan.BoundaryGlobalContracts()
		if len(globals) != len(contracts) {
			return nil, fmt.Errorf("prepare lexical forest: global contracts are incomplete")
		}
		for globalIndex, global := range globals {
			key := staticBoundaryCarrierKey{kind: staticGlobalCarrier, symbol: global}
			values[index][key] = staticBoundaryCarrier{key: key, owner: -1, contract: contracts[globalIndex]}
			queue = append(queue, staticBoundaryPair{body: index, key: key})
		}
	}
	for head := 0; head < len(queue); head++ {
		pair := queue[head]
		carrier := values[pair.body][pair.key]
		for _, caller := range reverse[pair.body] {
			if carrier.key.kind == staticCaptureCarrier && !b.captureAddressable(caller, carrier.owner) {
				continue
			}
			prior, present := values[caller][carrier.key]
			if present {
				if carrier.key.kind == staticCaptureCarrier && prior.owner != carrier.owner {
					return nil, fmt.Errorf("prepare lexical forest: capture carrier owners conflict")
				}
				if carrier.key.kind == staticGlobalCarrier && !product.Equal(b.statics[caller].registry, prior.contract, carrier.contract) {
					return nil, fmt.Errorf("prepare lexical forest: global carrier contracts conflict")
				}
				continue
			}
			values[caller][carrier.key] = carrier
			queue = append(queue, staticBoundaryPair{body: caller, key: carrier.key})
		}
	}
	out := make([]operationplan.CallTopologyBoundaryInput, len(b.statics))
	for index, static := range b.statics {
		boundary := operationplan.CallTopologyBoundaryInput{Body: static.lexicalBodyID, Captures: append([]symbol.ID(nil), directOrder[index]...)}
		direct := make(map[symbol.ID]struct{}, len(boundary.Captures))
		for _, capture := range boundary.Captures {
			direct[capture] = struct{}{}
		}
		var extras []symbol.ID
		for key := range values[index] {
			if key.kind == staticCaptureCarrier {
				if _, exists := direct[key.symbol]; !exists {
					extras = append(extras, key.symbol)
				}
			} else {
				boundary.Globals = append(boundary.Globals, key.symbol)
			}
		}
		sort.Slice(extras, func(i, j int) bool { return extras[i] < extras[j] })
		boundary.Captures = append(boundary.Captures, extras...)
		sort.Slice(boundary.Globals, func(i, j int) bool { return boundary.Globals[i] < boundary.Globals[j] })
		for _, global := range boundary.Globals {
			boundary.GlobalContracts = append(boundary.GlobalContracts, values[index][staticBoundaryCarrierKey{kind: staticGlobalCarrier, symbol: global}].contract)
		}
		out[index] = boundary
	}
	return out, nil
}

func (b *staticCallTopologyBuilder) captureAddressable(caller, owner int) bool {
	if caller < 0 || caller >= len(b.statics) || owner < 0 || owner >= len(b.statics) || caller == owner {
		return false
	}
	ownerFunction, callerFunction := b.statics[owner].function, b.statics[caller].function
	if ownerFunction == nil {
		return callerFunction != nil
	}
	return callerFunction != nil && b.statics[caller].bindings.FunctionDescendsFrom(callerFunction, ownerFunction)
}

func staticCallSCCs(adjacency []map[int]struct{}) [][]int {
	index, next := make([]int, len(adjacency)), 1
	low, stack, onStack := make([]int, len(adjacency)), make([]int, 0, len(adjacency)), make([]bool, len(adjacency))
	var out [][]int
	var visit func(int)
	visit = func(node int) {
		index[node], low[node], next = next, next, next+1
		stack, onStack[node] = append(stack, node), true
		targets := make([]int, 0, len(adjacency[node]))
		for target := range adjacency[node] {
			targets = append(targets, target)
		}
		sort.Ints(targets)
		for _, target := range targets {
			if index[target] == 0 {
				visit(target)
				if low[target] < low[node] {
					low[node] = low[target]
				}
			} else if onStack[target] && index[target] < low[node] {
				low[node] = index[target]
			}
		}
		if low[node] != index[node] {
			return
		}
		var component []int
		for {
			last := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			onStack[last] = false
			component = append(component, last)
			if last == node {
				break
			}
		}
		sort.Ints(component)
		out = append(out, component)
	}
	for node := range adjacency {
		if index[node] == 0 {
			visit(node)
		}
	}
	return out
}
