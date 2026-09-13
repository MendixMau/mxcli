// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/catalog"
)

// graphProbeCatalog builds a file-backed catalog holding just a snapshot and a
// hand-written refs graph — the state a `refresh catalog full` leaves behind,
// with no graph-analysis pass yet.
//
// The edge set is shaped to populate every graph table at once: two clusters so
// the community split is real (and graph_integration_surface has a cross-
// community edge to classify), and one MUTUAL pair so graph_cycles is non-empty.
// Without that pair the cycles table would stay empty and the table at the heart
// of mendixlabs/mxcli#1060 would be the one the drift guard below fails to cover.
func graphProbeCatalog(t *testing.T) *catalog.Catalog {
	t.Helper()
	cat, err := catalog.NewFromFile(filepath.Join(t.TempDir(), "catalog.db"))
	if err != nil {
		t.Fatalf("NewFromFile: %v", err)
	}
	t.Cleanup(func() { cat.Close() })
	db := cat.CatalogDB()
	if _, err := db.Exec(`INSERT INTO snapshots (SnapshotId, ProjectId) VALUES ('s1','default')`); err != nil {
		t.Fatalf("insert snapshot: %v", err)
	}
	edges := [][2]string{
		// Cluster A, densely connected; A.1 <-> A.2 is the cycle.
		{"A.1", "A.2"}, {"A.2", "A.1"}, {"A.2", "A.3"}, {"A.3", "A.1"},
		// Cluster B, densely connected, no cycle.
		{"B.1", "B.2"}, {"B.2", "B.3"}, {"B.1", "B.3"},
		// The single bridge between them.
		{"A.1", "B.1"},
	}
	for _, e := range edges {
		if _, err := db.Exec(`INSERT INTO refs
			(SourceType, SourceId, SourceName, TargetType, TargetId, TargetName, RefKind, ModuleName, ProjectId, SnapshotId)
			VALUES ('MICROFLOW','',?,'MICROFLOW','',?,'call',?,'default','s1')`,
			e[0], e[1], strings.SplitN(e[0], ".", 2)[0]); err != nil {
			t.Fatalf("insert ref: %v", err)
		}
	}
	return cat
}

func graphProbeRowCounts(t *testing.T, cat *catalog.Catalog) map[string]int {
	t.Helper()
	out := map[string]int{}
	for _, qualified := range cat.Tables() {
		table := strings.TrimPrefix(strings.ToLower(qualified), "catalog.")
		var n int
		if err := cat.CatalogDB().QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		out[table] = n
	}
	return out
}

// TestCommunitiesOnlyTablesMatchWhatTheGraphPassPopulates is the drift guard for
// mendixlabs/mxcli#1060. The bug was a classification hole, not a wrong string:
// GRAPH_CYCLES (and its four siblings) appeared in NO mode list, so a query
// against an un-run graph pass answered "0 rows" — the same output as "no cycles
// found" — while GRAPH_MODULE_COUPLING, which *is* listed, answered properly
// from the same catalog. That asymmetry is what got reported.
//
// So the set is not asserted against a second hand-written list, which would
// drift in lockstep with the first. It is MEASURED: run the pass and see which
// tables went from empty to non-empty. A graph table added later without a
// classification fails here.
func TestCommunitiesOnlyTablesMatchWhatTheGraphPassPopulates(t *testing.T) {
	cat := graphProbeCatalog(t)

	before := graphProbeRowCounts(t, cat)
	// Control: with the pass not yet run, every table this test is about is
	// empty — so "gained rows" below really is the pass's doing.
	for table := range communitiesOnlyTables {
		if before[table] != 0 {
			t.Fatalf("%s already has %d rows before the graph pass", table, before[table])
		}
	}

	if err := cat.AddGraphAnalysis(1.0); err != nil {
		t.Fatalf("AddGraphAnalysis: %v", err)
	}
	after := graphProbeRowCounts(t, cat)

	var gained []string
	for table, n := range after {
		if n > 0 && before[table] == 0 {
			gained = append(gained, table)
		}
	}
	sort.Strings(gained)

	var classified []string
	for table := range communitiesOnlyTables {
		classified = append(classified, table)
	}
	sort.Strings(classified)

	if strings.Join(gained, ",") != strings.Join(classified, ",") {
		t.Errorf("tables populated by the graph pass = %v\ncommunitiesOnlyTables   = %v\n"+
			"every table the pass fills must be classified, or a query against it "+
			"reports 0 rows instead of \"requires refresh catalog communities\"", gained, classified)
	}
}

// TestGraphAnalysisFlagDistinguishesEmptyFromNeverRun pins the distinction the
// whole fix rests on. Before #1060 there was nothing to read here: the pass left
// no trace, so an empty graph_cycles_data was ambiguous and the recorded build
// mode still said "full".
func TestGraphAnalysisFlagDistinguishesEmptyFromNeverRun(t *testing.T) {
	cat := graphProbeCatalog(t)

	info, err := cat.GetCacheInfo()
	if err != nil {
		t.Fatal(err)
	}
	if info.GraphAnalysis {
		t.Fatal("a catalog that has never run the pass must not claim graph analysis")
	}

	if err := cat.AddGraphAnalysis(0.42); err != nil {
		t.Fatalf("AddGraphAnalysis: %v", err)
	}
	info, err = cat.GetCacheInfo()
	if err != nil {
		t.Fatal(err)
	}
	if !info.GraphAnalysis {
		t.Error("graph analysis ran but the catalog does not record it")
	}
	if info.GraphResolution != 0.42 {
		t.Errorf("GraphResolution = %v, want 0.42 — a carried-over re-run must use the same resolution", info.GraphResolution)
	}
}

// TestGraphResolutionRecordsTheEffectiveValue: callers pass 0 to mean "default".
// Recording the 0 would make a carried-over re-run at "the same resolution" a
// different run from the original.
func TestGraphResolutionRecordsTheEffectiveValue(t *testing.T) {
	cat := graphProbeCatalog(t)
	if err := cat.AddGraphAnalysis(0); err != nil {
		t.Fatalf("AddGraphAnalysis: %v", err)
	}
	info, err := cat.GetCacheInfo()
	if err != nil {
		t.Fatal(err)
	}
	if info.GraphResolution != 1.0 {
		t.Errorf("GraphResolution = %v, want the effective 1.0 rather than the caller's 0", info.GraphResolution)
	}
}

func TestTableRequiredModeNamesTheCommandThatPopulates(t *testing.T) {
	for table, want := range map[string]string{
		"graph_cycles": "refresh catalog communities",
		"communities":  "refresh catalog communities",
		// Controls: the neighbours that were already classified must not move.
		"graph_module_coupling": "refresh catalog full",
		"source":                "refresh catalog full source",
		"entities":              "refresh catalog",
	} {
		if got := tableRequiredMode(table); got != want {
			t.Errorf("tableRequiredMode(%q) = %q, want %q", table, got, want)
		}
	}
}

// TestCarryGraphAnalysis covers the second half of #1060: a plain full rebuild
// used to drop the graph tables silently, leaving the mode reading "full" so
// nothing downstream could notice.
func TestCarryGraphAnalysis(t *testing.T) {
	with := &catalog.CacheInfo{GraphAnalysis: true, GraphResolution: 1.25}
	without := &catalog.CacheInfo{}

	cases := []struct {
		name              string
		prior             *catalog.CacheInfo
		full, communities bool
		wantCarry         bool
		wantRes           float64
	}{
		{"full rebuild over a graph cache carries it", with, true, false, true, 1.25},
		// Controls, one per reason not to carry:
		{"no prior cache", nil, true, false, false, 0},
		{"prior cache never ran the pass", without, true, false, false, 0},
		{"a fast build is not promoted", with, false, false, false, 0},
		{"an explicit communities run needs no carry", with, true, true, false, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			carry, res := carryGraphAnalysis(c.prior, c.full, c.communities)
			if carry != c.wantCarry || res != c.wantRes {
				t.Errorf("carryGraphAnalysis = (%v, %v), want (%v, %v)", carry, res, c.wantCarry, c.wantRes)
			}
		})
	}
}
