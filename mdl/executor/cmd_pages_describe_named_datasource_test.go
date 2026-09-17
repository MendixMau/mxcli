// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// customWidgetBSON builds the CustomWidget shape the describer reads: a Type
// declaring PropertyTypes (which is where a property key comes from) and an
// Object whose Properties point back at them by TypePointer.
func customWidgetBSON(props []struct {
	key string
	ds  map[string]any
}) map[string]any {
	var propTypes, objProps []any
	for i, p := range props {
		id := "pt-" + p.key + "-" + string(rune('a'+i))
		propTypes = append(propTypes, map[string]any{"$ID": id, "PropertyKey": p.key})
		objProps = append(objProps, map[string]any{
			"TypePointer": id,
			"Value":       map[string]any{"DataSource": p.ds},
		})
	}
	return map[string]any{
		"Type":   map[string]any{"ObjectType": map[string]any{"PropertyTypes": propTypes}},
		"Object": map[string]any{"Properties": objProps},
	}
}

func dbSource(entity string) map[string]any {
	return dsBSON(dsTypeDatabase, "EntityRef", map[string]any{"Entity": entity})
}

// A widget with two configured datasources is read as two, each keyed by the
// schema property it was stored under.
func TestNamedCustomWidgetDataSources_KeepsEachKey(t *testing.T) {
	w := customWidgetBSON([]struct {
		key string
		ds  map[string]any
	}{
		{"linkedDs", dbSource("Sales.Order")},
		{"refOptions", dbSource("Sales.Customer")},
	})

	got := namedCustomWidgetDataSources(w)
	if len(got) != 2 {
		t.Fatalf("read %d datasources, want 2: %+v", len(got), got)
	}
	want := map[string]string{"linkedDs": "Sales.Order", "refOptions": "Sales.Customer"}
	for _, src := range got {
		if want[src.Key] != src.DataSource.Reference {
			t.Errorf("%s -> %q, want %q", src.Key, src.DataSource.Reference, want[src.Key])
		}
	}
}

// An unset datasource property parses to no reference and drops out, so a widget
// with one configured source is still read as single-source and keeps the
// generic `DataSource:` clause it has always been described with. This is what
// makes the change invisible for every widget shipped today.
func TestNamedCustomWidgetDataSources_UnsetSourceDropsOut(t *testing.T) {
	w := customWidgetBSON([]struct {
		key string
		ds  map[string]any
	}{
		{"linkedDs", dsBSON(dsTypeDatabase)}, // declared, never configured
		{"refOptions", dbSource("Sales.Customer")},
	})

	got := namedCustomWidgetDataSources(w)
	if len(got) != 1 || got[0].Key != "refOptions" {
		t.Fatalf("read %+v, want only the configured refOptions", got)
	}
}

// The emitter prefers the named set and never emits both, so a multi-source
// widget's bindings each come back on the mapping they came from.
func TestAppendWidgetDataSources_PrefersNamed(t *testing.T) {
	w := rawWidget{
		// An extractor may have left this set; several emitter branches are gated
		// on it being non-nil, so it stays — but it must not be emitted as well.
		DataSource: &rawDataSource{Type: "database", Reference: "Sales.Customer"},
		NamedDataSources: []rawNamedDataSource{
			{Key: "linkedDs", DataSource: &rawDataSource{Type: "database", Reference: "Sales.Order"}},
			{Key: "refOptions", DataSource: &rawDataSource{Type: "database", Reference: "Sales.Customer"}},
		},
	}

	got := strings.Join(appendWidgetDataSources(nil, w), "\n")
	for _, want := range []string{"linkedDs: database from Sales.Order", "refOptions: database from Sales.Customer"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "DataSource:") {
		t.Errorf("the generic clause must not be emitted alongside the named ones:\n%s", got)
	}
}

// The control for the test above, and the defect it replaces: emitting
// w.DataSource is what the describer did, and it says one binding where the
// widget has two — so a rewrite fanned one source across both mappings or
// dropped the second.
func TestAppendWidgetDataSources_GenericClauseLosesTheSecondSource(t *testing.T) {
	w := rawWidget{
		DataSource: &rawDataSource{Type: "database", Reference: "Sales.Customer"},
		NamedDataSources: []rawNamedDataSource{
			{Key: "linkedDs", DataSource: &rawDataSource{Type: "database", Reference: "Sales.Order"}},
			{Key: "refOptions", DataSource: &rawDataSource{Type: "database", Reference: "Sales.Customer"}},
		},
	}
	got := strings.Join(appendDataSourceProp(nil, w.DataSource), "\n")
	if strings.Contains(got, "Sales.Order") {
		t.Fatalf("control did not reproduce the old behaviour — the generic clause "+
			"should carry only one entity, got:\n%s", got)
	}
}

// A single-datasource widget is emitted exactly as before: one generic clause.
func TestAppendWidgetDataSources_SingleSourceUnchanged(t *testing.T) {
	w := rawWidget{DataSource: &rawDataSource{Type: "database", Reference: "Sales.Customer"}}
	got := strings.Join(appendWidgetDataSources(nil, w), "\n")
	if got != "DataSource: database from Sales.Customer" {
		t.Errorf("single-source output changed: %q", got)
	}
}

// A datasource whose schema key did not resolve falls back to the unnamed
// spelling. Skipping it would lose the binding, which is #956's silent drop
// reached by a new route — and the key is exactly what cannot be relied on,
// since it comes from the widget's own PropertyTypes.
func TestAppendWidgetDataSources_UnresolvedKeyFallsBackNotDropped(t *testing.T) {
	w := rawWidget{
		NamedDataSources: []rawNamedDataSource{
			{Key: "", DataSource: &rawDataSource{Type: "database", Reference: "Sales.Order"}},
			{Key: "refOptions", DataSource: &rawDataSource{Type: "database", Reference: "Sales.Customer"}},
		},
	}
	got := strings.Join(appendWidgetDataSources(nil, w), "\n")
	if !strings.Contains(got, "DataSource: database from Sales.Order") {
		t.Errorf("an unresolved key must fall back to the unnamed spelling, got:\n%s", got)
	}
	if !strings.Contains(got, "refOptions: database from Sales.Customer") {
		t.Errorf("the resolved key must still be named, got:\n%s", got)
	}
}
