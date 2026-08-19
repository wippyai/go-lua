// Package programmount declares the Link's mount directory: the coordinate
// space of mounted programs, the column each mount's frozen compiled program
// is published in, and the semantic role the space is identified by.
//
// A Link mounts programs; a mounted program is a compiled artifact placed at
// one module key. That placement is a Link fact and belongs to the Link's own
// publication, while the compiled program it names is content produced once
// and shared by every Link that mounts it. The column states exactly that
// join, and it is the only place the join is stated: a consumer resolves a
// mount by module key and reads the frozen program's cold facts through it,
// rather than being handed a compiled artifact and a module identity as two
// separate arguments it has to keep consistent.
//
// The coordinate is the Link module key and nothing else. A module key is
// already the identity a Link derives its mount-qualified addresses from, so
// naming a mount a second way here would make one concept answer to two
// vocabularies while the digest preimages went on using the first.
//
// The column is total over the Link's module directory: the key universe is
// published with it, so a module key inside that universe with no row is an
// absent mount as a fact rather than as ignorance, and a key outside it is
// not a mount of this Link at all.
package programmount

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/ingress"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	"github.com/wippyai/go-lua/analysis/snapshot"
)

// The identities this package declares. Each is authored here and named from
// here, so the rows and the references that resolve them are one package's
// statement.
const (
	// AxisKey is this coordinate space's authored identity, and therefore the
	// identity of the principal admitted to write the column below: the Link
	// composition that placed the mounts publishes it as this axis.
	AxisKey schema.Key = "program-mount"
	// OutputKey is the one column this axis publishes: each mounted module
	// key against the frozen program mounted there.
	OutputKey schema.Key = "program-mount/programs"
	// AxisRole is the semantic role this axis is identified by. The space is
	// not a factor, so it is declared under the axis namespace of the role
	// vocabulary rather than the factor one.
	AxisRole = "axis/program-mount"
)

// SchemaFragment and HotAxis name the cold and hot halves an axis declaration
// is typed against. This axis declares neither, so neither is ever produced;
// they exist so the declaration reads at the same types a bound axis's does.
type (
	SchemaFragment struct{}
	HotAxis        struct{}
)

// Program is one Link mount row. ModuleKey belongs to this row because the
// same compiled content can be mounted at more than one module key.
type Program struct {
	ModuleKey identity.ContentID
	programschema.Program
}

func (row Program) Available() bool {
	return row.ModuleKey.Available() && row.Program.Available()
}

// ProgramFromSnapshot constructs the mount row from the neutral ingress
// publication and the module key that places it in a Link directory.
func ProgramFromSnapshot(source *ingress.Snapshot, module identity.ContentID) (Program, bool) {
	if source == nil || !source.Available() || !module.Available() {
		return Program{}, false
	}
	row := Program{
		ModuleKey: module,
		Program: programschema.Program{
			Frozen: source.Frozen(), ArtifactID: source.ArtifactID(),
			ProgramID: source.ProgramID(), SchemaID: source.SchemaID(),
		},
	}
	return row, row.Available()
}

// MountedArtifact is one Link mount: the mount directory row that names the
// program's compiled publication, and the ingress view that still carries
// families which have not moved onto it yet. Contributors receive no
// ProgramArtifact through either.
//
// The directory row is embedded rather than held beside a second copy of the
// module and program identities, so there is one authority for where this
// mount is and what it mounts.
type MountedArtifact struct {
	Program
	Snapshot *ingress.Snapshot
}

func (row MountedArtifact) Available() bool {
	return row.Program.Available() && row.Snapshot != nil && row.Snapshot.Available() &&
		row.Snapshot.ProgramID() == row.ProgramID && row.Snapshot.ArtifactID() == row.ArtifactID
}

// AxisEntry is this package's axis declaration. A is the composition's own
// Link input record: this axis names nothing in it, because it mounts no
// authority of its own and binds no factor against one.
func AxisEntry[A any]() axis.Spec[A] {
	return axis.Spec[A]{
		Key:     AxisKey,
		Storage: axis.StorageEngine,
		// The Link's module directory is the key universe this column is
		// total over, so the column is published together with that
		// denominator and a module key inside it with no row is a published
		// absence rather than ignorance.
		Cardinality: axis.CardinalityDense,
		// The directory is derived from the mounts of one Link and dies with
		// the binding that mounted them, and the composition publishes the
		// column once: no rule writes it afterwards.
		Lifetime:    axis.LifetimeLink,
		Mutability:  axis.MutabilityFrozen,
		Concurrency: axis.ConcurrencyShared,
		Frame:       axis.Frame{Outputs: []axis.Output{{Key: OutputKey, Writer: AxisKey}}},
		Semantic:    vocabulary.RoleKey(AxisRole),
	}
}

// StructureSpecs contributes this axis's one semantic role to the structural
// vocabulary. The role is declared beside the axis that consumes it, so the
// composition does not maintain a second role inventory.
func StructureSpecs() []structure.Spec { return vocabulary.RoleSpecs(AxisRole) }

// Content seals the mount directory of one Link into the column's payload.
// The rows and the denominator's membership are the same module keys, because
// the column is total over exactly the directory it publishes: it is one
// statement, so the two can never drift.
//
// A duplicate module key, an unavailable key, and a row that does not
// authenticate against the key it is stored under are all rejected. A Link
// with no mount seals no directory at all.
func Content(rows []Program, denominator identity.ContentID) (snapshot.Content[identity.ContentID, Program], bool) {
	if len(rows) == 0 || !denominator.Available() {
		return snapshot.Content[identity.ContentID, Program]{}, false
	}
	mounted := make(map[identity.ContentID]Program, len(rows))
	members := make([]identity.ContentID, 0, len(rows))
	for _, row := range rows {
		if !row.Available() {
			return snapshot.Content[identity.ContentID, Program]{}, false
		}
		if _, duplicate := mounted[row.ModuleKey]; duplicate {
			return snapshot.Content[identity.ContentID, Program]{}, false
		}
		mounted[row.ModuleKey] = row
		members = append(members, row.ModuleKey)
	}
	return snapshot.Content[identity.ContentID, Program]{
		Rows: mounted, Denominator: denominator, Members: members,
	}, true
}

// Axis is the address of the mount directory column of one Link publication.
func Axis(runtimeSchema identity.ContentID, slot uint32) snapshot.Axis[identity.ContentID, Program] {
	return snapshot.Axis[identity.ContentID, Program]{SchemaID: runtimeSchema, Slot: slot}
}

// DenominatorID is the identity of one Link's module directory: the key
// universe the mount column is total over. It is derived from the Link's own
// identity, so two Links never share a directory.
func DenominatorID(linkID identity.ContentID) (identity.ContentID, bool) {
	if !linkID.Available() {
		return identity.ContentID{}, false
	}
	return identity.DeriveContentID("analysis/program-mount-directory/v1", linkID[:])
}

// Mounted resolves one module key against a Link publication's mount
// directory. A key the directory covers with no row is an absent mount as a
// published fact; a key outside the directory is not a mount of this Link.
func Mounted(published *snapshot.Snapshot, address snapshot.Axis[identity.ContentID, Program], module identity.ContentID) (Program, bool) {
	row, status := snapshot.Read(published, address, module)
	return row, status == snapshot.ReadHit && row.Available()
}
