// Package executioncatalog owns the engine-private sealed address table from
// member rows to typed execution families. It is deliberately internal: a
// schema/domain/composition package cannot forge a rule/member/candidate row.
package executioncatalog

type Ref uint32

type Row struct {
	Family, Local            uint32
	Rule, Member             uint32
	Candidate                uint32
	inputStart, inputCount   uint32
	outputStart, outputCount uint32
}

// Catalog.At provides the presence proof. A read-only invocation can have no
// output ports, so zero outputCount is a valid sealed row.
func (row Row) Valid() bool { return true }

// Draft is construction-only input. Seal packs its handle vectors into the
// catalog's two flat immutable columns; no member owns a slice at runtime.
type Draft struct {
	Family, Local           uint32
	Rule, Member            uint32
	Candidate               uint32
	InputCount, OutputCount uint16
}

type Catalog struct {
	rows            []Row
	inputs, outputs []uint32
}

func Seal(drafts []Draft) (*Catalog, bool) {
	if len(drafts) == 0 {
		return &Catalog{}, true
	}
	inputCount, outputCount := 0, 0
	members := make(map[uint32]struct{}, len(drafts))
	for _, draft := range drafts {
		if _, duplicate := members[draft.Member]; duplicate {
			return nil, false
		}
		members[draft.Member] = struct{}{}
		inputCount += int(draft.InputCount)
		outputCount += int(draft.OutputCount)
	}
	rows := make([]Row, len(drafts))
	inputs := make([]uint32, 0, inputCount)
	outputs := make([]uint32, 0, outputCount)
	for index, draft := range drafts {
		if uint64(len(inputs)) > uint64(^uint32(0)) || uint64(len(outputs)) > uint64(^uint32(0)) {
			return nil, false
		}
		row := Row{Family: draft.Family, Local: draft.Local, Rule: draft.Rule, Member: draft.Member, Candidate: draft.Candidate, inputStart: uint32(len(inputs)), inputCount: uint32(draft.InputCount), outputStart: uint32(len(outputs)), outputCount: uint32(draft.OutputCount)}
		for handle := uint16(0); handle < draft.InputCount; handle++ {
			inputs = append(inputs, uint32(handle))
		}
		for handle := uint16(0); handle < draft.OutputCount; handle++ {
			outputs = append(outputs, uint32(handle))
		}
		rows[index] = row
	}
	return &Catalog{rows: rows, inputs: inputs, outputs: outputs}, true
}

func (catalog *Catalog) Count() int {
	if catalog == nil {
		return 0
	}
	return len(catalog.rows)
}

func (catalog *Catalog) At(ref Ref) (Row, bool) {
	if catalog == nil || uint64(ref) >= uint64(len(catalog.rows)) {
		return Row{}, false
	}
	return catalog.rows[ref], true
}

func (catalog *Catalog) Inputs(row Row) ([]uint32, bool) {
	if catalog == nil || !row.Valid() || uint64(row.inputStart)+uint64(row.inputCount) > uint64(len(catalog.inputs)) {
		return nil, false
	}
	return catalog.inputs[row.inputStart : row.inputStart+row.inputCount], true
}

func (catalog *Catalog) Outputs(row Row) ([]uint32, bool) {
	if catalog == nil || !row.Valid() || uint64(row.outputStart)+uint64(row.outputCount) > uint64(len(catalog.outputs)) {
		return nil, false
	}
	return catalog.outputs[row.outputStart : row.outputStart+row.outputCount], true
}

func (row Row) FamilyOrdinal() uint32    { return row.Family }
func (row Row) LocalOrdinal() uint32     { return row.Local }
func (row Row) RuleOrdinal() uint32      { return row.Rule }
func (row Row) MemberOrdinal() uint32    { return row.Member }
func (row Row) CandidateOrdinal() uint32 { return row.Candidate }
