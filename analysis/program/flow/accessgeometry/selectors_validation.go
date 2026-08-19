package accessgeometry

import (
	"errors"

	staticquery "github.com/wippyai/go-lua/analysis/program/static/query"

	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	binding "github.com/wippyai/go-lua/analysis/program/flow/binding"
	"github.com/wippyai/go-lua/analysis/program/flow/body"
	"github.com/wippyai/go-lua/analysis/program/imports"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

func validateSelectorInputs(sourceView sourceColumns, flow authored.View, bodies *body.Result, bindings binding.Result, staticView staticquery.View, moduleView imports.View) error {
	identity := sourceView.Identity()
	if !identity.ContentID().Available() || identity.TermCount() == 0 {
		return errors.New("program/flow/accessgeometry: Source sourceView is unavailable")
	}
	if !flow.Cold().ContentID().Available() {
		return errors.New("program/flow/accessgeometry: authored Flow is unavailable")
	}
	if !body.Matches(bodies, identity.ContentID(), flow.Cold().ContentID()) {
		return errors.New("program/flow/accessgeometry: Body provenance disagrees with Source or Flow")
	}
	if !binding.Matches(&bindings, identity.ContentID(), flow.Cold().ContentID()) {
		return errors.New("program/flow/accessgeometry: Bindings provenance disagrees with Source or Flow")
	}
	if !staticView.ContentID().Available() {
		return errors.New("program/flow/accessgeometry: Static view is unavailable")
	}
	if !moduleView.ContentID().Available() {
		return errors.New("program/flow/accessgeometry: Module view is unavailable")
	}
	bodyCount := identity.FamilyCount(keyspace.FamilyBody)
	if bodyCount <= 0 || !keyspace.TermOrdinalFits(bodyCount) {
		return errors.New("program/flow/accessgeometry: Source Body denominator is unavailable")
	}
	if err := validateFlowDenominators(identity, flow); err != nil {
		return err
	}
	if bindings.CellCount() != flow.Storage().Cells().Count() {
		return errors.New("program/flow/accessgeometry: Bindings Cell denominator mismatch")
	}
	if staticView.Publications().Count() != identity.FamilyCount(keyspace.FamilyTypePublication) {
		return errors.New("program/flow/accessgeometry: Static publication denominator mismatch")
	}
	if moduleView.Count() != identity.FamilyCount(keyspace.FamilyImport) {
		return errors.New("program/flow/accessgeometry: Module Import denominator mismatch")
	}
	return nil
}

// Access geometry consumes only these authored families. Other Flow owner tables
// remain the responsibility of their authored-owner finalizer; rechecking them
// here would duplicate authority. Source family counts are still the one bound
// for each relation retained by this package.
func validateFlowDenominators(identity source.Identity, flow authored.View) error {
	checks := [...]struct {
		family keyspace.Family
		count  int
	}{
		{keyspace.FamilyValues, flow.Values().Count()},
		{keyspace.FamilyLensExact, flow.Access().Exact().Count()},
		{keyspace.FamilyLensKey, flow.Access().Dynamic().Count()},
		{keyspace.FamilyCell, flow.Storage().Cells().Count()},
		{keyspace.FamilyRead, flow.Storage().Reads().Count()},
		{keyspace.FamilyBind, flow.Storage().Binds().Count()},
		{keyspace.FamilyAssign, flow.Storage().Assigns().Count()},
		{keyspace.FamilyWrite, flow.Storage().Writes().Count()},
		{keyspace.FamilyCall, flow.Calls().Count()},
		{keyspace.FamilyUnary, flow.Operators().Unaries().Count()},
	}
	for _, check := range checks {
		if identity.FamilyCount(check.family) != check.count {
			return errors.New("program/flow/accessgeometry: authored family denominator mismatch")
		}
	}
	return nil
}

func validFlowBody(term keyspace.Term, bodyCount int) bool {
	return keyspace.TermFamily(term) == keyspace.FamilyBody && keyspace.TermOrdinal(term) != 0 &&
		uint64(keyspace.TermOrdinal(term)) <= uint64(bodyCount)
}
