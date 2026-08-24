// Package render turns an inventoried declaration surface and a legacy
// protocol census into the stable plain-text cutover manifest: the same
// five sections FT-25's hand-written Store manifest (journal seq 6299)
// established as the shape a cutover lands against.
package render

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/wippyai/go-lua/internal/cutovermanifest/inventory"
	"github.com/wippyai/go-lua/internal/cutovermanifest/residue"
)

// Manifest is the raw material one domain package's manifest is rendered
// from. Build gathers it; Render formats it. Splitting the two keeps
// formatting testable without a repository checkout.
type Manifest struct {
	DomainPkg   string
	RepoRoot    string
	Package     inventory.Package
	Mismatches  []inventory.Mismatch
	Hits        []residue.Hit
	LegacyFiles []residue.File
}

// Build walks domainPkg's declaration surface and legacy-protocol residue
// under repoRoot. It is read-only: no file under repoRoot is written.
func Build(repoRoot, domainPkg string) (Manifest, error) {
	pkg, err := inventory.Load(repoRoot, domainPkg)
	if err != nil {
		return Manifest{}, fmt.Errorf("inventory %s: %w", domainPkg, err)
	}
	mismatches, err := inventory.Verify(pkg, repoRoot)
	if err != nil {
		return Manifest{}, fmt.Errorf("verify %s: %w", domainPkg, err)
	}
	dir := filepath.Join(repoRoot, filepath.FromSlash(domainPkg))
	hits, err := residue.Census(dir)
	if err != nil {
		return Manifest{}, fmt.Errorf("residue census %s: %w", domainPkg, err)
	}
	legacyFiles, err := residue.LegacyFiles(dir)
	if err != nil {
		return Manifest{}, fmt.Errorf("residue legacy files %s: %w", domainPkg, err)
	}
	return Manifest{
		DomainPkg:   domainPkg,
		RepoRoot:    repoRoot,
		Package:     pkg,
		Mismatches:  mismatches,
		Hits:        hits,
		LegacyFiles: legacyFiles,
	}, nil
}

// RenderPackage is Build followed by Render, the entry point cmd/cutover-manifest calls.
func RenderPackage(repoRoot, domainPkg string) (string, error) {
	m, err := Build(repoRoot, domainPkg)
	if err != nil {
		return "", err
	}
	return Render(m), nil
}

// Render formats m as the stable plain-text cutover manifest.
func Render(m Manifest) string {
	var b strings.Builder
	fmt.Fprintf(&b, "CUTOVER MANIFEST: %s\n", m.DomainPkg)
	fmt.Fprintf(&b, "memberdefinition: %s (present=%v)\n", m.Package.MemberDefinitionRelDir, m.Package.Present)
	if len(m.Package.LoadErrors) > 0 {
		fmt.Fprintln(&b, "load errors:")
		for _, e := range m.Package.LoadErrors {
			fmt.Fprintf(&b, "  %s\n", e)
		}
	}
	b.WriteString("\n")

	renderSection1(&b, m)
	renderSection2(&b, m)
	renderSection3(&b, m)
	renderSection4(&b, m)
	renderSection5(&b, m)
	return b.String()
}

func renderSection1(b *strings.Builder, m Manifest) {
	b.WriteString("(1) FAMILY-OWNED CUT SUMMARY\n")
	if !m.Package.Present {
		fmt.Fprintf(b, "  no memberdefinition child package at %s - nothing declared to summarize\n\n", m.Package.MemberDefinitionRelDir)
		return
	}
	if len(m.Package.Declarations) == 0 {
		b.WriteString("  memberdefinition package present, but no Contribution() declaration was found\n\n")
		return
	}
	for i, decl := range m.Package.Declarations {
		if len(m.Package.Declarations) > 1 {
			fmt.Fprintf(b, "  declaration %d of %d\n", i+1, len(m.Package.Declarations))
		}
		fmt.Fprintf(b, "  axis=%s rule=%s (%s)\n", fieldOr(decl.Axis, "<unresolved>"), fieldOr(decl.Rule, "<unresolved>"), decl.FuncPos)

		if len(decl.Relations) == 0 {
			b.WriteString("  candidate source: none declared (reducer folds only joined inputs)\n")
		} else {
			b.WriteString("  candidate source (relations):\n")
			for _, r := range decl.Relations {
				fmt.Fprintf(b, "    %s (key %s): subject=%s candidate=%s\n",
					r.FieldValue("Name"), r.FieldValue("Key"), r.FieldValue("Subject"), relationRefText(r, "CandidateProvider"))
			}
		}

		if len(decl.Projections) > 0 {
			b.WriteString("  projections:\n")
			for _, p := range decl.Projections {
				fmt.Fprintf(b, "    %s (key %s): relation=%s role=%s result=%s accessor=%s\n",
					p.FieldValue("Name"), p.FieldValue("Key"), p.FieldValue("Relation"), p.FieldValue("Role"), p.FieldValue("Result"), symbolText(p, "Accessor"))
			}
		}

		for _, reducer := range decl.Reducers {
			fmt.Fprintf(b, "  fold %s (key %s):\n", reducer.FieldValue("Name"), reducer.FieldValue("Key"))
			candidate := reducer.FieldValue("Candidate")
			if candidate == "" {
				candidate = "(none - no candidate argument)"
			}
			fmt.Fprintf(b, "    candidate: %s\n", candidate)
			b.WriteString("    joins (declaration order):\n")
			for idx, in := range reducerInputRows(reducer) {
				fmt.Fprintf(b, "      [%d] axis=%s carrier=%s form=%s multiplicity=%s%s\n",
					idx, entryReferenceText(in, "Axis"), in.FieldValue("Carrier"), in.FieldValue("Form"), in.FieldValue("Multiplicity"), tagSuffix(in))
			}
			b.WriteString("    outputs:\n")
			for _, out := range reducerOutputRows(reducer) {
				fmt.Fprintf(b, "      axis=%s carrier=%s\n", entryReferenceText(out, "Axis"), out.FieldValue("Carrier"))
			}
			fmt.Fprintf(b, "    implementation: %s\n", symbolText(reducer, "Implementation"))
		}
		b.WriteString("\n")
	}
}

func renderSection2(b *strings.Builder, m Manifest) {
	b.WriteString("(2) CANONICAL KEYS INVENTORY\n")
	type row struct{ kind, name, key string }
	var rows []row
	for _, decl := range m.Package.Declarations {
		for _, c := range decl.Carriers {
			rows = append(rows, row{"carrier", c.FieldValue("Name"), c.FieldValue("Key")})
		}
		for _, r := range decl.Relations {
			rows = append(rows, row{"relation", r.FieldValue("Name"), r.FieldValue("Key")})
		}
		for _, p := range decl.Projections {
			rows = append(rows, row{"projection", p.FieldValue("Name"), p.FieldValue("Key")})
		}
		for _, rd := range decl.Reducers {
			rows = append(rows, row{"reducer", rd.FieldValue("Name"), rd.FieldValue("Key")})
		}
	}
	if len(rows) == 0 {
		b.WriteString("  none declared\n\n")
		return
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].kind != rows[j].kind {
			return rows[i].kind < rows[j].kind
		}
		return rows[i].name < rows[j].name
	})
	for _, r := range rows {
		fmt.Fprintf(b, "  %-10s %-40s %s\n", r.kind, r.name, r.key)
	}
	b.WriteString("\n")
}

func renderSection3(b *strings.Builder, m Manifest) {
	b.WriteString("(3) LEGACY FILES TO REMOVE\n")
	if len(m.LegacyFiles) == 0 {
		b.WriteString("  none - no file under this package is structurally pure protocol residue\n\n")
		return
	}
	for _, f := range m.LegacyFiles {
		rel := relTo(m.RepoRoot, f.Path)
		suffix := ""
		if f.NoExported {
			suffix = " (no exported declarations)"
		}
		fmt.Fprintf(b, "  %s%s\n", rel, suffix)
		for _, h := range f.Hits {
			fmt.Fprintf(b, "    %s:%d: %s\n", rel, h.Line, h.Text)
		}
	}
	b.WriteString("\n")
}

func renderSection4(b *strings.Builder, m Manifest) {
	b.WriteString("(4) VISIBLE MISMATCHES\n")
	if len(m.Mismatches) == 0 {
		b.WriteString("  none found\n\n")
		return
	}
	for _, mm := range m.Mismatches {
		status := "requires solve"
		if mm.Confirmed {
			status = "confirmed"
		}
		fmt.Fprintf(b, "  [%s] %s %s.%s: %s\n", status, mm.Pos, mm.Row, mm.Field, mm.Detail)
	}
	b.WriteString("\n")
}

func renderSection5(b *strings.Builder, m Manifest) {
	b.WriteString("(5) REQUIRED LAWS CHECKLIST\n")
	if len(m.Package.Declarations) == 0 {
		b.WriteString("  no declaration to check laws against\n")
		return
	}
	for _, decl := range m.Package.Declarations {
		axis := fieldOr(decl.Axis, "<axis>")
		rule := fieldOr(decl.Rule, "<rule>")
		fmt.Fprintf(b, "  axis=%s rule=%s\n", axis, rule)
		fmt.Fprintf(b, "  1. member-definition key law: every relation/projection/reducer/carrier key %s declares is unique within axis %s and traces to exactly this rule's contribution.\n", rule, axis)
		fmt.Fprintf(b, "  2. once-per-invocation law: %s's fold reads each declared input exactly once per candidate invocation; no relation or projection is probed a second time to recover a value already read.\n", rule)
		fmt.Fprintf(b, "  3. seal law: the generated Program.Check for axis %s accepts only the shape this contribution declares (candidate, inputs in declaration order, one fold, declared outputs) - no undeclared carry, no extra join.\n", axis)
		fmt.Fprintf(b, "  4. single-slot/single-family law: axis %s has exactly one generated RuleEntry slot for %s and exactly one family claimant; no duplicate or legacy registration remains wired.\n", axis, rule)
		fmt.Fprintf(b, "  5. refusal cases: empty/bottom/foreign/zero-tag evidence at any declared input refuses rather than silently defaulting; %s settles NoSelection or a refusal outcome, never a synthesized value.\n", rule)
		b.WriteString("  (requires solve - run solvedump <fixture> rows to confirm these laws hold at the sealed composition, not just at the declaration)\n")
	}
}

func fieldOr(f inventory.Field, fallback string) string {
	if f.Value == "" {
		return fallback
	}
	return f.Value
}

func relationRefText(rec inventory.Record, field string) string {
	ref, ok := rec.Field(field)
	if !ok {
		return "(none)"
	}
	if ref.Nested == nil {
		return ref.Value
	}
	axis := entryReferenceText(*ref.Nested, "Axis")
	member := ref.Nested.FieldValue("Member")
	if axis == "(none)" && member == "" {
		return ref.Value
	}
	return fmt.Sprintf("axis=%s member=%s", axis, member)
}

func symbolText(rec inventory.Record, field string) string {
	sym, ok := rec.Field(field)
	if !ok {
		return "(none)"
	}
	if sym.Nested == nil {
		return sym.Value
	}
	pkgPath := sym.Nested.FieldValue("PackagePath")
	name := sym.Nested.FieldValue("Name")
	receiver := ""
	if r, ok := sym.Nested.Field("Receiver"); ok && r.Nested != nil {
		receiver = r.Nested.FieldValue("Name")
	}
	if receiver != "" {
		return fmt.Sprintf("%s.%s.%s", pkgPath, receiver, name)
	}
	return fmt.Sprintf("%s.%s", pkgPath, name)
}

// entryReferenceText reads field off rec as a schema.EntryReference{Surface,
// Key} record and returns its Key.
func entryReferenceText(rec inventory.Record, field string) string {
	nested, ok := rec.Field(field)
	if !ok {
		return "(none)"
	}
	if nested.Nested == nil {
		return nested.Value
	}
	if key, ok := nested.Nested.Field("Key"); ok {
		return key.Value
	}
	return nested.Value
}

func reducerInputRows(reducer inventory.Record) []inventory.Record {
	inputs, ok := reducer.Field("Inputs")
	if !ok || inputs.Nested == nil {
		return nil
	}
	return positionalRows(*inputs.Nested)
}

func reducerOutputRows(reducer inventory.Record) []inventory.Record {
	outputs, ok := reducer.Field("Outputs")
	if !ok || outputs.Nested == nil {
		return nil
	}
	return positionalRows(*outputs.Nested)
}

func positionalRows(list inventory.Record) []inventory.Record {
	var rows []inventory.Record
	for _, f := range list.Fields {
		if f.Nested != nil {
			rows = append(rows, *f.Nested)
		}
	}
	return rows
}

func tagSuffix(rec inventory.Record) string {
	tag := rec.FieldValue("Tag")
	if tag == "" {
		return ""
	}
	return fmt.Sprintf(" tag=%s", tag)
}

func relTo(repoRoot, path string) string {
	if repoRoot == "" {
		return path
	}
	if rel, err := filepath.Rel(repoRoot, path); err == nil {
		return filepath.ToSlash(rel)
	}
	return path
}
