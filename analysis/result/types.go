package result

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/link"
)

// ValueCoordinate is the Link substitution for one Value factor coordinate.
type ValueCoordinate struct {
	id    identity.ContentID
	mount identity.ContentID
}

// NewValueCoordinate admits one value identity at a mount.
func NewValueCoordinate(id, mount identity.ContentID) (ValueCoordinate, bool) {
	if !id.Available() || !mount.Available() {
		return ValueCoordinate{}, false
	}
	return ValueCoordinate{id: id, mount: mount}, true
}

// Coordinates projects the Link boundary's canonical Value rows into the
// mount-qualified coordinates consumed by Geometry. The result package owns
// this projection because coordinates are result identity inputs; callers do
// not need to reconstruct the same mount join in a compiler facade.
func Coordinates(source *link.Link) ([]ValueCoordinate, bool) {
	if source == nil || source.Project() == nil || source.Boundary() == nil {
		return nil, false
	}
	values := source.Boundary().Values()
	if values.Count() == 0 {
		return nil, false
	}
	rows := make([]ValueCoordinate, values.Count())
	seen := make(map[struct {
		mount identity.ContentID
		id    identity.ContentID
	}]struct{}, len(rows))
	for index := range rows {
		value, valueOK := values.At(index)
		id, idOK := values.ID(value)
		shard, _, originOK := values.Origin(value)
		mounted, programOK := source.Project().Mounts().Program(shard)
		module, moduleOK := source.Project().ModuleKey(shard)
		if !valueOK || !idOK || !originOK || !programOK || mounted == nil || !moduleOK || !id.Available() || !module.Available() {
			return nil, false
		}
		key := struct {
			mount identity.ContentID
			id    identity.ContentID
		}{mount: module, id: id}
		if _, duplicate := seen[key]; duplicate {
			return nil, false
		}
		seen[key] = struct{}{}
		coordinate, coordinateOK := NewValueCoordinate(id, module)
		if !coordinateOK {
			return nil, false
		}
		rows[index] = coordinate
	}
	return rows, true
}

// ID returns the canonical Value identity carried by this coordinate.
func (coordinate ValueCoordinate) ID() identity.ContentID { return coordinate.id }

// MountID returns the Link mount substitution carried by this coordinate.
func (coordinate ValueCoordinate) MountID() identity.ContentID { return coordinate.mount }
