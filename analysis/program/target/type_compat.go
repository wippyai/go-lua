package target

import (
	"errors"
	"fmt"

	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
)

// Type relations are delegated to the explicit schema semantic authority.
// Target owns the coordinates and the sealed rows; it never interprets a
// neutral declaration or substitutes a presence check for a domain judgment.
func (d *operationDraft) typeAssignable(sourceKey, destinationKey string) (bool, error) {
	source, destination, err := d.typePair(sourceKey, destinationKey)
	if err != nil {
		return false, err
	}
	return d.semantics.Assignable(source, destination, d.formalConstraints)
}

func (d *operationDraft) typeAccepts(source schematype.Type, destinationKey string) (bool, error) {
	destination, ok := d.declarations[destinationKey]
	if !ok || destinationKey == "" {
		return false, errors.New("type declaration is not admitted")
	}
	return d.semantics.Assignable(source, destination, d.formalConstraints)
}

func (d *operationDraft) typePair(sourceKey, destinationKey string) (schematype.Type, schematype.Type, error) {
	source, sourceOK := d.declarations[sourceKey]
	destination, destinationOK := d.declarations[destinationKey]
	if sourceKey == "" || destinationKey == "" || !sourceOK || !destinationOK {
		return schematype.Type{}, schematype.Type{}, errors.New("type declaration is not admitted")
	}
	return source, destination, nil
}

func (d *operationDraft) hasType(key string) bool {
	_, ok := d.declarations[key]
	return ok && key != ""
}

func (d *operationDraft) admitsAdmission(key string, admission Admission) (bool, error) {
	value, ok := d.declarations[key]
	if key == "" || !ok {
		return false, errors.New("type declaration is not admitted")
	}
	var requested schematype.CallableAdmission
	switch admission {
	case DirectFunction:
		requested = schematype.CallableAdmissionDirectFunction
	case OrdinaryCallable:
		requested = schematype.CallableAdmissionOrdinary
	default:
		return false, fmt.Errorf("invalid callable admission %d", admission)
	}
	return d.semantics.Callable(value, requested, d.formalConstraints)
}

func (d *operationDraft) freshCompatible(key string, fresh FreshKind) (bool, error) {
	value, ok := d.declarations[key]
	if key == "" || !ok {
		return false, errors.New("type declaration is not admitted")
	}
	var requested schematype.FreshClass
	switch fresh {
	case FreshTable:
		requested = schematype.FreshClassTable
	case FreshFunction:
		requested = schematype.FreshClassFunction
	case FreshThread:
		requested = schematype.FreshClassThread
	case FreshUserdata:
		requested = schematype.FreshClassUserdata
	case FreshError:
		requested = schematype.FreshClassError
	case FreshReflection:
		requested = schematype.FreshClassReflection
	default:
		return false, fmt.Errorf("invalid fresh kind %d", fresh)
	}
	return d.semantics.Fresh(value, requested, d.formalConstraints)
}
