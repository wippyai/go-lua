package flow

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/executable"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/semanticpath"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

// All occurrence identities below are projections of semanticpath.Certificate.
// Flow has no Body/root/containment path builder or retained path plane.
type allocationPath struct{ id identity.ContentID }

func (view View) BodyPath(term keyspace.Term) (identity.ContentID, bool) {
	if !view.available() || view.component.semanticPaths == nil || keyspace.TermFamily(term) != keyspace.FamilyBody {
		return identity.ContentID{}, false
	}
	p := view.component.provenance
	return view.component.semanticPaths.BodyPathAt(p.Source, p.Flow, p.Static, p.Module, keyspace.TermOrdinal(term))
}
func (view View) CallPath(term keyspace.Term) (identity.ContentID, bool) {
	if !view.available() || view.component.semanticPaths == nil || !view.component.semanticPaths.Matches(view.component.provenance.Source, view.component.provenance.Flow, view.component.provenance.Static, view.component.provenance.Module) || keyspace.TermFamily(term) != keyspace.FamilyCall || keyspace.TermOrdinal(term) == 0 || uint64(keyspace.TermOrdinal(term)) > uint64(len(view.component.callPaths)) {
		return identity.ContentID{}, false
	}
	id := view.component.callPaths[keyspace.TermOrdinal(term)-1]
	return id, id.Available()
}
func (view View) ValueSourcePath(term keyspace.Term) (identity.ContentID, bool) {
	family, ordinal := keyspace.TermFamily(term), keyspace.TermOrdinal(term)
	if !view.available() || view.component.semanticPaths == nil || !view.component.semanticPaths.Matches(view.component.provenance.Source, view.component.provenance.Flow, view.component.provenance.Static, view.component.provenance.Module) || family <= keyspace.FamilyInvalid || family >= keyspace.FamilyCount || ordinal == 0 || uint64(ordinal) > uint64(len(view.component.valueSourcePaths[family])) {
		return identity.ContentID{}, false
	}
	id := view.component.valueSourcePaths[family][ordinal-1]
	return id, id.Available()
}

// SemanticTermPath forwards one already-sealed semantic-path certificate row.
// It is a narrow owner-fenced join for Program receipts; Flow does not expose
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
func (view View) StorageAssignmentPath(term keyspace.Term) (identity.ContentID, bool) {
	if !view.available() || view.component.semanticPaths == nil || !view.component.semanticPaths.Matches(view.component.provenance.Source, view.component.provenance.Flow, view.component.provenance.Static, view.component.provenance.Module) || keyspace.TermFamily(term) != keyspace.FamilyAssign || keyspace.TermOrdinal(term) == 0 || uint64(keyspace.TermOrdinal(term)) > uint64(len(view.component.storagePaths[keyspace.FamilyAssign])) {
		return identity.ContentID{}, false
	}
	id := view.component.storagePaths[keyspace.FamilyAssign][keyspace.TermOrdinal(term)-1]
	return id, id.Available()
}

type AllocationOccurrence struct {
	owner   *Component
	program identity.ContentID
	role    AllocationRole
	id      identity.ContentID
}

func (o AllocationOccurrence) Available() bool {
	return o.owner != nil && o.owner.semanticPaths != nil && o.owner.semanticPaths.Matches(o.owner.provenance.Source, o.owner.provenance.Flow, o.owner.provenance.Static, o.owner.provenance.Module) && o.program == o.owner.ContentID() && o.role.Valid() && o.id.Available()
}
func (o AllocationOccurrence) ID() identity.ContentID {
	if !o.Available() {
		return identity.ContentID{}
	}
	return o.id
}
func (o AllocationOccurrence) Equal(other AllocationOccurrence) bool {
	return o.Available() && other.Available() && o.owner == other.owner && o.program == other.program && o.role == other.role && o.id == other.id
}
func (c *Component) allocationOccurrence(term keyspace.Term, role AllocationRole) AllocationOccurrence {
	if c == nil || !role.Valid() {
		return AllocationOccurrence{}
	}
	family, ordinal := keyspace.TermFamily(term), keyspace.TermOrdinal(term)
	if family <= keyspace.FamilyInvalid || family >= keyspace.FamilyCount || ordinal == 0 || uint64(ordinal) > uint64(len(c.allocationPaths[family])) {
		return AllocationOccurrence{}
	}
	id := c.allocationPaths[family][ordinal-1].id
	if !id.Available() {
		return AllocationOccurrence{}
	}
	return AllocationOccurrence{owner: c, program: c.ContentID(), role: role, id: id}
}

func certificateTerm(paths *semanticpath.Certificate, sourceID, flowID, staticID, moduleID identity.ContentID, term keyspace.Term) (identity.ContentID, bool) {
	return paths.TermPathAt(sourceID, flowID, staticID, moduleID, keyspace.TermFamily(term), keyspace.TermOrdinal(term))
}

func sealCertificateValueSourcePaths(sourceView source.View, view authored.View, paths *semanticpath.Certificate, staticID, moduleID identity.ContentID) ([keyspace.FamilyCount][]identity.ContentID, error) {
	sourceID, flowID := sourceView.Identity().ContentID(), view.Cold().ContentID()
	if !paths.Matches(sourceID, flowID, staticID, moduleID) {
		return [keyspace.FamilyCount][]identity.ContentID{}, errors.New("certificate provenance disagrees")
	}
	var out [keyspace.FamilyCount][]identity.ContentID
	for f := keyspace.Family(1); f < keyspace.FamilyCount; f++ {
		if n := sourceView.Identity().FamilyCount(f); n > 0 {
			out[f] = make([]identity.ContentID, n)
		}
	}
	store := func(term keyspace.Term) error {
		f, o := keyspace.TermFamily(term), keyspace.TermOrdinal(term)
		id, ok := certificateTerm(paths, sourceID, flowID, staticID, moduleID, term)
		if !ok || o == 0 || uint64(o) > uint64(len(out[f])) || out[f][o-1].Available() {
			return errors.New("certificate value-source path is unavailable")
		}
		out[f][o-1] = id
		return nil
	}
	l := sourceView.Literals()
	for i := 0; i < l.Nils().Count(); i++ {
		x, _, ok := l.Nils().At(i)
		if !ok {
			return out, errors.New("nil source row unavailable")
		}
		if err := store(x); err != nil {
			return out, err
		}
	}
	for i := 0; i < l.Bools().Count(); i++ {
		x, _, _, ok := l.Bools().At(i)
		if !ok {
			return out, errors.New("bool source row unavailable")
		}
		if err := store(x); err != nil {
			return out, err
		}
	}
	for i := 0; i < l.Integers().Count(); i++ {
		x, _, _, ok := l.Integers().At(i)
		if !ok {
			return out, errors.New("integer source row unavailable")
		}
		if err := store(x); err != nil {
			return out, err
		}
	}
	for i := 0; i < l.Floats().Count(); i++ {
		x, _, _, ok := l.Floats().At(i)
		if !ok {
			return out, errors.New("float source row unavailable")
		}
		if err := store(x); err != nil {
			return out, err
		}
	}
	for i := 0; i < l.Strings().Count(); i++ {
		x, _, _, ok := l.Strings().At(i)
		if !ok {
			return out, errors.New("string source row unavailable")
		}
		if err := store(x); err != nil {
			return out, err
		}
	}
	t := view.TypeValues()
	for i := 0; i < t.Count(); i++ {
		x, ok := t.At(i)
		if !ok {
			return out, errors.New("TypeValue source row unavailable")
		}
		if _, ok := t.Get(x); !ok {
			return out, errors.New("TypeValue owner unavailable")
		}
		if err := store(x); err != nil {
			return out, err
		}
	}
	return out, nil
}
func sealCertificateStoragePaths(sourceView source.View, view authored.View, paths *semanticpath.Certificate, staticID, moduleID identity.ContentID) ([keyspace.FamilyCount][]identity.ContentID, error) {
	sourceID, flowID := sourceView.Identity().ContentID(), view.Cold().ContentID()
	if !paths.Matches(sourceID, flowID, staticID, moduleID) {
		return [keyspace.FamilyCount][]identity.ContentID{}, errors.New("certificate provenance disagrees")
	}
	var out [keyspace.FamilyCount][]identity.ContentID
	out[keyspace.FamilyAssign] = make([]identity.ContentID, sourceView.Identity().FamilyCount(keyspace.FamilyAssign))
	a := view.Storage().Assigns()
	for i := 0; i < a.Count(); i++ {
		term, ok := a.At(i)
		if !ok {
			return out, errors.New("assignment row unavailable")
		}
		if _, _, ok := a.Get(term); !ok {
			return out, errors.New("assignment owner unavailable")
		}
		o := keyspace.TermOrdinal(term)
		id, ok := certificateTerm(paths, sourceID, flowID, staticID, moduleID, term)
		if !ok || o == 0 || uint64(o) > uint64(len(out[keyspace.FamilyAssign])) {
			return out, errors.New("assignment certificate path unavailable")
		}
		out[keyspace.FamilyAssign][o-1] = id
	}
	return out, nil
}
func sealCertificateAllocationPaths(sourceView source.View, exec *executable.Result, view authored.View, paths *semanticpath.Certificate, staticID, moduleID identity.ContentID) ([keyspace.FamilyCount][]allocationPath, error) {
	sourceID, flowID := sourceView.Identity().ContentID(), view.Cold().ContentID()
	if !paths.Matches(sourceID, flowID, staticID, moduleID) {
		return [keyspace.FamilyCount][]allocationPath{}, errors.New("certificate provenance disagrees")
	}
	var out [keyspace.FamilyCount][]allocationPath
	for f := keyspace.Family(1); f < keyspace.FamilyCount; f++ {
		if n := sourceView.Identity().FamilyCount(f); n > 0 {
			out[f] = make([]allocationPath, n)
		}
	}
	store := func(term keyspace.Term, role AllocationRole) error {
		if !exec.Executable(term) {
			return nil
		}
		f, o := keyspace.TermFamily(term), keyspace.TermOrdinal(term)
		id, ok := certificateTerm(paths, sourceID, flowID, staticID, moduleID, term)
		if !ok || o == 0 || uint64(o) > uint64(len(out[f])) || out[f][o-1].id.Available() {
			return errors.New("allocation certificate path unavailable")
		}
		out[f][o-1] = allocationPath{id: digestPath("allocation", id, uint32(role), 0, source.Span{})}
		return nil
	}
	for i := 0; i < view.Tables().Count(); i++ {
		term, ok := view.Tables().At(i)
		if !ok {
			return out, errors.New("table allocation row unavailable")
		}
		if err := store(term, AllocationTable); err != nil {
			return out, err
		}
	}
	for i := 0; i < view.Functions().Count(); i++ {
		term, ok := view.Functions().At(i)
		if !ok {
			return out, errors.New("closure allocation row unavailable")
		}
		if err := store(term, AllocationClosure); err != nil {
			return out, err
		}
	}
	return out, nil
}
func sealCertificateCallPaths(sourceView source.View, view authored.View, paths *semanticpath.Certificate, staticID, moduleID identity.ContentID) ([]identity.ContentID, error) {
	sourceID, flowID := sourceView.Identity().ContentID(), view.Cold().ContentID()
	if !paths.Matches(sourceID, flowID, staticID, moduleID) {
		return nil, errors.New("certificate provenance disagrees")
	}
	c := view.Calls()
	out := make([]identity.ContentID, c.Count())
	for i := 0; i < c.Count(); i++ {
		term, ok := c.At(i)
		if !ok {
			return nil, errors.New("call row unavailable")
		}
		owner, _, _, _, ok := c.Get(term)
		if !ok {
			return nil, errors.New("call owner unavailable")
		}
		body, bok := paths.BodyPathAt(sourceID, flowID, staticID, moduleID, keyspace.TermOrdinal(owner))
		id, iok := certificateTerm(paths, sourceID, flowID, staticID, moduleID, term)
		o := keyspace.TermOrdinal(term)
		if !bok || !iok || o == 0 || uint64(o) > uint64(len(out)) {
			return nil, errors.New("call certificate path unavailable")
		}
		out[o-1] = digestBytes("call-occurrence", body, id)
	}
	return out, nil
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
