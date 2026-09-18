// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// ako/mxcli#512. A List View's "Search attributes" — the attributes its search
// bar filters on — had no MDL spelling, so `mxcli check` reported
// `SearchAttributes` as an unrecognised property that would be silently dropped
// (MDL-WIDGET07) and the search bar could only be set in Studio Pro.
//
// Stored as Forms$ListViewSearch.SearchRefs, a list of DomainModels$AttributeRef
// — the same element a Forms$GridSortItem carries, pinned against a Studio
// Pro-authored sort bar in a blank 11.12.2 project.
//
// The syntax mirrors `sort by`: same position on the database source, same comma
// list, no direction (a search attribute is only a name). Reusing the shape is
// the point — one example teaches both.
func TestSearchBy_ParsesOntoTheDatabaseSource(t *testing.T) {
	prog, errs := visitor.Build(`create page W.P ( Title: 'P' )
{
  LISTVIEW lv (DataSource: DATABASE W.Product sort by Name asc search by Name, Code) {
    DYNAMICTEXT t (Content: 'x')
  }
}`)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	ds := firstListViewDataSource(t, prog)
	if got := strings.Join(ds.SearchAttributes, ","); got != "Name,Code" {
		t.Errorf("SearchAttributes = %q, want \"Name,Code\"", got)
	}
	// The sort clause must survive beside it — the two share a position in the
	// grammar, so getting the clause order wrong drops one of them silently.
	if len(ds.OrderBy) != 1 || ds.OrderBy[0].Attribute != "Name" {
		t.Errorf("sort by was lost: %+v", ds.OrderBy)
	}
}

// Controls. Each covers a way the clause could be wrong without failing above.
func TestSearchBy_Optional(t *testing.T) {
	prog, errs := visitor.Build(`create page W.P ( Title: 'P' )
{
  LISTVIEW lv (DataSource: DATABASE W.Product) { DYNAMICTEXT t (Content: 'x') }
}`)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	if got := firstListViewDataSource(t, prog).SearchAttributes; len(got) != 0 {
		t.Errorf("a source with no search clause grew one: %v", got)
	}
}

// `search` is a non-reserved keyword (the full-text `search 'x'` statement uses
// it), so SEARCH_BY must not swallow it elsewhere. This is the statement that
// would break first.
func TestSearchBy_DoesNotBreakTheSearchStatement(t *testing.T) {
	if _, errs := visitor.Build(`search 'customer';`); len(errs) > 0 {
		t.Errorf("the full-text search statement stopped parsing: %v", errs)
	}
}

// Without the sort clause, so the two are independently optional rather than
// only ever tested together.
func TestSearchBy_WithoutSort(t *testing.T) {
	prog, errs := visitor.Build(`create page W.P ( Title: 'P' )
{
  LISTVIEW lv (DataSource: DATABASE W.Product search by Name) { DYNAMICTEXT t (Content: 'x') }
}`)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	ds := firstListViewDataSource(t, prog)
	if len(ds.SearchAttributes) != 1 || ds.SearchAttributes[0] != "Name" {
		t.Errorf("SearchAttributes = %v, want [Name]", ds.SearchAttributes)
	}
	if len(ds.OrderBy) != 0 {
		t.Errorf("a sort clause appeared from nowhere: %+v", ds.OrderBy)
	}
}

// firstListViewDataSource digs the list view's datasource out of a parsed page.
func firstListViewDataSource(t *testing.T, prog *ast.Program) *ast.DataSourceV3 {
	t.Helper()
	page, ok := prog.Statements[0].(*ast.CreatePageStmtV3)
	if !ok {
		t.Fatalf("statement 0 is %T, want *ast.CreatePageStmtV3", prog.Statements[0])
	}
	for _, w := range page.Widgets {
		if strings.EqualFold(w.Type, "listview") {
			if ds := w.GetDataSource(); ds != nil {
				return ds
			}
		}
	}
	t.Fatal("no list view datasource in the parsed page")
	return nil
}
