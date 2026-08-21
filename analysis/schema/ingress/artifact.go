// Package ingress lowers a sealed ProgramArtifact into closed immutable
// columns exactly once per ContentID. After Lower succeeds the snapshot
// retains no owner pointer and cannot reopen artifact interiors.
package ingress

import (
	"github.com/wippyai/go-lua/analysis/identity"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/schema/program"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	programfamily "github.com/wippyai/go-lua/analysis/schema/program/family"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	snapshotstore "github.com/wippyai/go-lua/analysis/snapshot"
)

// Snapshot is the sealed ingress receipt: closed identity columns projected
// once from a ProgramArtifact. Accessors read these columns; they never hold
// or reopen the owner.
type Snapshot struct {
	artifactID  identity.ContentID
	programID   identity.ContentID
	schemaID    identity.ContentID
	entryBodyID identity.ContentID
	frozen      snapshotstore.Frozen
	coldCatalog identity.ContentID
}

func (snapshot *Snapshot) Available() bool {
	return snapshot != nil && snapshot.artifactID.Available() && snapshot.programID.Available() && snapshot.schemaID.Available() && snapshot.entryBodyID.Available() && snapshot.frozen.Published() && snapshot.coldCatalog.Available()
}

func vocabularyAuthority(vocabulary structure.Table) bool {
	return vocabulary.Count(structure.CategoryArm) != 0 &&
		vocabulary.Count(structure.CategoryEvent) != 0 &&
		vocabulary.Count(structure.CategoryOutcome) != 0
}

// coldView is the address a snapshot reads its cold families at. It travels
// inside the rows that name a child span, so a span is rejoined where it is
// read and the snapshot never holds a second copy of a published plane.
type coldView struct {
	frozen  *snapshotstore.Frozen
	catalog identity.ContentID
}

func (snapshot *Snapshot) coldView() coldView {
	if snapshot == nil {
		return coldView{}
	}
	return coldView{frozen: &snapshot.frozen, catalog: snapshot.coldCatalog}
}

func coldCount[V programfamily.Row](view coldView, family programfamily.Family[V]) (int, bool) {
	if view.frozen == nil {
		return 0, false
	}
	return family.Count(view.frozen, view.catalog)
}

func coldRow[V programfamily.Row](view coldView, family programfamily.Family[V], index int) (V, bool) {
	var absent V
	if view.frozen == nil {
		return absent, false
	}
	return family.At(view.frozen, view.catalog, index)
}

func artifactAuthority(artifact *programartifact.Artifact) bool {
	return artifact != nil && artifact.Available()
}

func (snapshot *Snapshot) ArtifactID() identity.ContentID {
	if !snapshot.Available() {
		return identity.ContentID{}
	}
	return snapshot.artifactID
}
func (snapshot *Snapshot) ProgramID() identity.ContentID {
	if !snapshot.Available() {
		return identity.ContentID{}
	}
	return snapshot.programID
}
func (snapshot *Snapshot) SchemaID() identity.ContentID {
	if !snapshot.Available() {
		return identity.ContentID{}
	}
	return snapshot.schemaID
}

// EntryBodyID returns the owner-issued executable-root Body identity carried
// through this immutable ingress receipt. It is never reconstructed from the
// ordered Body plane.
func (snapshot *Snapshot) EntryBodyID() identity.ContentID {
	if !snapshot.Available() {
		return identity.ContentID{}
	}
	return snapshot.entryBodyID
}

// Frozen returns this snapshot's neutral compiled publication. The publication
// carries no mount identity; a mount row adds its module key at the boundary
// that owns placement.
func (snapshot *Snapshot) Frozen() snapshotstore.Frozen {
	if !snapshot.Available() {
		return snapshotstore.Frozen{}
	}
	return snapshot.frozen
}

// Program returns the one canonical compiled Program named by this ingress
// publication. Families already owned by Program are consumed through this
// value rather than copied into another ingress row vocabulary.
func (snapshot *Snapshot) Program() programschema.Program {
	if !snapshot.Available() {
		return programschema.Program{}
	}
	return programschema.Program{
		Frozen: snapshot.frozen, ArtifactID: snapshot.artifactID,
		ProgramID: snapshot.programID, SchemaID: snapshot.schemaID, EntryBodyID: snapshot.entryBodyID,
	}
}

// Lower projects one sealed artifact through the sealed structural vocabulary
// into closed columns. The returned snapshot retains no owner pointer.
func Lower(artifact *programartifact.Artifact, vocabulary structure.Table) (*Snapshot, bool) {
	if !artifactAuthority(artifact) || !vocabularyAuthority(vocabulary) {
		return nil, false
	}
	program := artifact.Program()
	coldCatalog, catalogOK := programcatalog.CatalogID(program.SchemaID)
	if !program.Available() || !catalogOK {
		return nil, false
	}
	snapshot := &Snapshot{
		artifactID:  program.ArtifactID,
		programID:   program.ProgramID,
		schemaID:    program.SchemaID,
		entryBodyID: program.EntryBodyID,
		frozen:      program.Frozen,
		coldCatalog: coldCatalog,
	}
	// Every published route arm must name a member of the sealed structural
	// vocabulary. The admission is stated once here over the published plane;
	// the arm each reader receives is projected at the read site.
	edgeCount, edgesPublished := coldCount(snapshot.coldView(), programschema.EnvironmentEdgeFamily())
	if !edgesPublished {
		return nil, false
	}
	for index := 0; index < edgeCount; index++ {
		row, held := coldRow(snapshot.coldView(), programschema.EnvironmentEdgeFamily(), index)
		if !held {
			return nil, false
		}
		if _, armOK := vocabulary.At(structure.CategoryArm, uint16(row.Arm())); !armOK {
			return nil, false
		}
	}
	// Every published order bracket must name a member of the sealed
	// structural vocabulary. The admission is stated once here over the
	// published plane; the kind each reader receives is projected at the read
	// site.
	eventCount, eventsPublished := coldCount(snapshot.coldView(), programschema.WTOEventFamily())
	if !eventsPublished {
		return nil, false
	}
	for index := 0; index < eventCount; index++ {
		row, held := coldRow(snapshot.coldView(), programschema.WTOEventFamily(), index)
		if !held {
			return nil, false
		}
		if _, kindOK := vocabulary.At(structure.CategoryEvent, uint16(row.Kind())); !kindOK {
			return nil, false
		}
	}
	ruleCount, rulesPublished := program.RuleOccurrenceCount()
	occurrenceCount, occurrencesPublished := program.OccurrenceCount()
	occurrencePointCount, occurrencePointsPublished := programschema.OccurrencePointFamily().Count(&snapshot.frozen, snapshot.coldCatalog)
	occurrenceInputCount, occurrenceInputsPublished := programschema.OccurrenceInputFamily().Count(&snapshot.frozen, snapshot.coldCatalog)
	if !rulesPublished || !occurrencesPublished || !occurrencePointsPublished || !occurrenceInputsPublished {
		return nil, false
	}
	for index := 0; index < occurrenceCount; index++ {
		row, ok := program.OccurrenceAt(index)
		pointOffset, pointWidth, pointSpanOK := row.PointSpan()
		inputOffset, inputWidth, inputSpanOK := row.InputSpan()
		if !ok || !row.Available() || !pointSpanOK || !inputSpanOK || uint64(pointOffset)+uint64(pointWidth) > uint64(occurrencePointCount) || uint64(inputOffset)+uint64(inputWidth) > uint64(occurrenceInputCount) {
			return nil, false
		}
	}
	for index := 0; index < ruleCount; index++ {
		row, ok := program.RuleOccurrenceAt(index)
		if !ok || !row.Available() {
			return nil, false
		}
		parent, parentOK := row.Occurrence()
		if !parentOK || uint64(parent) >= uint64(occurrenceCount) {
			return nil, false
		}
	}
	// Every published call must name operand and argument spans the two child
	// planes actually hold. The admission is stated once here over the
	// published planes; each child row is rejoined at the read site.
	callView := snapshot.coldView()
	callCount, callsPublished := coldCount(callView, programschema.CallFamily())
	operandCount, operandsPublished := coldCount(callView, programschema.CallOperandFamily())
	argumentCount, argumentsPublished := coldCount(callView, programschema.CallArgumentFamily())
	if !callsPublished || !operandsPublished || !argumentsPublished {
		return nil, false
	}
	if _, typeArgumentsPublished := coldCount(callView, programschema.CallTypeArgumentFamily()); !typeArgumentsPublished {
		return nil, false
	}
	for index := 0; index < callCount; index++ {
		row, held := coldRow(callView, programschema.CallFamily(), index)
		if !held {
			return nil, false
		}
		operandOffset, operandWidth, operandSpanOK := row.OperandSpan()
		argumentOffset, argumentWidth, argumentSpanOK := row.ArgumentSpan()
		if !operandSpanOK || !argumentSpanOK ||
			uint64(operandOffset)+uint64(operandWidth) > uint64(operandCount) ||
			uint64(argumentOffset)+uint64(argumentWidth) > uint64(argumentCount) {
			return nil, false
		}
	}
	return snapshot, snapshot.Available()
}
