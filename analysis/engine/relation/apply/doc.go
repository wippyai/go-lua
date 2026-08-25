// Package apply invokes authenticated semantic bindings, validates their
// bounded proposals, and hands an exclusive proposal lease to the publication
// door. It never owns relation state and never performs publication itself.
// Outcomes remain a separate algebra: a refusal has no proposal cells, while
// an accepted non-refusal result may carry an empty or produced batch.
package apply
