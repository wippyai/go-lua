package directbinding

import (
	"errors"

	"github.com/wippyai/go-lua/program/flow/internal/authored"
	flowbinding "github.com/wippyai/go-lua/program/flow/internal/binding"
	"github.com/wippyai/go-lua/program/flow/internal/body"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/module"
	"github.com/wippyai/go-lua/program/source"
	"github.com/wippyai/go-lua/program/static"
)

func validateInputs(preimage source.Preimage, flow authored.View, bodies *body.Result, bindings flowbinding.Result, staticView static.View, moduleView module.View) error {
	identity := preimage.Identity()
	if !identity.ContentID().Available() || identity.TermCount() == 0 {
		return errors.New("program/flow/directbinding: Source preimage is unavailable")
	}
	if !flow.Cold().ContentID().Available() {
		return errors.New("program/flow/directbinding: authored Flow is unavailable")
	}
	if !body.Matches(bodies, identity.ContentID(), flow.Cold().ContentID()) {
		return errors.New("program/flow/directbinding: Body provenance disagrees with Source or Flow")
	}
	if !flowbinding.Matches(&bindings, identity.ContentID(), flow.Cold().ContentID()) {
		return errors.New("program/flow/directbinding: Binding provenance disagrees with Source or Flow")
	}
	if !staticView.ContentID().Available() {
		return errors.New("program/flow/directbinding: Static view is unavailable")
	}
	if !moduleView.ContentID().Available() {
		return errors.New("program/flow/directbinding: Module view is unavailable")
	}
	bodyCount := identity.FamilyCount(keyspace.FamilyBody)
	if bodyCount <= 0 || !keyspace.TermOrdinalFits(bodyCount) {
		return errors.New("program/flow/directbinding: Source Body denominator is unavailable")
	}
	if err := validateFlowDenominators(identity, flow); err != nil {
		return err
	}
	if bindings.CellCount() != flow.Storage().Cells().Count() {
		return errors.New("program/flow/directbinding: Binding Cell denominator mismatch")
	}
	if staticView.Publications().Count() != identity.FamilyCount(keyspace.FamilyTypePublication) {
		return errors.New("program/flow/directbinding: Static publication denominator mismatch")
	}
	if moduleView.Count() != identity.FamilyCount(keyspace.FamilyImport) {
		return errors.New("program/flow/directbinding: Module Import denominator mismatch")
	}
	return nil
}

// DirectBinding consumes only these authored families. Other Flow owner tables
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
			return errors.New("program/flow/directbinding: authored family denominator mismatch")
		}
	}
	return nil
}

func validFlowBody(term keyspace.Term, bodyCount int) bool {
	return keyspace.TermFamily(term) == keyspace.FamilyBody && keyspace.TermOrdinal(term) != 0 &&
		uint64(keyspace.TermOrdinal(term)) <= uint64(bodyCount)
}
