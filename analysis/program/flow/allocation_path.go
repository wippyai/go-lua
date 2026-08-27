package flow

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/executable"
	"github.com/wippyai/go-lua/analysis/program/flow/semanticpath"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/relation/schema/region"
)

const (
	allocationRoleTable   uint8 = 1
	allocationRoleClosure uint8 = 2
	allocationIDPrefix          = "program-flow-allocation-v1"
	allocationIDPrefixLen       = len(allocationIDPrefix)
	fieldIDPrefix               = "program-flow-allocation-field-v1"
	fieldIDPrefixLen            = len(fieldIDPrefix)
)

// AllocationID returns the owner-issued identity for one executable table or
// closure occurrence.  The authored term and executable denominator are the
// canonical coordinates; Flow does not expose an Allocation proof object or
// make callers reconstruct one from a private index.
func (view View) AllocationID(term keyspace.Term) (identity.ContentID, bool) {
	if !view.available() || view.component.semanticPaths == nil ||
		!view.component.semanticPaths.Matches(view.component.provenance.Source, view.component.provenance.Flow, view.component.provenance.Static, view.component.provenance.Module) ||
		!view.Executable().Contains(term) {
		return identity.ContentID{}, false
	}
	family := keyspace.TermFamily(term)
	var role uint8
	switch family {
	case keyspace.FamilyTable:
		if _, ok := view.Authored().Tables().Get(term); !ok {
			return identity.ContentID{}, false
		}
		role = allocationRoleTable
	case keyspace.FamilyFunction:
		if _, _, _, ok := view.Authored().Functions().Get(term); !ok {
			return identity.ContentID{}, false
		}
		role = allocationRoleClosure
	default:
		return identity.ContentID{}, false
	}
	p := view.component.provenance
	path, pathOK := certificateTerm(view.component.semanticPaths, p.Source, p.Flow, p.Static, p.Module, term)
	if !pathOK {
		return identity.ContentID{}, false
	}
	id := digestPath("allocation", path, uint32(role), 0, source.Span{})
	return id, id.Available()
}

// AllocationFieldID returns the owner-issued identity for one authored table
// field.  Field membership is checked against the Tables denominator so the
// caller consumes the existing authored relation instead of constructing a
// second field proof or index.
func (view View) AllocationFieldID(table, field keyspace.Term) (identity.ContentID, bool) {
	_, ok := view.AllocationID(table)
	if !ok || keyspace.TermFamily(table) != keyspace.FamilyTable || keyspace.TermFamily(field) != keyspace.FamilyTableField {
		return identity.ContentID{}, false
	}
	count, ok := view.Authored().Tables().FieldCount(table)
	if !ok {
		return identity.ContentID{}, false
	}
	member := false
	for index := 0; index < count; index++ {
		candidate, candidateOK := view.Authored().Tables().FieldAt(table, index)
		if !candidateOK {
			return identity.ContentID{}, false
		}
		if candidate == field {
			member = true
			break
		}
	}
	if !member {
		return identity.ContentID{}, false
	}
	programID := view.ContentID()
	var allocationPayload [allocationIDPrefixLen + sha256.Size + 1 + 4]byte
	copy(allocationPayload[:allocationIDPrefixLen], allocationIDPrefix)
	copy(allocationPayload[allocationIDPrefixLen:allocationIDPrefixLen+sha256.Size], programID[:])
	allocationPayload[allocationIDPrefixLen+sha256.Size] = allocationRoleTable
	binary.BigEndian.PutUint32(allocationPayload[allocationIDPrefixLen+sha256.Size+1:], uint32(table))
	allocationProof := sha256.Sum256(allocationPayload[:])
	var payload [fieldIDPrefixLen + sha256.Size + 4]byte
	copy(payload[:fieldIDPrefixLen], fieldIDPrefix)
	copy(payload[fieldIDPrefixLen:fieldIDPrefixLen+sha256.Size], allocationProof[:])
	binary.BigEndian.PutUint32(payload[fieldIDPrefixLen+sha256.Size:], uint32(field))
	return sha256.Sum256(payload[:]), true
}

func (view View) BodyPath(term keyspace.Term) (identity.ContentID, bool) {
	if !view.available() || view.component.semanticPaths == nil || keyspace.TermFamily(term) != keyspace.FamilyBody {
		return identity.ContentID{}, false
	}
	p := view.component.provenance
	return view.component.semanticPaths.BodyPathAt(p.Source, p.Flow, p.Static, p.Module, keyspace.TermOrdinal(term))
}
func (view View) CallPath(term keyspace.Term) (identity.ContentID, bool) {
	if !view.available() || view.component.semanticPaths == nil ||
		!view.component.semanticPaths.Matches(view.component.provenance.Source, view.component.provenance.Flow, view.component.provenance.Static, view.component.provenance.Module) ||
		keyspace.TermFamily(term) != keyspace.FamilyCall {
		return identity.ContentID{}, false
	}
	owner, _, _, _, callOK := view.Authored().Calls().Get(term)
	p := view.component.provenance
	body, bodyOK := view.component.semanticPaths.BodyPathAt(p.Source, p.Flow, p.Static, p.Module, keyspace.TermOrdinal(owner))
	path, pathOK := certificateTerm(view.component.semanticPaths, p.Source, p.Flow, p.Static, p.Module, term)
	if !callOK || !bodyOK || !pathOK {
		return identity.ContentID{}, false
	}
	id := digestBytes("call-occurrence", body, path)
	return id, id.Available()
}

// SemanticTermPath forwards one already-sealed semantic-path certificate row.
// It is a narrow owner-fenced join for Program certificate rows; Flow does not expose
// the certificate plane or create a second term index.
func (view View) SemanticTermPath(term keyspace.Term) (identity.ContentID, bool) {
	if !view.available() || view.component.semanticPaths == nil {
		return identity.ContentID{}, false
	}
	family, ordinal := keyspace.TermFamily(term), keyspace.TermOrdinal(term)
	if family <= keyspace.FamilyInvalid || family >= keyspace.FamilyCount || ordinal == 0 {
		return identity.ContentID{}, false
	}
	p := view.component.provenance
	return view.component.semanticPaths.TermPathAt(p.Source, p.Flow, p.Static, p.Module, family, ordinal)
}

// SemanticTermAtom forwards the neutral Boolean proposition sealed beside one
// exact semantic Flow term. The atom is issued once with the path certificate;
// callers never derive it from a runtime coordinate, scope ordinal, or
// physical digest.
func (view View) SemanticTermAtom(term keyspace.Term) (region.Atom, bool) {
	if !view.available() || view.component.semanticPaths == nil {
		return region.Atom{}, false
	}
	family, ordinal := keyspace.TermFamily(term), keyspace.TermOrdinal(term)
	if family <= keyspace.FamilyInvalid || family >= keyspace.FamilyCount || ordinal == 0 {
		return region.Atom{}, false
	}
	p := view.component.provenance
	return view.component.semanticPaths.TermAtomAt(p.Source, p.Flow, p.Static, p.Module, family, ordinal)
}
func certificateTerm(paths *semanticpath.Certificate, sourceID, flowID, staticID, moduleID identity.ContentID, term keyspace.Term) (identity.ContentID, bool) {
	return paths.TermPathAt(sourceID, flowID, staticID, moduleID, keyspace.TermFamily(term), keyspace.TermOrdinal(term))
}

func validateCertificateValueSourcePaths(sourceView source.View, view authored.View, paths *semanticpath.Certificate, staticID, moduleID identity.ContentID) error {
	sourceID, flowID := sourceView.Identity().ContentID(), view.ContentID()
	if !paths.Matches(sourceID, flowID, staticID, moduleID) {
		return errors.New("certificate provenance disagrees")
	}
	store := func(term keyspace.Term) error {
		id, ok := certificateTerm(paths, sourceID, flowID, staticID, moduleID, term)
		if !ok || !id.Available() {
			return errors.New("certificate value-source path is unavailable")
		}
		return nil
	}
	l := sourceView.Literals()
	for i := 0; i < l.Nils().Count(); i++ {
		x, _, ok := l.Nils().At(i)
		if !ok {
			return errors.New("nil source row unavailable")
		}
		if err := store(x); err != nil {
			return err
		}
	}
	for i := 0; i < l.Bools().Count(); i++ {
		x, _, _, ok := l.Bools().At(i)
		if !ok {
			return errors.New("bool source row unavailable")
		}
		if err := store(x); err != nil {
			return err
		}
	}
	for i := 0; i < l.Integers().Count(); i++ {
		x, _, _, ok := l.Integers().At(i)
		if !ok {
			return errors.New("integer source row unavailable")
		}
		if err := store(x); err != nil {
			return err
		}
	}
	for i := 0; i < l.Floats().Count(); i++ {
		x, _, _, ok := l.Floats().At(i)
		if !ok {
			return errors.New("float source row unavailable")
		}
		if err := store(x); err != nil {
			return err
		}
	}
	for i := 0; i < l.Strings().Count(); i++ {
		x, _, _, ok := l.Strings().At(i)
		if !ok {
			return errors.New("string source row unavailable")
		}
		if err := store(x); err != nil {
			return err
		}
	}
	t := view.TypeValues()
	for i := 0; i < t.Count(); i++ {
		x, ok := t.At(i)
		if !ok {
			return errors.New("TypeValue source row unavailable")
		}
		if _, ok := t.Get(x); !ok {
			return errors.New("TypeValue owner unavailable")
		}
		if err := store(x); err != nil {
			return err
		}
	}
	return nil
}
func validateCertificateStoragePaths(sourceView source.View, view authored.View, paths *semanticpath.Certificate, staticID, moduleID identity.ContentID) error {
	sourceID, flowID := sourceView.Identity().ContentID(), view.ContentID()
	if !paths.Matches(sourceID, flowID, staticID, moduleID) {
		return errors.New("certificate provenance disagrees")
	}
	a := view.Storage().Assigns()
	for i := 0; i < a.Count(); i++ {
		term, ok := a.At(i)
		if !ok {
			return errors.New("assignment row unavailable")
		}
		if _, _, ok := a.Get(term); !ok {
			return errors.New("assignment owner unavailable")
		}
		id, ok := certificateTerm(paths, sourceID, flowID, staticID, moduleID, term)
		if !ok || !id.Available() {
			return errors.New("assignment certificate path unavailable")
		}
	}
	return nil
}
func validateCertificateAllocationPaths(sourceView source.View, exec *executable.Result, view authored.View, paths *semanticpath.Certificate, staticID, moduleID identity.ContentID) error {
	sourceID, flowID := sourceView.Identity().ContentID(), view.ContentID()
	if !paths.Matches(sourceID, flowID, staticID, moduleID) {
		return errors.New("certificate provenance disagrees")
	}
	store := func(term keyspace.Term, role uint8) error {
		if !exec.Contains(term) {
			return nil
		}
		id, ok := certificateTerm(paths, sourceID, flowID, staticID, moduleID, term)
		if !ok || !digestPath("allocation", id, uint32(role), 0, source.Span{}).Available() {
			return errors.New("allocation certificate path unavailable")
		}
		return nil
	}
	for i := 0; i < view.Tables().Count(); i++ {
		term, ok := view.Tables().At(i)
		if !ok {
			return errors.New("table allocation row unavailable")
		}
		if err := store(term, allocationRoleTable); err != nil {
			return err
		}
	}
	for i := 0; i < view.Functions().Count(); i++ {
		term, ok := view.Functions().At(i)
		if !ok {
			return errors.New("closure allocation row unavailable")
		}
		if err := store(term, allocationRoleClosure); err != nil {
			return err
		}
	}
	return nil
}
func validateCertificateCallPaths(sourceView source.View, view authored.View, paths *semanticpath.Certificate, staticID, moduleID identity.ContentID) error {
	sourceID, flowID := sourceView.Identity().ContentID(), view.ContentID()
	if !paths.Matches(sourceID, flowID, staticID, moduleID) {
		return errors.New("certificate provenance disagrees")
	}
	c := view.Calls()
	for i := 0; i < c.Count(); i++ {
		term, ok := c.At(i)
		if !ok {
			return errors.New("call row unavailable")
		}
		owner, _, _, _, ok := c.Get(term)
		if !ok {
			return errors.New("call owner unavailable")
		}
		body, bok := paths.BodyPathAt(sourceID, flowID, staticID, moduleID, keyspace.TermOrdinal(owner))
		id, iok := certificateTerm(paths, sourceID, flowID, staticID, moduleID, term)
		if !bok || !iok || !digestBytes("call-occurrence", body, id).Available() {
			return errors.New("call certificate path unavailable")
		}
	}
	return nil
}
func digestPath(label string, parent identity.ContentID, role, aux uint32, span source.Span) identity.ContentID {
	var p [88]byte
	copy(p[:], label)
	copy(p[32:64], parent[:])
	binary.BigEndian.PutUint32(p[64:68], role)
	binary.BigEndian.PutUint32(p[68:72], aux)
	binary.BigEndian.PutUint32(p[72:76], span.StartLine)
	binary.BigEndian.PutUint32(p[76:80], span.StartCol)
	binary.BigEndian.PutUint32(p[80:84], span.EndLine)
	binary.BigEndian.PutUint32(p[84:88], span.EndCol)
	return sha256.Sum256(p[:])
}
func digestBytes(label string, parent, value identity.ContentID) identity.ContentID {
	var p [96]byte
	copy(p[:], label)
	copy(p[32:64], parent[:])
	copy(p[64:96], value[:])
	return sha256.Sum256(p[:])
}
