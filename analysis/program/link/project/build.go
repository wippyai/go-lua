package project

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"math"
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/scalar"
	"github.com/wippyai/go-lua/analysis/program/target"
)

const maxHandle = uint64(^uint32(0))

func fits(count int) bool { return count >= 0 && uint64(count) <= maxHandle }

// Build validates and canonically seals mounts and their exact-key quotient.
// It accepts duplicate Program content identities so that Mounts can represent
// every authored mount; lookup by content then fails closed as ambiguous.
func Build(input Input) (*Draft, error) {
	if input.Target == nil || !input.Target.ContentID().Available() {
		return nil, errors.New("link/project: unavailable target contract")
	}
	if !fits(len(input.Modules)) {
		return nil, errors.New("link/project: shard handle overflow")
	}
	mounts, err := canonicalMounts(input.Modules)
	if err != nil {
		return nil, err
	}
	keys, targetKeys, initialKeys, programKeys, err := buildKeys(mounts, input.Target)
	if err != nil {
		return nil, err
	}
	targetKeyByProject, err := invertTargetKeys(targetKeys, len(keys))
	if err != nil {
		return nil, err
	}
	applications, bases, calls, callsBySource, imports, err := buildApplications(mounts)
	if err != nil {
		return nil, err
	}
	counts, err := buildCountRows(mounts, bases)
	if err != nil {
		return nil, err
	}
	mountContentID := mountRelationID(mounts)
	if !mountContentID.Available() {
		return nil, errors.New("link/project: unavailable mount relation identity")
	}
	applicationContentID := applicationRelationID(applications)
	if !applicationContentID.Available() {
		return nil, errors.New("link/project: unavailable application relation identity")
	}
	projectID := contentID(input.Target.ContentID(), mounts)
	if !projectID.Available() {
		return nil, errors.New("link/project: unavailable content identity")
	}
	authority := &authority{
		target: input.Target, contentID: projectID,
		counts:         counts,
		mountContentID: mountContentID, applicationContentID: applicationContentID,
		mounts: mounts, keys: keys,
		targetKeys: targetKeys, targetKeyByProject: targetKeyByProject, initialKeys: initialKeys, programKeys: programKeys,
		applications: applications, baseApplications: bases, callApplications: calls,
		callApplicationsBySource: callsBySource, importApplications: imports,
	}
	return &Draft{state: &draftState{fence: &draftFence{}, authority: authority}}, nil
}

// invertTargetKeys seals the one owner-local inverse needed by hot consumers
// that already possess a Project key.  Target's exact-key handles remain
// Target-owned; this slice merely records the canonical Target handle at the
// corresponding Project-key ordinal.  A duplicate mapping is ambiguous and
// therefore rejects the Project seal instead of silently choosing a row.
func invertTargetKeys(targetKeys []uint32, projectKeyCount int) ([]vocabulary.ExactKey, error) {
	if projectKeyCount < 0 || uint64(projectKeyCount) > maxHandle {
		return nil, errors.New("link/project: invalid Project key inverse size")
	}
	inverse := make([]vocabulary.ExactKey, projectKeyCount)
	for index, projectOrdinal := range targetKeys {
		if uint64(index) >= maxHandle || uint64(projectOrdinal) >= uint64(projectKeyCount) {
			return nil, errors.New("link/project: malformed Target exact-key inverse")
		}
		if inverse[projectOrdinal] != 0 {
			return nil, errors.New("link/project: ambiguous Target exact-key inverse")
		}
		inverse[projectOrdinal] = vocabulary.ExactKey(index + 1)
	}
	return inverse, nil
}

// Finalize consumes the construction-only source without mutating its sealed
// semantic authority. Enclosing Link identity is a later pure-codec input.
func (d *Draft) Finalize() (*Component, error) {
	if d == nil || d.state == nil || d.state.consumed || d.state.authority == nil {
		return nil, errors.New("link/project: invalid finalization")
	}
	d.state.consumed = true
	if d.state.fence != nil {
		d.state.fence.consumed = true
	}
	component := &Component{authority: d.state.authority}
	d.state.authority = nil
	return component, nil
}

func buildApplications(mounts []mountRow) ([]applicationRow, []uint32, []uint32, map[callSource]uint32, []uint32, error) {
	items := make([]applicationKey, 0, len(mounts))
	appendItem := func(key applicationKey) { items = append(items, key) }
	for index, mount := range mounts {
		shard := uint32(index + 1)
		p := mount.program
		flowView := p.Flow()
		authored := flowView.Authored()
		callsView := authored.Calls()
		executable := flowView.Executable()
		directFunctions := flowView.DirectFunctions()
		functions := authored.Functions()
		for at := 0; at < callsView.Count(); at++ {
			call, ok := callsView.At(at)
			if !ok {
				return nil, nil, nil, nil, nil, errors.New("link/project: malformed Program Call table")
			}
			if !executable.Contains(call) {
				continue
			}
			callID, callOK := p.CallIDAt(at)
			if !callOK || !callID.Available() {
				return nil, nil, nil, nil, nil, errors.New("link/project: malformed executable Program Call occurrence proof")
			}
			_, callee, _, _, callOK := callsView.Get(call)
			if !callOK {
				return nil, nil, nil, nil, nil, errors.New("link/project: malformed Program Call")
			}
			if function, direct := directFunctions.For(callee); direct {
				_, body, _, valid := functions.Get(function)
				if !valid || !executable.Contains(function) || !executable.Contains(body) {
					return nil, nil, nil, nil, nil, errors.New("link/project: direct Call names malformed Function")
				}
			}
			appendItem(applicationKey{kind: applicationCall, shard: uint32(shard), term: call, callID: callID})
		}
		if err := appendFunctionStyleApplications(p, shard, appendItem); err != nil {
			return nil, nil, nil, nil, nil, err
		}
		module := p.Module()
		for at := 0; at < module.Count(); at++ {
			item, ok := module.ImportAt(at)
			if !ok {
				return nil, nil, nil, nil, nil, errors.New("link/project: malformed Program Import table")
			}
			row, valid := module.Import(item.Term)
			if !valid || row.Call == 0 {
				return nil, nil, nil, nil, nil, errors.New("link/project: malformed Program Import")
			}
			if executable.Contains(row.Call) {
				appendItem(applicationKey{kind: applicationImport, shard: uint32(shard), term: item.Term})
			}
		}
	}
	if !fits(len(items)) {
		return nil, nil, nil, nil, nil, errors.New("link/project: application handle overflow")
	}
	sort.Slice(items, func(left, right int) bool { return compareApplicationKey(items[left], items[right]) < 0 })
	applications := make([]applicationRow, len(items))
	lookup := make(map[applicationKey]uint32, len(items))
	bases := make([]uint32, 0, len(items))
	calls := make([]uint32, 0, len(items))
	// The map is a one-time owner-local inverse. Size it from the complete
	// application row count so Project sealing does not grow it repeatedly.
	callsBySource := make(map[callSource]uint32, len(items))
	imports := make([]uint32, 0, len(items))
	for index, item := range items {
		if index != 0 && compareApplicationKey(items[index-1], item) == 0 {
			return nil, nil, nil, nil, nil, errors.New("link/project: duplicate Application")
		}
		ordinal := uint32(index + 1)
		lookup[applicationLookupKey(item)] = ordinal
		applications[index] = applicationRow{kind: item.kind, shard: item.shard, term: item.term, slot: item.slot, callID: item.callID}
		switch item.kind {
		case applicationCall:
			formal := callApplicationID(item.callID)
			if !item.callID.Available() || !formal.Available() {
				return nil, nil, nil, nil, nil, errors.New("link/project: Call Application lacks occurrence identity")
			}
			applications[index].callFormal = formal
			bases = append(bases, ordinal)
			calls = append(calls, ordinal)
			source := callSource{shard: item.shard, callID: item.callID}
			if prior := callsBySource[source]; prior != 0 {
				return nil, nil, nil, nil, nil, errors.New("link/project: ambiguous Call Application")
			}
			callsBySource[source] = ordinal
		case applicationMeta, applicationGeneric:
			bases = append(bases, ordinal)
		case applicationImport:
			imports = append(imports, ordinal)
		}
	}
	importForCall := make(map[uint32]uint32, len(imports))
	for importIndex := range applications {
		item := &applications[importIndex]
		if item.kind != applicationImport {
			continue
		}
		p := mounts[item.shard-1].program
		row, ok := p.Module().Import(item.term)
		if !ok {
			return nil, nil, nil, nil, nil, errors.New("link/project: Import Application names malformed Program Import")
		}
		callOrdinal, found := lookup[applicationKey{kind: applicationCall, shard: item.shard, term: row.Call}]
		if !found {
			return nil, nil, nil, nil, nil, errors.New("link/project: Import Application has no Call")
		}
		importOrdinal := uint32(importIndex + 1)
		if prior := importForCall[callOrdinal]; prior != 0 && prior != importOrdinal {
			return nil, nil, nil, nil, nil, errors.New("link/project: Call has multiple Import Applications")
		}
		importForCall[callOrdinal] = importOrdinal
		item.root = callOrdinal
	}
	return applications, bases, calls, callsBySource, imports, nil
}

func appendFunctionStyleApplications(p *program.Program, shard uint32, appendItem func(applicationKey)) error {
	if p == nil {
		return errors.New("link/project: missing function-style Program")
	}
	flowView := p.Flow()
	executable := flowView.Executable()
	operators := flowView.Candidates()
	appendMeta := func(slot applicationSlot, count int, at func(int) (keyspace.Term, bool), fallback bool) error {
		for index := 0; index < count; index++ {
			source, ok := at(index)
			if !ok || !executable.Contains(source) {
				return errors.New("link/project: malformed Program metamethod source")
			}
			appendItem(applicationKey{kind: applicationMeta, shard: shard, term: source, slot: slot})
			if fallback {
				_, op, _, _, valid := flowView.Authored().Operators().Binaries().Get(source)
				if valid && (op == flowkind.BinaryLessEqual || op == flowkind.BinaryGreaterEqual) {
					appendItem(applicationKey{kind: applicationMeta, shard: shard, term: source, slot: applicationSlotOrderFallback})
				}
			}
		}
		return nil
	}
	if err := appendMeta(applicationSlotUnaryNumeric, operators.Unary().NumericCount(), operators.Unary().NumericAt, false); err != nil {
		return err
	}
	if err := appendMeta(applicationSlotLength, operators.Unary().LengthCount(), operators.Unary().LengthAt, false); err != nil {
		return err
	}
	if err := appendMeta(applicationSlotArithmetic, operators.Binary().ArithmeticCount(), operators.Binary().ArithmeticAt, false); err != nil {
		return err
	}
	if err := appendMeta(applicationSlotBitwise, operators.Binary().BitwiseCount(), operators.Binary().BitwiseAt, false); err != nil {
		return err
	}
	if err := appendMeta(applicationSlotConcat, operators.Binary().ConcatCount(), operators.Binary().ConcatAt, false); err != nil {
		return err
	}
	if err := appendMeta(applicationSlotEquality, operators.Binary().EqualityCount(), operators.Binary().EqualityAt, false); err != nil {
		return err
	}
	if err := appendMeta(applicationSlotOrderPrimary, operators.Binary().OrderCount(), operators.Binary().OrderAt, true); err != nil {
		return err
	}
	if err := appendMeta(applicationSlotIndexGet, operators.Access().GetCount(), operators.Access().GetAt, false); err != nil {
		return err
	}
	if err := appendMeta(applicationSlotIndexSet, operators.Access().SetCount(), operators.Access().SetAt, false); err != nil {
		return err
	}
	loops := flowView.Authored().Control().Loops()
	for index := 0; index < loops.Count(); index++ {
		loop, ok := loops.At(index)
		if !ok || !executable.Contains(loop) {
			continue
		}
		_, _, kind, _, valid := loops.Get(loop)
		if !valid || kind != flowkind.LoopGenericFor {
			continue
		}
		appendItem(applicationKey{kind: applicationGeneric, shard: shard, term: loop})
	}
	return nil
}

func compareApplicationKey(left, right applicationKey) int {
	a := [...]uint64{uint64(left.kind), uint64(left.shard), uint64(left.term), 0, 0, uint64(left.slot)}
	b := [...]uint64{uint64(right.kind), uint64(right.shard), uint64(right.term), 0, 0, uint64(right.slot)}
	for index := range a {
		if a[index] < b[index] {
			return -1
		}
		if a[index] > b[index] {
			return 1
		}
	}
	return 0
}

// applicationLookupKey deliberately strips the call identity from the
// construction-only physical-row lookup. Import roots name the existing
// portable Application row; the owner-local call inverse is populated from
// the sealed scalar call identity separately.
func applicationLookupKey(key applicationKey) applicationKey {
	key.callID = identity.ContentID{}
	return key
}

func canonicalMounts(input []Module) ([]mountRow, error) {
	mounts := make([]mountRow, len(input))
	names := make(map[string]struct{}, len(input))
	for index, item := range input {
		if item.Name == "" {
			return nil, fmt.Errorf("link/project: module %d has empty name", index)
		}
		if _, duplicate := names[item.Name]; duplicate {
			return nil, fmt.Errorf("link/project: duplicate module name %q", item.Name)
		}
		names[item.Name] = struct{}{}
		if item.Program == nil {
			return nil, fmt.Errorf("link/project: module %q has nil Program", item.Name)
		}
		id := item.Program.ContentID()
		if !id.Available() {
			return nil, fmt.Errorf("link/project: module %q has unavailable Program ContentID", item.Name)
		}
		key := moduleKeyID(item.Name, id)
		if !key.Available() {
			return nil, fmt.Errorf("link/project: module %q has unavailable ModuleKey", item.Name)
		}
		mounts[index] = mountRow{name: item.Name, program: item.Program, id: id, key: key}
	}
	sort.Slice(mounts, func(left, right int) bool {
		if order := bytes.Compare(mounts[left].id[:], mounts[right].id[:]); order != 0 {
			return order < 0
		}
		return mounts[left].name < mounts[right].name
	})
	return mounts, nil
}

func buildKeys(mounts []mountRow, contract *target.Contract) ([]keyRow, []uint32, map[vocabulary.InitialValue]uint32, [][]uint32, error) {
	unique := make(map[keyspace.LiteralValue]struct{})
	addExact := func(value keyspace.LiteralValue) error {
		normalized, ok := scalar.Normalize(value)
		if !ok {
			return errors.New("link/project: nil or NaN exact key")
		}
		unique[normalized] = struct{}{}
		return nil
	}
	addLiteral := func(value keyspace.LiteralValue) error {
		if normalized, ok := scalar.Normalize(value); ok {
			unique[normalized] = struct{}{}
		}
		return nil
	}
	for _, mount := range mounts {
		programKeys := mount.program.Source().Keys()
		for index := 0; index < programKeys.ExactCount(); index++ {
			key, _, ok := programKeys.ExactAt(index)
			if !ok {
				return nil, nil, nil, nil, errors.New("link/project: malformed Program exact key sequence")
			}
			value, ok := programKeys.Exact(key)
			if !ok || addExact(value) != nil {
				return nil, nil, nil, nil, errors.New("link/project: malformed Program exact key")
			}
		}
		if err := addProgramLiterals(mount.program, addLiteral); err != nil {
			return nil, nil, nil, nil, err
		}
	}
	for index := 0; index < contract.ExactKeyCount(); index++ {
		key, ok := contract.ExactKeyAt(index)
		if !ok {
			return nil, nil, nil, nil, errors.New("link/project: malformed Target exact key sequence")
		}
		value, ok := contract.ExactKeyValue(key)
		if !ok || addExact(value) != nil {
			return nil, nil, nil, nil, errors.New("link/project: malformed Target exact key")
		}
	}
	initialLiterals, err := addInitialLiterals(contract, addLiteral)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	if len(unique) > math.MaxUint32 {
		return nil, nil, nil, nil, errors.New("link/project: key handle overflow")
	}
	identities := make([]keyspace.LiteralValue, 0, len(unique))
	for identity := range unique {
		identities = append(identities, identity)
	}
	sort.Slice(identities, func(left, right int) bool {
		order, ok := scalar.Compare(identities[left], identities[right])
		return ok && order < 0
	})
	keys := make([]keyRow, len(identities))
	lookup := make(map[keyspace.LiteralValue]uint32, len(identities))
	for index, identity := range identities {
		keys[index] = keyRow{value: identity}
		lookup[identity] = uint32(index)
	}
	targetKeys := make([]uint32, contract.ExactKeyCount())
	for index := range targetKeys {
		targetKey, ok := contract.ExactKeyAt(index)
		value, valueOK := contract.ExactKeyValue(targetKey)
		normalized, normalizedOK := scalar.Normalize(value)
		mapped, found := lookup[normalized]
		if !ok || !valueOK || !normalizedOK || !found {
			return nil, nil, nil, nil, errors.New("link/project: missing sealed Target exact key")
		}
		targetKeys[index] = mapped
	}
	initialKeys := make(map[vocabulary.InitialValue]uint32, len(initialLiterals))
	for initial, value := range initialLiterals {
		normalized, ok := scalar.Normalize(value)
		mapped, found := lookup[normalized]
		if !ok || !found {
			return nil, nil, nil, nil, errors.New("link/project: missing sealed Target initial literal key")
		}
		initialKeys[initial] = mapped
	}
	programKeys := make([][]uint32, len(mounts))
	for mountIndex, mount := range mounts {
		programKeysView := mount.program.Source().Keys()
		count := programKeysView.ExactCount()
		programKeys[mountIndex] = make([]uint32, count)
		for index := 0; index < count; index++ {
			key, _, keyOK := programKeysView.ExactAt(index)
			value, valueOK := programKeysView.Exact(key)
			normalized, normalizedOK := scalar.Normalize(value)
			mapped, found := lookup[normalized]
			if !keyOK || key == 0 || uint64(key) > uint64(count) || !valueOK || !normalizedOK || !found {
				return nil, nil, nil, nil, errors.New("link/project: missing sealed Program exact key")
			}
			programKeys[mountIndex][key-1] = mapped
		}
	}
	return keys, targetKeys, initialKeys, programKeys, nil
}

func addProgramLiterals(p *program.Program, add func(keyspace.LiteralValue) error) error {
	if p == nil || add == nil {
		return errors.New("link/project: unavailable Program literal key authority")
	}
	literals := p.Source().Literals()
	bools := literals.Bools()
	for index := 0; index < bools.Count(); index++ {
		_, _, value, valid := bools.At(index)
		if !valid {
			return errors.New("link/project: malformed Program boolean literal")
		}
		if err := add(keyspace.LiteralValue{Kind: keyspace.LiteralBool, Bool: value}); err != nil {
			return err
		}
	}
	integers := literals.Integers()
	for index := 0; index < integers.Count(); index++ {
		_, _, value, valid := integers.At(index)
		if !valid {
			return errors.New("link/project: malformed Program integer literal")
		}
		if err := add(keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: value}); err != nil {
			return err
		}
	}
	floats := literals.Floats()
	for index := 0; index < floats.Count(); index++ {
		_, _, bits, valid := floats.At(index)
		if !valid {
			return errors.New("link/project: malformed Program float literal")
		}
		if err := add(keyspace.LiteralValue{Kind: keyspace.LiteralFloat, FloatBits: bits}); err != nil {
			return err
		}
	}
	strings := literals.Strings()
	for index := 0; index < strings.Count(); index++ {
		_, _, value, valid := strings.At(index)
		if !valid {
			return errors.New("link/project: malformed Program string literal")
		}
		if err := add(keyspace.LiteralValue{Kind: keyspace.LiteralString, String: value}); err != nil {
			return err
		}
	}
	return nil
}

func addInitialLiterals(contract *target.Contract, add func(keyspace.LiteralValue) error) (map[vocabulary.InitialValue]keyspace.LiteralValue, error) {
	if contract == nil || add == nil {
		return nil, errors.New("link/project: unavailable Target initial literal authority")
	}
	literals := make(map[vocabulary.InitialValue]keyspace.LiteralValue)
	addValue := func(value vocabulary.InitialValue) error {
		kind, ok := contract.InitialValueKind(value)
		if !ok {
			return errors.New("link/project: malformed Target initial value")
		}
		var literal keyspace.LiteralValue
		switch kind {
		case vocabulary.InitialValueBoolean:
			item, ok := contract.InitialValueBoolean(value)
			if !ok {
				return errors.New("link/project: malformed Target initial boolean")
			}
			literal = keyspace.LiteralValue{Kind: keyspace.LiteralBool, Bool: item}
		case vocabulary.InitialValueInteger:
			item, ok := contract.InitialValueInteger(value)
			if !ok {
				return errors.New("link/project: malformed Target initial integer")
			}
			literal = keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: item}
		case vocabulary.InitialValueFloat:
			item, ok := contract.InitialValueFloatBits(value)
			if !ok {
				return errors.New("link/project: malformed Target initial float")
			}
			literal = keyspace.LiteralValue{Kind: keyspace.LiteralFloat, FloatBits: item}
		case vocabulary.InitialValueString:
			item, ok := contract.InitialValueString(value)
			if !ok {
				return errors.New("link/project: malformed Target initial string")
			}
			literal = keyspace.LiteralValue{Kind: keyspace.LiteralString, String: item}
		case vocabulary.InitialValueNil, vocabulary.InitialValueRoot, vocabulary.InitialValueOperation, vocabulary.InitialValueDeniedOperation, vocabulary.InitialValueAbsent:
			return nil
		default:
			return errors.New("link/project: malformed Target initial value kind")
		}
		if _, ok := scalar.Normalize(literal); ok {
			literals[value] = literal
		}
		return add(literal)
	}
	for index := 0; index < contract.InitialRootCount(); index++ {
		root, rootOK := contract.InitialRootAt(index)
		shape, shapeOK := contract.InitialRootBootShape(root)
		value, valueOK := contract.BootShapeValue(shape)
		if !rootOK || !shapeOK || !valueOK {
			return nil, errors.New("link/project: malformed Target initial root")
		}
		if err := addValue(value); err != nil {
			return nil, err
		}
	}
	for index := 0; index < contract.InitialEntryCount(); index++ {
		_, _, value, _, ok := contract.InitialEntryAt(index)
		if !ok {
			return nil, errors.New("link/project: malformed Target initial entry")
		}
		if err := addValue(value); err != nil {
			return nil, err
		}
	}
	for index := 0; index < contract.InitialBindingCount(); index++ {
		_, _, value, _, _, ok := contract.InitialBindingAt(index)
		if !ok {
			return nil, errors.New("link/project: malformed Target initial binding")
		}
		if err := addValue(value); err != nil {
			return nil, err
		}
	}
	return literals, nil
}
