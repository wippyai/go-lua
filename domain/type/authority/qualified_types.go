package typeauthority

import (
	"errors"
	"strings"

	programstaticnode "github.com/wippyai/go-lua/analysis/schema/program/staticnode"
	"github.com/wippyai/go-lua/domain/type/typ"
)

// QualifiedType is one owner-qualified type the Link's target publishes under
// its exact authored name. It is the value form of one sealed qualified type
// index row: the target owns the name and the denominator, and the type domain
// owns the reading, so the reading arrives here as a finished type rather than
// as a declaration this package would have to decode.
type QualifiedType struct {
	Name  string
	Value typ.Type
}

// qualifiedDirectory is the immutable name-to-type directory a canonical
// reference resolves through. A Link that mounts a target that publishes no
// qualified type carries an empty directory, and every canonical reference
// then refuses by name.
type qualifiedDirectory map[string]typ.Type

func newQualifiedDirectory(types []QualifiedType) (qualifiedDirectory, error) {
	if len(types) == 0 {
		return nil, nil
	}
	directory := make(qualifiedDirectory, len(types))
	for _, entry := range types {
		if entry.Name == "" || entry.Value == nil {
			return nil, errors.New("typeauthority: incomplete qualified type row")
		}
		if _, duplicate := directory[entry.Name]; duplicate {
			return nil, errors.New("typeauthority: duplicate qualified type " + entry.Name)
		}
		directory[entry.Name] = entry.Value
	}
	return directory, nil
}

// resolveCanonical materializes the declaration one canonical reference names.
// The path is read from the reference's own canonical key spelling, so the
// reference resolves to exactly the qualified type it was authored against and
// a path the directory does not publish is refused under that path rather than
// silently becoming Unknown.
func (a *artifactResolver) resolveCanonical(row programstaticnode.StaticTypeNode) (typ.Type, bool) {
	path, pathOK := a.canonicalPath(row)
	if !pathOK {
		return nil, false
	}
	value, published := a.authority.qualified[path]
	if !published || value == nil {
		a.authority.refuse("typeauthority: no qualified type named " + path)
		return nil, false
	}
	return value, true
}

// canonicalPath reads one reference's canonical path back as its exact
// authored spelling. The spelling is a column of the published row: a
// canonical reference names a declaration outside this Program, so its name
// travels with it rather than being rebuilt from a Program-local key table.
func (a *artifactResolver) canonicalPath(row programstaticnode.StaticTypeNode) (string, bool) {
	view, index, viewOK := a.programIndex(row)
	if !viewOK {
		return "", false
	}
	_, count, spanOK := row.ReferenceCanonicalKeySpan()
	if !spanOK || count == 0 {
		return "", false
	}
	segments := make([]string, 0, count)
	for position := 0; position < int(count); position++ {
		key, keyOK := view.StaticTypeNodeReferenceCanonicalKeyFor(index, position)
		if !keyOK || !key.Available() || key.ParentID() != row.ID() || key.Position() != uint32(position) {
			return "", false
		}
		segments = append(segments, key.Text())
	}
	return strings.Join(segments, "."), true
}
