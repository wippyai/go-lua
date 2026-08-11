package workbench

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"sort"

	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/cutplan"
	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/render"
	"github.com/wippyai/go-lua/cmd/flashrefactor/internal/semantic"
)

func deriveRoutes(intent cutplan.Intent, witnesses []render.RouteWitness, source, target semantic.Snapshot) ([]cutplan.ReferenceRoute, error) {
	required, err := cutplan.ReferenceRouteRequirements(intent)
	if err != nil {
		return nil, err
	}
	if source.Workspace == nil || target.Workspace == nil {
		return nil, fmt.Errorf("route construction requires source and target semantic workspaces")
	}
	witnessByRoute := map[string]render.RouteWitness{}
	for _, witness := range witnesses {
		key := routeKey(witness.From, witness.To)
		if _, exists := witnessByRoute[key]; exists {
			return nil, fmt.Errorf("renderer emitted duplicate route witness: %s", key)
		}
		witnessByRoute[key] = witness
	}
	seen := map[string]bool{}
	routes := make([]cutplan.ReferenceRoute, 0, len(required))
	for _, requirement := range required {
		key := routeKey(requirement.From, requirement.To)
		witness, exists := witnessByRoute[key]
		if !exists {
			return nil, fmt.Errorf("renderer did not provide relocation witness: %s", key)
		}
		sites, routeErr := witnessSites(witness, source, target)
		if routeErr != nil {
			return nil, fmt.Errorf("route %s: %w", key, routeErr)
		}
		routes = append(routes, cutplan.ReferenceRoute{From: requirement.From, To: requirement.To, Sites: sites})
		seen[key] = true
	}
	for key := range witnessByRoute {
		if !seen[key] {
			return nil, fmt.Errorf("renderer emitted undeclared relocation witness: %s", key)
		}
	}
	sort.Slice(routes, func(i, j int) bool {
		return routeKey(routes[i].From, routes[i].To) < routeKey(routes[j].From, routes[j].To)
	})
	return routes, nil
}

func witnessSites(witness render.RouteWitness, source, target semantic.Snapshot) ([]cutplan.ReferenceSiteRoute, error) {
	before, err := evidenceFor(source.Objects, witness.From, cutplan.ObjectSource)
	if err != nil {
		return nil, err
	}
	after, err := evidenceFor(target.Objects, witness.To, cutplan.ObjectTarget)
	if err != nil {
		return nil, err
	}
	result := make([]cutplan.ReferenceSiteRoute, 0, len(witness.Sites))
	seenSource, seenTarget := map[string]bool{}, map[string]bool{}
	for _, site := range witness.Sites {
		role, roleErr := siteRole(site.Source.Role)
		if roleErr != nil {
			return nil, roleErr
		}
		from, fromErr := semanticSite(endpointSites(before), site.Source.Path, site.Source.Offset, role)
		if fromErr != nil {
			return nil, fmt.Errorf("source witness %s:%d: %w", site.Source.Path, site.Source.Offset, fromErr)
		}
		offset, targetErr := anchorOffset(target, site.Target)
		if targetErr != nil {
			return nil, targetErr
		}
		to, toErr := semanticSite(endpointSites(after), site.Target.Path, offset, role)
		if toErr != nil {
			return nil, fmt.Errorf("target witness %s#%d: %w", site.Target.Path, site.Target.Identifier, toErr)
		}
		if from.Role == cutplan.SiteDeclaration && to.Role != cutplan.SiteDeclaration {
			return nil, fmt.Errorf("declaration witness routes to non-declaration")
		}
		if from.Role != cutplan.SiteDeclaration && to.Role == cutplan.SiteDeclaration {
			return nil, fmt.Errorf("use witness routes to declaration")
		}
		fromKey, toKey := semanticPositionKey(from), semanticPositionKey(to)
		if seenSource[fromKey] || seenTarget[toKey] {
			return nil, fmt.Errorf("witness site is not one-to-one")
		}
		seenSource[fromKey], seenTarget[toKey] = true, true
		result = append(result, cutplan.ReferenceSiteRoute{Source: from, Target: to})
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("route witness has no sites")
	}
	if err := completeSites(endpointSites(before), seenSource, "source"); err != nil {
		return nil, err
	}
	if err := completeSites(endpointSites(after), seenTarget, "target"); err != nil {
		return nil, err
	}
	sort.Slice(result, func(i, j int) bool {
		return semanticPositionKey(result[i].Source) < semanticPositionKey(result[j].Source)
	})
	return result, nil
}

func evidenceFor(values []cutplan.ObjectEvidence, object cutplan.SymbolRef, role cutplan.ObjectRole) (cutplan.ObjectEvidence, error) {
	var found *cutplan.ObjectEvidence
	for index := range values {
		value := &values[index]
		if value.Object != object || value.Role != role {
			continue
		}
		if found != nil {
			return cutplan.ObjectEvidence{}, fmt.Errorf("semantic evidence is ambiguous for %s", object.Object)
		}
		found = value
	}
	if found == nil {
		return cutplan.ObjectEvidence{}, fmt.Errorf("semantic evidence is missing for %s", object.Object)
	}
	return *found, nil
}

func siteRole(role cutplan.SiteRole) (cutplan.SiteRole, error) {
	switch role {
	case cutplan.SiteDeclaration, cutplan.SiteUse, cutplan.SiteSelector, cutplan.SiteImport:
		return role, nil
	default:
		return "", fmt.Errorf("unsupported renderer witness role %q", role)
	}
}

func semanticSite(values []cutplan.Position, path string, offset int, role cutplan.SiteRole) (cutplan.Position, error) {
	var found *cutplan.Position
	for index := range values {
		value := &values[index]
		if value.Path != path || value.Offset != offset || value.Role != role {
			continue
		}
		if found != nil {
			continue
		} // one physical site may have many package variants
		found = value
	}
	if found == nil {
		return cutplan.Position{}, fmt.Errorf("no matching typed semantic site")
	}
	return *found, nil
}

// endpointSites is the actual route denominator. Definition is deliberately
// included independently of References: whether a resolver also repeats that
// point in its use list is an implementation detail, never a route law.
func endpointSites(evidence cutplan.ObjectEvidence) []cutplan.Position {
	values := append([]cutplan.Position{evidence.Definition}, evidence.References...)
	seen := map[string]bool{}
	result := make([]cutplan.Position, 0, len(values))
	for _, value := range values {
		key := physicalPositionKey(value)
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return physicalPositionKey(result[i]) < physicalPositionKey(result[j]) })
	return result
}

func anchorOffset(snapshot semantic.Snapshot, anchor render.StructuralAnchor) (int, error) {
	var source []byte
	for _, file := range snapshot.Workspace.Files() {
		if file.Path != anchor.Path {
			continue
		}
		if source == nil {
			source = append([]byte(nil), file.Source...)
			continue
		}
		if string(source) != string(file.Source) {
			return 0, fmt.Errorf("anchor file is ambiguous across package variants: %s", anchor.Path)
		}
	}
	if source == nil {
		return 0, fmt.Errorf("anchor file is absent from target workspace: %s", anchor.Path)
	}
	set := token.NewFileSet()
	file, err := parser.ParseFile(set, anchor.Path, source, parser.ParseComments)
	if err != nil {
		return 0, fmt.Errorf("parse target anchor file: %w", err)
	}
	index, offset := 0, -1
	ast.Inspect(file, func(node ast.Node) bool {
		ident, isIdentifier := node.(*ast.Ident)
		if !isIdentifier {
			return true
		}
		index++
		if index != anchor.Identifier {
			return true
		}
		if ident.Name != anchor.Name {
			return false
		}
		tokenFile := set.File(ident.Pos())
		if tokenFile != nil {
			offset = tokenFile.Offset(ident.Pos())
		}
		return false
	})
	if offset < 0 {
		return 0, fmt.Errorf("target anchor cannot be resolved: %s#%d", anchor.Path, anchor.Identifier)
	}
	return offset, nil
}

func completeSites(values []cutplan.Position, seen map[string]bool, side string) error {
	for _, value := range values {
		if !seen[semanticPositionKey(value)] {
			return fmt.Errorf("%s semantic site lacks witness coverage: %s", side, semanticPositionKey(value))
		}
	}
	return nil
}

func semanticPositionKey(value cutplan.Position) string {
	return physicalPositionKey(value)
}

func physicalPositionKey(value cutplan.Position) string {
	return fmt.Sprintf("%s:%d:%s", value.Path, value.Offset, value.Role)
}

func routeKey(from, to cutplan.SymbolRef) string { return from.Object + "\x00" + to.Object }
