package flow

import (
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/join"
)

// structuredCarryForward is the transfer stage that preserves root and child
// facts when a structured path write creates a new symbol version.
type structuredCarryForward struct {
	s              *Solution
	point          cfg.Point
	targetPath     constraint.Path
	currentBaseKey constraint.PathKey
	preds          []cfg.Point
	predBaseKeys   []constraint.PathKey
}

func newStructuredCarryForward(s *Solution, p cfg.Point, targetPath constraint.Path) *structuredCarryForward {
	return &structuredCarryForward{s: s, point: p, targetPath: targetPath}
}

func (c *structuredCarryForward) apply() []constraint.PathKey {
	if !c.prepare() {
		return nil
	}
	changed := c.seedRoot()
	c.seedSuffixes()
	return changed
}

func (c *structuredCarryForward) prepare() bool {
	if c.s == nil || c.s.inputs == nil || c.s.inputs.Graph == nil || c.s.pkResolver == nil {
		return false
	}
	if c.targetPath.Symbol == 0 || len(c.targetPath.Segments) == 0 {
		return false
	}
	currentBase := constraint.Path{
		Root:    c.targetPath.Root,
		Symbol:  c.targetPath.Symbol,
		Version: c.targetPath.Version,
	}
	baseKey := c.s.pkResolver.KeyAt(c.point, currentBase)
	if baseKey == "" {
		return false
	}
	c.currentBaseKey = baseKey
	c.preds = graphPredecessors(c.s.inputs.Graph, c.point)
	if len(c.preds) == 0 {
		return false
	}
	c.predBaseKeys = c.collectPredecessorBaseKeys()
	return len(c.predBaseKeys) > 0
}

func (c *structuredCarryForward) collectPredecessorBaseKeys() []constraint.PathKey {
	predBaseKeys := make([]constraint.PathKey, 0, len(c.preds))
	seen := make(map[constraint.PathKey]struct{}, len(c.preds))
	for _, pred := range c.preds {
		key := c.predecessorBaseKey(pred)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		predBaseKeys = append(predBaseKeys, key)
	}
	return predBaseKeys
}

func (c *structuredCarryForward) predecessorBaseKey(pred cfg.Point) constraint.PathKey {
	ver := c.s.inputs.Graph.VisibleVersion(pred, c.targetPath.Symbol)
	if ver.Symbol == 0 || ver.ID == 0 {
		return ""
	}
	return c.s.pkResolver.KeyAtVersion(ver.Symbol, ver.ID, nil)
}

func (c *structuredCarryForward) seedRoot() []constraint.PathKey {
	baseTypes := c.s.predecessorNarrowedRootTypes(c.point, c.targetPath, c.predBaseKeys)
	if _, exists := c.s.values[c.currentBaseKey]; len(baseTypes) == 0 || exists {
		return nil
	}
	joinedBase := join.Types(baseTypes...)
	if sameFlowValue(projectFlowValue(c.s.values[c.currentBaseKey]), joinedBase) {
		return nil
	}
	c.s.setValue(string(c.currentBaseKey), joinedBase)
	return []constraint.PathKey{c.currentBaseKey}
}

func (c *structuredCarryForward) seedSuffixes() {
	suffixTypes := c.collectPredecessorSuffixTypes()
	for suffix, types := range suffixTypes {
		if len(types) == 0 {
			continue
		}
		key := suffix.PathUnder(c.currentBaseKey)
		keyString := string(key)
		if c.s.valueAtPoint(c.point, keyString) != nil {
			continue
		}
		joined := join.Types(types...)
		if !sameFlowValue(c.s.valueAtPoint(c.point, keyString), joined) {
			c.s.setMutableValue(c.point, keyString, joined)
		}
	}
}

func (c *structuredCarryForward) collectPredecessorSuffixTypes() map[pathSuffixKey][]typ.Type {
	out := make(map[pathSuffixKey][]typ.Type)
	for _, pred := range c.preds {
		predBaseKey := c.predecessorBaseKey(pred)
		if predBaseKey == "" {
			continue
		}
		c.collectStableSuffixTypes(out, predBaseKey)
		c.collectMutableSuffixTypes(out, pred, predBaseKey)
	}
	return out
}

func (c *structuredCarryForward) collectStableSuffixTypes(out map[pathSuffixKey][]typ.Type, predBaseKey constraint.PathKey) {
	for _, suffix := range c.s.valueSuffixesForRoot(predBaseKey) {
		if av, ok := c.s.values[suffix.PathUnder(predBaseKey)]; ok {
			if t := projectFlowValue(av); t != nil {
				out[suffix] = append(out[suffix], t)
			}
		}
	}
}

func (c *structuredCarryForward) collectMutableSuffixTypes(out map[pathSuffixKey][]typ.Type, pred cfg.Point, predBaseKey constraint.PathKey) {
	state := c.s.mutableOut[pred]
	for _, suffix := range c.s.mutableSuffixesForRoot(pred, predBaseKey) {
		if av, ok := state[suffix.PathUnder(predBaseKey)]; ok {
			if t := projectFlowValue(av); t != nil {
				out[suffix] = append(out[suffix], t)
			}
		}
	}
}
