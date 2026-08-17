package static

import (
	"crypto/sha256"
	"errors"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/domain/type/authority"
)

// sealRuntime transfers the complete evaluated structural denominator into
// typeauthority exactly once. ClassSet keeps only its Pack carrier after this
// call; no runtime reflection graph or mutable bridge points back into Static.
func (s *ClassSet) sealRuntime() (*typeauthority.Runtime, error) {
	if s == nil || s.authority == nil || s.authority.types == nil {
		return nil, errors.New("static: Runtime seal source unavailable")
	}
	inputs := make([]typeauthority.RuntimeInput, 0, len(s.rows))
	inputByClass := make(map[uint32]typeauthority.RuntimeInput, len(s.rows))
	classByInput := make([]uint32, 0, len(s.rows))
	for index := 1; index < len(s.rows); index++ {
		row := s.rows[index]
		if row.kind != ClassConcrete {
			continue
		}
		input := row.input
		inputs = append(inputs, input)
		inputByClass[uint32(index)] = input
		classByInput = append(classByInput, uint32(index))
	}

	runtime, mapped, err := typeauthority.SealRuntime(s.authority.types, inputs)
	if err != nil {
		return nil, err
	}
	if runtime == nil || len(mapped) != len(classByInput) {
		return nil, errors.New("static: malformed Runtime seal projection")
	}
	seenInner := make(map[typeauthority.RuntimeInner]struct{}, len(mapped))
	for index, inner := range mapped {
		classIndex := classByInput[index]
		if uint64(classIndex) >= uint64(len(s.rows)) {
			return nil, errors.New("static: Runtime input class out of range")
		}
		s.rows[classIndex].inner = inner
		if _, duplicate := seenInner[inner]; duplicate {
			return nil, errors.New("static: duplicate concrete class escaped canonical row admission")
		}
		seenInner[inner] = struct{}{}
	}
	if err := s.sealTypeValueOccurrences(runtime); err != nil {
		return nil, err
	}
	return runtime, nil
}

// sealTypeValueOccurrences is Static's sole occurrence grounding pass.  It
// joins an existing executable Boundary Value to the already-contextual
// Static result and, only for a concrete result, to Runtime's structural row.
func (s *ClassSet) sealTypeValueOccurrences(runtime *typeauthority.Runtime) error {
	if s == nil || s.authority == nil || runtime == nil {
		return errors.New("static: TypeValue occurrence source unavailable")
	}
	// A Static authority is admitted through ProgramArtifact rows. Keeping a
	// Link/Program traversal here would make the old composition a second
	// production path and would retain authored terms past the seal boundary.
	if len(s.authority.mounts) == 0 {
		return errors.New("static: TypeValue artifacts required")
	}
	return s.sealMountedTypeValueOccurrences(runtime)
}

// sealMountedTypeValueOccurrences grounds TypeValue rows solely through the
// immutable artifact occurrence ID and Link Boundary mounted-value receipt.
// No source graph or authored coordinate is reopened here.
func (s *ClassSet) sealMountedTypeValueOccurrences(runtime *typeauthority.Runtime) error {
	a := s.authority
	if a == nil || a.types == nil || runtime == nil {
		return errors.New("static: mounted TypeValue source unavailable")
	}
	rows := make([]typeValueRow, 0)
	seen := make(map[identity.ContentID]struct{})
	for _, mount := range a.mounts {
		if mount.Artifact == nil || !mount.Artifact.Available() {
			return errors.New("static: unavailable mounted TypeValue artifact")
		}
		for index := 0; index < mount.Artifact.StaticTypeValueCount(); index++ {
			artifactRow, rowOK := mount.Artifact.StaticTypeValueAt(index)
			if !rowOK || !artifactRow.Available() {
				return errors.New("static: malformed mounted TypeValue row")
			}
			valueID, valueOK := a.valueIDs[mountedValueKey{module: mount.ModuleID, semantic: artifactRow.ID()}]
			if !valueOK || !valueID.Available() {
				return errors.New("static: unavailable mounted TypeValue value receipt")
			}
			if _, duplicate := seen[valueID]; duplicate {
				return errors.New("static: duplicate TypeValue Boundary Value")
			}
			reference, referenceOK := a.types.FindByReferenceID(artifactRow.ReferenceID())
			if !referenceOK {
				return errors.New("static: unavailable mounted TypeValue reference")
			}
			coordinate, coordinateOK := a.coordinateFor(coordinateKey{reference: reference, namespace: mount.NamespaceID})
			if !coordinateOK {
				return errors.New("static: unavailable mounted TypeValue coordinate")
			}
			result, resultOK := a.Result(coordinate)
			if !resultOK {
				return errors.New("static: unavailable mounted TypeValue result")
			}
			row := typeValueRow{valueID: valueID, name: artifactRow.Name(), root: artifactRow.RootID()}
			if !staticPrimitiveTypeValueName(row.name) {
				row.root = mountedTypeValueRootID(mount.ModuleID, row.root)
			}
			inner, exact, dispositionErr := s.typeValueExactInner(result, runtime)
			if dispositionErr != nil {
				return dispositionErr
			}
			row.inner, row.exact = inner, exact
			row.id, rowOK = staticTypeValueRowID(a.linkID, runtime, result, row)
			if !rowOK {
				return errors.New("static: unavailable mounted TypeValue row identity")
			}
			seen[valueID] = struct{}{}
			rows = append(rows, row)
		}
	}
	a.typeValues = rows
	return nil
}

func staticPrimitiveTypeValueName(name string) bool {
	switch name {
	case "nil", "boolean", "number", "integer", "string", "any", "unknown", "never":
		return true
	default:
		return false
	}
}

func mountedTypeValueRootID(module, local identity.ContentID) identity.ContentID {
	if !module.Available() || !local.Available() {
		return identity.ContentID{}
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte("wippy.analysis.static/typevalue-mounted-root/v1"))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write(module[:])
	_, _ = hash.Write(local[:])
	var id identity.ContentID
	copy(id[:], hash.Sum(nil))
	return id
}

// typeValueExactInner classifies an already-grounded Static result. Bottom
// and Invalid are lawful explicit Other outcomes; all structural authority
// still flows exclusively through Static's Class image.
func (s *ClassSet) typeValueExactInner(result Value, runtime *typeauthority.Runtime) (typeauthority.RuntimeInner, bool, error) {
	if s == nil || s.authority == nil || runtime == nil || !s.authority.Owns(result) {
		return typeauthority.RuntimeInner{}, false, errors.New("static: foreign TypeValue result")
	}
	kind, valid := result.Kind()
	if !valid {
		return typeauthority.RuntimeInner{}, false, errors.New("static: malformed TypeValue result")
	}
	if kind == KindBottom || kind == KindInvalid {
		return typeauthority.RuntimeInner{}, false, nil
	}
	class, classified := s.ClassForStatic(result)
	if !classified {
		return typeauthority.RuntimeInner{}, false, errors.New("static: unavailable TypeValue class")
	}
	if class.descriptor != nil || class.index >= uint32(len(s.rows)) || s.rows[class.index].kind != ClassConcrete {
		return typeauthority.RuntimeInner{}, false, nil
	}
	inner := s.rows[class.index].inner
	if !runtime.Equal(inner, inner) {
		return typeauthority.RuntimeInner{}, false, errors.New("static: unavailable TypeValue Runtime inner")
	}
	return inner, true, nil
}
