package modulecomposition

import (
	"github.com/wippyai/go-lua/analysis/identity"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
)

// InitGeneration is one module initialization generation for a cache ingress.
// Construction validates the target mount once, while the sealed row keeps
// only the target module/program identities and body identity.
type InitGeneration struct {
	id, link, ingressID, moduleKey, artifactID, programID, bodyID identity.ContentID
}

// NewInitGeneration constructs a generation only when the ingress target and
// mounted Program agree and body is exactly that Program's unique entry Body.
// Link, mount key, ArtifactID, ProgramID, and body identity all participate in
// its stable identity.
func NewInitGeneration(ingress CacheIngress, mount programmount.Program, body programschema.Body) (InitGeneration, bool) {
	if !ingress.Available() || !mount.Available() || !body.Available() || ingress.TargetModuleKey() != mount.ModuleKey {
		return InitGeneration{}, false
	}
	entry, ok := mount.Program.EntryBody()
	if !ok || entry != body {
		return InitGeneration{}, false
	}
	row := InitGeneration{
		link: ingress.LinkID(), ingressID: ingress.ID(), moduleKey: mount.ModuleKey,
		artifactID: mount.ArtifactID, programID: mount.ProgramID, bodyID: body.ID(),
	}
	row.id = initGenerationID(row)
	return row, row.Available()
}

func (row InitGeneration) Available() bool {
	return row.id.Available() && row.link.Available() && row.ingressID.Available() && row.moduleKey.Available() &&
		row.artifactID.Available() && row.programID.Available() && row.bodyID.Available() &&
		row.id == initGenerationID(row)
}
func (row InitGeneration) ID() identity.ContentID {
	if row.Available() {
		return row.id
	}
	return identity.ContentID{}
}
func (row InitGeneration) LinkID() identity.ContentID {
	if row.Available() {
		return row.link
	}
	return identity.ContentID{}
}
func (row InitGeneration) CacheIngressID() identity.ContentID {
	if row.Available() {
		return row.ingressID
	}
	return identity.ContentID{}
}
func (row InitGeneration) ModuleKey() identity.ContentID {
	if row.Available() {
		return row.moduleKey
	}
	return identity.ContentID{}
}
func (row InitGeneration) ArtifactID() identity.ContentID {
	if row.Available() {
		return row.artifactID
	}
	return identity.ContentID{}
}
func (row InitGeneration) ProgramID() identity.ContentID {
	if row.Available() {
		return row.programID
	}
	return identity.ContentID{}
}
func (row InitGeneration) BodyID() identity.ContentID {
	if row.Available() {
		return row.bodyID
	}
	return identity.ContentID{}
}

// InitOutcome is one outcome witness of one initialization generation.
// OutcomeID and Kind are the canonical Program outcome identity and kind;
// ordinal is the canonical one-based ModuleEntry ReturnOrdinal for a Return.
type InitOutcome struct {
	id, link, generationID, outcomeID identity.ContentID
	kind                              programschema.OutcomeKind
	ordinal                           uint32
}

// NewInitOutcome authenticates one canonical Body-owned Outcome against the
// target Program and generation. Return outcomes must also have their
// canonical ModuleEntry ReturnOrdinal; callers that already hold that witness
// may use NewInitOutcomeFromModuleEntry.
func NewInitOutcome(generation InitGeneration, mount programmount.Program, outcome programschema.Outcome) (InitOutcome, bool) {
	if !generationMountMatches(generation, mount) || !outcome.Available() || !admittedOutcome(outcome.Kind()) {
		return InitOutcome{}, false
	}
	body, bodyOK := mount.Program.EntryBody()
	if !bodyOK || body.ID() != generation.BodyID() {
		return InitOutcome{}, false
	}
	canonical, canonicalOK := programBodyOutcome(mount.Program, body, outcome)
	if !canonicalOK || canonical != outcome {
		return InitOutcome{}, false
	}
	ordinal := uint32(0)
	if outcome.Kind() == programschema.OutcomeReturn {
		var ordinalOK bool
		ordinal, ordinalOK = programReturnOrdinal(mount.Program, body, outcome.ID())
		if !ordinalOK {
			return InitOutcome{}, false
		}
	}
	row := InitOutcome{link: generation.LinkID(), generationID: generation.ID(), outcomeID: outcome.ID(), kind: outcome.Kind(), ordinal: ordinal}
	row.id = initOutcomeID(row)
	return row, row.Available()
}

// NewInitOutcomeFromModuleEntry authenticates one canonical ModuleEntry and
// its ReturnID against the generation's canonical entry Body.
func NewInitOutcomeFromModuleEntry(generation InitGeneration, mount programmount.Program, entry programschema.ModuleEntry) (InitOutcome, bool) {
	if !generationMountMatches(generation, mount) || !entry.Available() {
		return InitOutcome{}, false
	}
	body, bodyOK := mount.Program.EntryBody()
	if !bodyOK || body.ID() != generation.BodyID() || !programModuleEntry(mount.Program, entry) {
		return InitOutcome{}, false
	}
	outcome, outcomeOK := programOutcomeByID(mount.Program, body, entry.ReturnID())
	if !outcomeOK || outcome.Kind() != programschema.OutcomeReturn {
		return InitOutcome{}, false
	}
	ordinal, ordinalOK := entry.ReturnOrdinal()
	if !ordinalOK {
		return InitOutcome{}, false
	}
	row := InitOutcome{link: generation.LinkID(), generationID: generation.ID(), outcomeID: outcome.ID(), kind: outcome.Kind(), ordinal: ordinal}
	row.id = initOutcomeID(row)
	return row, row.Available()
}

func (row InitOutcome) Available() bool {
	return row.id.Available() && row.link.Available() && row.generationID.Available() && row.outcomeID.Available() &&
		admittedOutcome(row.kind) && (row.kind == programschema.OutcomeReturn || row.ordinal == 0) && row.id == initOutcomeID(row)
}
func (row InitOutcome) ID() identity.ContentID {
	if row.Available() {
		return row.id
	}
	return identity.ContentID{}
}
func (row InitOutcome) LinkID() identity.ContentID {
	if row.Available() {
		return row.link
	}
	return identity.ContentID{}
}
func (row InitOutcome) GenerationID() identity.ContentID {
	if row.Available() {
		return row.generationID
	}
	return identity.ContentID{}
}
func (row InitOutcome) OutcomeID() identity.ContentID {
	if row.Available() {
		return row.outcomeID
	}
	return identity.ContentID{}
}
func (row InitOutcome) Kind() programschema.OutcomeKind {
	if row.Available() {
		return row.kind
	}
	return programschema.OutcomeInvalid
}
func (row InitOutcome) ReturnOrdinal() (uint32, bool) {
	return row.ordinal, row.Available() && row.kind == programschema.OutcomeReturn
}

// InitTerminal is the terminal projection of one init outcome. It carries
// only the stable outcome/generation join, so terminal membership cannot copy
// or wrap an outcome row.
type InitTerminal struct {
	id, link, generationID, outcomeID identity.ContentID
}

func NewInitTerminal(outcome InitOutcome) (InitTerminal, bool) {
	if !outcome.Available() || outcome.Kind() != programschema.OutcomeThrow && outcome.Kind() != programschema.OutcomeCancel {
		return InitTerminal{}, false
	}
	row := InitTerminal{link: outcome.LinkID(), generationID: outcome.GenerationID(), outcomeID: outcome.OutcomeID()}
	row.id = initTerminalID(row)
	return row, row.Available()
}

func (row InitTerminal) Available() bool {
	return row.id.Available() && row.link.Available() && row.generationID.Available() && row.outcomeID.Available() &&
		row.id == initTerminalID(row)
}
func (row InitTerminal) ID() identity.ContentID {
	if row.Available() {
		return row.id
	}
	return identity.ContentID{}
}
func (row InitTerminal) LinkID() identity.ContentID {
	if row.Available() {
		return row.link
	}
	return identity.ContentID{}
}
func (row InitTerminal) GenerationID() identity.ContentID {
	if row.Available() {
		return row.generationID
	}
	return identity.ContentID{}
}
func (row InitTerminal) OutcomeID() identity.ContentID {
	if row.Available() {
		return row.outcomeID
	}
	return identity.ContentID{}
}

func generationMountMatches(generation InitGeneration, mount programmount.Program) bool {
	return generation.Available() && mount.Available() && generation.ModuleKey() == mount.ModuleKey &&
		generation.ArtifactID() == mount.ArtifactID && generation.ProgramID() == mount.ProgramID
}

func admittedOutcome(kind programschema.OutcomeKind) bool {
	switch kind {
	case programschema.OutcomeNormal, programschema.OutcomeReturn, programschema.OutcomeThrow,
		programschema.OutcomeYield, programschema.OutcomeCancel:
		return true
	default:
		return false
	}
}

func programBodyIndex(program programschema.Program, body programschema.Body) (int, bool) {
	count, published := program.BodyCount()
	if !published || !body.Available() {
		return 0, false
	}
	index := -1
	for candidateIndex := 0; candidateIndex < count; candidateIndex++ {
		candidate, held := program.BodyAt(candidateIndex)
		if !held || !candidate.Available() {
			return 0, false
		}
		if candidate != body {
			continue
		}
		if index >= 0 {
			return 0, false
		}
		index = candidateIndex
	}
	return index, index >= 0
}

func programBodyOutcome(program programschema.Program, body programschema.Body, wanted programschema.Outcome) (programschema.Outcome, bool) {
	bodyIndex, bodyOK := programBodyIndex(program, body)
	if !bodyOK || !wanted.Available() {
		return programschema.Outcome{}, false
	}
	found := programschema.Outcome{}
	for childIndex := 0; childIndex < body.OutcomeCount(); childIndex++ {
		candidate, held := program.BodyOutcomeFor(bodyIndex, childIndex)
		if !held {
			return programschema.Outcome{}, false
		}
		if candidate.ID() != wanted.ID() {
			continue
		}
		if found.Available() || candidate != wanted {
			return programschema.Outcome{}, false
		}
		found = candidate
	}
	return found, found.Available()
}

func programOutcomeByID(program programschema.Program, body programschema.Body, outcomeID identity.ContentID) (programschema.Outcome, bool) {
	bodyIndex, bodyOK := programBodyIndex(program, body)
	if !bodyOK || !outcomeID.Available() {
		return programschema.Outcome{}, false
	}
	var found programschema.Outcome
	for childIndex := 0; childIndex < body.OutcomeCount(); childIndex++ {
		candidate, held := program.BodyOutcomeFor(bodyIndex, childIndex)
		if !held {
			return programschema.Outcome{}, false
		}
		if candidate.ID() != outcomeID {
			continue
		}
		if found.Available() {
			return programschema.Outcome{}, false
		}
		found = candidate
	}
	return found, found.Available()
}

func programModuleEntry(program programschema.Program, wanted programschema.ModuleEntry) bool {
	count, published := program.ModuleEntryCount()
	if !published || !wanted.Available() {
		return false
	}
	found := false
	for index := 0; index < count; index++ {
		candidate, held := program.ModuleEntryAt(index)
		if !held || !candidate.Available() {
			return false
		}
		if candidate.ID() != wanted.ID() {
			continue
		}
		if found || candidate != wanted {
			return false
		}
		found = true
	}
	return found
}

func programReturnOrdinal(program programschema.Program, body programschema.Body, outcomeID identity.ContentID) (uint32, bool) {
	if _, ok := programOutcomeByID(program, body, outcomeID); !ok {
		return 0, false
	}
	count, published := program.ModuleEntryCount()
	if !published {
		return 0, false
	}
	var ordinal uint32
	found := false
	for index := 0; index < count; index++ {
		entry, held := program.ModuleEntryAt(index)
		if !held || !entry.Available() {
			return 0, false
		}
		if entry.ReturnID() != outcomeID {
			continue
		}
		candidate, ordinalOK := entry.ReturnOrdinal()
		if !ordinalOK || found {
			return 0, false
		}
		ordinal, found = candidate, true
	}
	return ordinal, found
}
