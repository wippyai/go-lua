package residence

import (
	"crypto/sha256"
	"encoding/binary"

	programartifact "github.com/wippyai/go-lua/analysis/internal/programartifact"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
)

// ArtifactMount is the complete post-seal mount receipt consumed by
// Residence.  It deliberately contains no Project Shard: mount, module and
// Program are exact scalar identities, while the reusable Artifact remains
// Program-owned.
type ArtifactMount struct {
	artifact *programartifact.Artifact
	mount    keyspace.ContentID
	module   keyspace.ContentID
	program  keyspace.ContentID
}

func NewArtifactMount(artifact *programartifact.Artifact, mount, module, program keyspace.ContentID) (ArtifactMount, bool) {
	if artifact == nil || !artifact.Available() || !mount.Available() || !module.Available() || !program.Available() || artifact.CompileKey().ProgramID() != program {
		return ArtifactMount{}, false
	}
	return ArtifactMount{artifact: artifact, mount: mount, module: module, program: program}, true
}

func (mount ArtifactMount) Available() bool {
	return mount.artifact != nil && mount.artifact.Available() && mount.mount.Available() && mount.module.Available() && mount.program.Available() && mount.artifact.CompileKey().ProgramID() == mount.program
}
func (mount ArtifactMount) Artifact() *programartifact.Artifact {
	if !mount.Available() {
		return nil
	}
	return mount.artifact
}
func (mount ArtifactMount) MountID() keyspace.ContentID {
	if !mount.Available() {
		return keyspace.ContentID{}
	}
	return mount.mount
}
func (mount ArtifactMount) Module() keyspace.ContentID {
	if !mount.Available() {
		return keyspace.ContentID{}
	}
	return mount.module
}
func (mount ArtifactMount) ProgramID() keyspace.ContentID {
	if !mount.Available() {
		return keyspace.ContentID{}
	}
	return mount.program
}

func (owner *schema) addProgramBoundaries(mounts []ArtifactMount) bool {
	for _, mount := range mounts {
		if !mount.Available() {
			return false
		}
		class, classOK := owner.mountClasses[mount.mount]
		if !classOK {
			return false
		}
		for index := 0; index < mount.artifact.BoundaryCount(); index++ {
			row, ok := mount.artifact.BoundaryAt(index)
			if !ok || !row.Available() || row.Kind() == programartifact.BoundaryStore && !row.Eligible() {
				continue
			}
			if !owner.addBoundaryClass(boundaryRow{kind: boundaryKind(row.Kind()), id: residenceProgramBoundaryID(owner.linkOwner, mount.mount, row)}, class) {
				return false
			}
		}
	}
	return true
}

func boundaryKind(kind programartifact.BoundaryKind) BoundaryKind {
	switch kind {
	case programartifact.BoundaryCapture:
		return BoundaryCapture
	case programartifact.BoundaryStore:
		return BoundaryStore
	case programartifact.BoundaryReturn:
		return BoundaryReturn
	default:
		return BoundaryInvalid
	}
}

func residenceProgramBoundaryID(owner link.OwnerCapability, mountID keyspace.ContentID, row programartifact.BoundaryRow) keyspace.ContentID {
	if !owner.Available() || !mountID.Available() || !row.Available() {
		return keyspace.ContentID{}
	}
	linkID := owner.ContentID()
	var image [120]byte
	copy(image[:32], linkID[:])
	copy(image[32:64], mountID[:])
	binary.BigEndian.PutUint64(image[64:], 0x7265732d617274) // res-art
	binary.BigEndian.PutUint64(image[72:], uint64(row.Kind()))
	rowID := row.ID()
	copy(image[80:112], rowID[:])
	position, _ := row.Position()
	binary.BigEndian.PutUint64(image[112:], uint64(position))
	return keyspace.ContentID(sha256.Sum256(image[:]))
}

func (owner *schema) validArtifactBoundary(kind BoundaryKind, mount ArtifactMount, row programartifact.BoundaryRow) bool {
	return owner != nil && mount.Available() && row.Available() && boundaryKind(row.Kind()) == kind && mount.artifact.CompileKey().ProgramID() == mount.program && (kind != BoundaryStore || row.Eligible())
}
