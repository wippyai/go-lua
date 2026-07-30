package axis

import "fmt"

// RetentionMode is the mandatory artifact-retention contract of one sparse
// axis. Registration rejects Unspecified, so a newly added axis cannot silently
// cross immutable artifact boundaries.
type RetentionMode uint8

const (
	RetentionUnspecified RetentionMode = iota
	RetentionImmutable
	RetentionValidated
)

// RetentionPolicy is owned by the axis Spec, never by individual values.
// Immutable declares that every value of T is safe to retain. Validated is for
// carriers whose safety depends on the concrete value and requires Validate.
type RetentionPolicy[T any] struct {
	Mode     RetentionMode
	Validate func(T) bool
}

func ImmutableRetention[T any]() RetentionPolicy[T] {
	return RetentionPolicy[T]{Mode: RetentionImmutable}
}

func ValidatedRetention[T any](validate func(T) bool) RetentionPolicy[T] {
	return RetentionPolicy[T]{Mode: RetentionValidated, Validate: validate}
}

func validateRetentionPolicy[T any](axisID string, policy RetentionPolicy[T]) error {
	switch policy.Mode {
	case RetentionImmutable:
		if policy.Validate != nil {
			return fmt.Errorf("axis %q: immutable retention policy forbids a validator", axisID)
		}
	case RetentionValidated:
		if policy.Validate == nil {
			return fmt.Errorf("axis %q: validated retention policy requires a validator", axisID)
		}
	default:
		return fmt.Errorf("axis %q: artifact retention policy is unspecified", axisID)
	}
	return nil
}
