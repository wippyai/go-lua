// Package apply invokes authenticated semantic bindings, validates their
// bounded proposals, and hands an exclusive proposal lease to the publication
// door. Each Application also seals the exact mounted lineage used by that
// invocation; publication cannot attach an independent provenance sidecar.
// Apply never owns relation state and never performs publication itself.
// Outcomes remain a separate algebra: a refusal has no proposal cells, while
// an accepted result may carry an empty or publishing batch (including Opaque
// rows).
package apply
