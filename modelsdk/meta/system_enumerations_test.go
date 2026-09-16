// SPDX-License-Identifier: Apache-2.0

package meta

import (
	"os"
	"slices"
	"strings"
	"testing"
)

// The System module's enumerations were defined in SystemEnumerations and then
// never exposed: `describe enumeration System.WorkflowActivityType` reported
// "enumeration not found" while `describe entity` happily printed attributes
// typed against it, so the values could only be guessed at until the build
// rejected one with CE1613 (mendixlabs/mxcli#1102).
//
// The entity and Java-action halves of the virtual System module each have a
// Build* helper; this is the enumeration one, plus the resolvability guard that
// the entity half has had all along (TestModelerSystemEntities_
// HaveResolvableGeneralizations) and this half did not.

// TestModelerSystemEntities_HaveResolvableEnumerations is the sibling of
// TestModelerSystemEntities_HaveResolvableGeneralizations: an attribute typed
// against a System enumeration mxcli cannot produce is an attribute whose valid
// values nothing can report. It passes on today's table — the 15 definitions
// cover all 14 enumerations the modeler-view entities reference — and exists so
// that adding an enum-typed System attribute without its enumeration fails here
// rather than at a user's build.
func TestModelerSystemEntities_HaveResolvableEnumerations(t *testing.T) {
	known := make(map[string]bool, len(SystemEnumerations))
	for _, e := range SystemEnumerations {
		known[e.Name] = true
	}
	for _, ent := range ModelerSystemEntities() {
		for _, a := range ent.Attributes {
			if a.Type != "Enumeration" {
				continue
			}
			if a.EnumQN == "" {
				t.Errorf("System.%s.%s is an Enumeration with no EnumQN", ent.Name, a.Name)
				continue
			}
			if !known[a.EnumQN] {
				t.Errorf("System.%s.%s references %s, which is not in SystemEnumerations — "+
					"describe enumeration %s will report it missing",
					ent.Name, a.Name, a.EnumQN, a.EnumQN)
			}
		}
	}
}

// TestBuildSystemEnumerations_CoversTheTable checks the helper exposes every
// definition, with the module stripped off the Name (the model carries the local
// name; the module comes from ContainerID) and every value carried over.
func TestBuildSystemEnumerations_CoversTheTable(t *testing.T) {
	built := BuildSystemEnumerations()
	if len(built) != len(SystemEnumerations) {
		t.Fatalf("BuildSystemEnumerations() = %d enumerations, want %d", len(built), len(SystemEnumerations))
	}
	byName := make(map[string]int, len(built))
	for _, e := range built {
		if string(e.ContainerID) != SystemModuleID {
			t.Errorf("%s ContainerID = %q, want the System module ID", e.Name, e.ContainerID)
		}
		if e.TypeName != "Enumerations$Enumeration" {
			t.Errorf("%s TypeName = %q", e.Name, e.TypeName)
		}
		byName[e.Name] = len(e.Values)
	}
	for _, def := range SystemEnumerations {
		local := def.Name[len("System."):]
		got, ok := byName[local]
		if !ok {
			t.Errorf("%s missing from BuildSystemEnumerations() (looked for local name %q)", def.Name, local)
			continue
		}
		if got != len(def.Values) {
			t.Errorf("%s value count = %d, want %d", def.Name, got, len(def.Values))
		}
	}
}

// TestBuildSystemEnumerations_IDsAreDeterministicAndUnique matters because the
// catalog keys enumerations_data on Id: a colliding or run-varying ID either
// breaks the insert or makes the catalog differ run to run. Mirrors the Java
// action helper's scheme.
func TestBuildSystemEnumerations_IDsAreDeterministicAndUnique(t *testing.T) {
	first, second := BuildSystemEnumerations(), BuildSystemEnumerations()
	seen := map[string]string{}
	for i, e := range first {
		if e.ID != second[i].ID {
			t.Errorf("%s ID varies between calls: %q vs %q", e.Name, e.ID, second[i].ID)
		}
		if e.ID == "" {
			t.Errorf("%s has an empty ID", e.Name)
		}
		if prev, dup := seen[string(e.ID)]; dup {
			t.Errorf("%s and %s share ID %q", prev, e.Name, e.ID)
		}
		seen[string(e.ID)] = e.Name
		vals := map[string]bool{}
		for _, v := range e.Values {
			if v.ID == "" {
				t.Errorf("%s.%s has an empty value ID", e.Name, v.Name)
			}
			if vals[string(v.ID)] {
				t.Errorf("%s has duplicate value ID %q", e.Name, v.ID)
			}
			vals[string(v.ID)] = true
		}
	}
}

// TestSystemEnumerations_NamesAreQualified guards the assumption the builder
// makes when it strips the prefix.
func TestSystemEnumerations_NamesAreQualified(t *testing.T) {
	for _, def := range SystemEnumerations {
		if len(def.Name) <= len("System.") || def.Name[:len("System.")] != "System." {
			t.Errorf("SystemEnumerations entry %q is not a System-qualified name", def.Name)
		}
		if len(def.Values) == 0 {
			t.Errorf("%s has no values", def.Name)
		}
	}
}

// TestSkillDocumentsTheSameValues pins the system-module skill's enumeration
// section to this table.
//
// It exists because the section had DRIFTED into wrong casing — `created`,
// `end`, `single`, `microflow`, `error`, `user`, `external` — and was missing
// three enumerations entirely, WorkflowActivityState among them. Enumeration
// value names are case-sensitive and a wrong one is only caught at build time as
// CE1613, so a developer copying `created` out of the skill hit exactly the
// failure the skill was there to prevent (mendixlabs/mxcli#1102). A hand-kept
// list of platform values is only as good as the thing that compares it.
func TestSkillDocumentsTheSameValues(t *testing.T) {
	const skill = "../../.claude/skills/mendix/system-module/SKILL.md"
	raw, err := os.ReadFile(skill)
	if err != nil {
		t.Skipf("skill not readable (%v) — nothing to compare", err)
	}

	// Section 8 lists one `### <LocalName>` heading per enumeration, followed by
	// its values as `backticked`, comma-separated names.
	body := string(raw)
	start := strings.Index(body, "## 8. Enumerations")
	if start < 0 {
		t.Fatalf("%s no longer has an '## 8. Enumerations' section — update this test with it", skill)
	}
	section := body[start:]
	if end := strings.Index(section, "\n## 9."); end > 0 {
		section = section[:end]
	}

	documented := map[string][]string{}
	var current string
	for _, line := range strings.Split(section, "\n") {
		if after, ok := strings.CutPrefix(line, "### "); ok {
			current = strings.TrimSpace(after)
			continue
		}
		if current == "" || !strings.HasPrefix(strings.TrimSpace(line), "`") {
			continue
		}
		for _, part := range strings.Split(line, ",") {
			if v := strings.Trim(strings.TrimSpace(part), "`"); v != "" {
				documented[current] = append(documented[current], v)
			}
		}
		current = ""
	}

	for _, def := range SystemEnumerations {
		local := strings.TrimPrefix(def.Name, "System.")
		got, ok := documented[local]
		if !ok {
			t.Errorf("%s is not documented in the system-module skill's section 8", def.Name)
			continue
		}
		if !slices.Equal(got, def.Values) {
			t.Errorf("%s: skill documents %v, table has %v", def.Name, got, def.Values)
		}
		delete(documented, local)
	}
	for extra := range documented {
		t.Errorf("the skill documents System.%s, which is not in SystemEnumerations", extra)
	}
}
