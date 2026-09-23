// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// Tab pages are addressable by the name DESCRIBE PAGE prints for them
// (`tabpage tabPage2 (Caption: 'Guests')`). Before the fix, every ALTER PAGE
// operation naming one failed with `widget "tabPage2" not found` — the finder
// descended into each tab page's Widgets but never compared the tab page's own
// Name — and the error went on to talk about DataGrid2 columns.

// tabPageDeps serializes a domain widget to a document carrying its real $Type
// and a fresh $ID, which is all the tab-page plumbing inspects.
type tabPageDeps struct {
	Deps
	next byte
}

func (d *tabPageDeps) SerializeWidget(w pages.Widget) bson.D {
	d.next++
	return bson.D{
		{Key: "$ID", Value: makeBsonID(0xA0 + d.next)},
		{Key: "$Type", Value: w.GetTypeName()},
		{Key: "Name", Value: w.GetName()},
	}
}

func makeTabPage(name string, id primitive.Binary, children ...bson.D) bson.D {
	widgets := bson.A{int32(2)}
	for _, c := range children {
		widgets = append(widgets, c)
	}
	return bson.D{
		{Key: "$ID", Value: id},
		{Key: "$Type", Value: "Forms$TabPage"},
		{Key: "Caption", Value: bson.D{
			{Key: "$Type", Value: "Texts$Text"},
			{Key: "Items", Value: bson.A{int32(3), bson.D{
				{Key: "$Type", Value: "Texts$Translation"},
				{Key: "LanguageCode", Value: "en_US"},
				{Key: "Text", Value: name + " caption"},
			}}},
		}},
		{Key: "ConditionalVisibilitySettings", Value: nil},
		{Key: "Name", Value: name},
		{Key: "RefreshOnShow", Value: false},
		{Key: "Widgets", Value: widgets},
	}
}

// tabFixture is a page holding one tab container with tabPage1 (the default),
// tabPage2 (holding snippetCall4) and tabPage3, followed by a plain text box.
func tabFixture() (*Mutator, bson.D) {
	tabs := bson.D{
		{Key: "$ID", Value: makeBsonID(0x10)},
		{Key: "$Type", Value: "Forms$TabControl"},
		{Key: "Appearance", Value: bson.D{{Key: "$Type", Value: "Forms$Appearance"}, {Key: "Class", Value: ""}}},
		{Key: "DefaultPagePointer", Value: makeBsonID(0x11)},
		{Key: "Name", Value: "tabContainer1"},
		{Key: "TabPages", Value: bson.A{int32(3),
			makeTabPage("tabPage1", makeBsonID(0x11)),
			makeTabPage("tabPage2", makeBsonID(0x12), makeWidget("snippetCall4", "Forms$SnippetCallWidget")),
			makeTabPage("tabPage3", makeBsonID(0x13)),
		}},
	}
	raw := makeRawPage(tabs, makeWidget("txtAfter", "Forms$TextBox"))
	return &Mutator{rawData: raw, widgetFinder: findBsonWidget, deps: &tabPageDeps{}}, tabs
}

func tabPageNames(t *testing.T, m *Mutator) []string {
	t.Helper()
	tc := m.widgetFinder(m.rawData, "tabContainer1")
	if tc == nil {
		t.Fatal("tab container not found")
	}
	var names []string
	for _, tp := range bsonnav.DGetArrayElements(bsonnav.DGet(tc.widget, "TabPages")) {
		names = append(names, bsonnav.DGetString(tp.(bson.D), "Name"))
	}
	return names
}

func defaultPageID(t *testing.T, m *Mutator) string {
	t.Helper()
	tc := m.widgetFinder(m.rawData, "tabContainer1")
	return bsonnav.ExtractBinaryIDFromDoc(bsonnav.DGet(tc.widget, "DefaultPagePointer"))
}

func idOf(b byte) string { return bsonnav.ExtractBinaryIDFromDoc(makeBsonID(b)) }

func newTabPage(name string) *pages.TabPage {
	return &pages.TabPage{BaseElement: model.BaseElement{TypeName: "Forms$TabPage"}, Name: name}
}

func TestTabPage_FoundByName(t *testing.T) {
	m, _ := tabFixture()
	r := m.widgetFinder(m.rawData, "tabPage2")
	if r == nil {
		t.Fatal(`tabPage2 not found — tab pages must be addressable by their DESCRIBE name`)
	}
	if got := bsonnav.DGetString(r.widget, "$Type"); got != "Forms$TabPage" {
		t.Errorf("resolved $Type = %q, want Forms$TabPage", got)
	}
	if r.parentKey != "TabPages" {
		t.Errorf("parentKey = %q, want TabPages", r.parentKey)
	}
	// Children of a tab page stay reachable.
	if m.widgetFinder(m.rawData, "snippetCall4") == nil {
		t.Error("snippetCall4 inside tabPage2 is no longer found")
	}
	if !m.FindWidget("tabPage2") {
		t.Error("FindWidget(tabPage2) = false")
	}
	if _, ok := m.WidgetScope()["tabPage2"]; !ok {
		t.Error("WidgetScope lacks tabPage2, so a new widget could reuse its name")
	}
}

func TestTabPage_Drop(t *testing.T) {
	m, _ := tabFixture()
	if err := m.DropWidget([]backend.WidgetRef{{Widget: "tabPage2"}}); err != nil {
		t.Fatalf("drop tabPage2: %v", err)
	}
	if got := strings.Join(tabPageNames(t, m), ","); got != "tabPage1,tabPage3" {
		t.Errorf("tab pages after drop = %s", got)
	}
	if got := defaultPageID(t, m); got != idOf(0x11) {
		t.Errorf("default page moved although it was not dropped: %s", got)
	}
	// The list marker survives.
	tc := m.widgetFinder(m.rawData, "tabContainer1")
	if arr := bsonnav.ToBsonA(bsonnav.DGet(tc.widget, "TabPages")); len(arr) == 0 || arr[0] != int32(3) {
		t.Errorf("TabPages marker lost: %v", arr)
	}
}

// Dropping the default tab page must not leave DefaultPagePointer pointing at
// an element that no longer exists — an unresolvable pointer is a document
// Studio Pro cannot open.
func TestTabPage_DropDefaultRepointsDefault(t *testing.T) {
	m, _ := tabFixture()
	if err := m.DropWidget([]backend.WidgetRef{{Widget: "tabPage1"}}); err != nil {
		t.Fatalf("drop tabPage1: %v", err)
	}
	if got := defaultPageID(t, m); got != idOf(0x12) {
		t.Errorf("DefaultPagePointer = %s, want the first remaining tab page %s", got, idOf(0x12))
	}
}

func TestTabPage_DropLastRefused(t *testing.T) {
	m, _ := tabFixture()
	err := m.DropWidget([]backend.WidgetRef{{Widget: "tabPage1"}, {Widget: "tabPage2"}, {Widget: "tabPage3"}})
	if err == nil || !strings.Contains(err.Error(), "last tab page") {
		t.Fatalf("dropping every tab page: err = %v, want a refusal naming the last tab page", err)
	}
}

func TestTabPage_SetVisibleAndCaption(t *testing.T) {
	m, _ := tabFixture()
	if err := m.SetWidgetProperty("tabPage2", "Visible", "false"); err != nil {
		t.Fatalf("set Visible: %v", err)
	}
	tp := m.widgetFinder(m.rawData, "tabPage2").widget
	cvs := bsonnav.DGetDoc(tp, "ConditionalVisibilitySettings")
	if cvs == nil || bsonnav.DGetString(cvs, "Expression") != "false" {
		t.Errorf("ConditionalVisibilitySettings = %v, want expression false", cvs)
	}
	if err := m.SetWidgetProperty("tabPage2", "Caption", "Visitors"); err != nil {
		t.Fatalf("set Caption: %v", err)
	}
	items := bsonnav.DGetArrayElements(bsonnav.DGet(bsonnav.DGetDoc(tp, "Caption"), "Items"))
	if len(items) == 0 || bsonnav.DGetString(items[0].(bson.D), "Text") != "Visitors" {
		t.Errorf("caption not updated: %v", items)
	}
}

// A Forms$TabPage has no Appearance, so Class/Style have nowhere to go. The
// setter used to return success and write nothing; refuse instead and point at
// the property that does work.
func TestTabPage_SetClassRefused(t *testing.T) {
	m, _ := tabFixture()
	for _, prop := range []string{"Class", "style", "DynamicClasses"} {
		err := m.SetWidgetProperty("tabPage2", prop, "hidden")
		if err == nil || !strings.Contains(err.Error(), "tab page") || !strings.Contains(err.Error(), "Visible") {
			t.Errorf("set %s on a tab page: err = %v, want a refusal pointing at Visible", prop, err)
		}
	}
}

func TestTabPage_InsertBeforeAfter(t *testing.T) {
	m, _ := tabFixture()
	if err := m.InsertWidget("tabPage2", "", "after", []pages.Widget{newTabPage("tabNew")}); err != nil {
		t.Fatalf("insert after tabPage2: %v", err)
	}
	if err := m.InsertWidget("tabPage1", "", "before", []pages.Widget{newTabPage("tabFirst")}); err != nil {
		t.Fatalf("insert before tabPage1: %v", err)
	}
	if got := strings.Join(tabPageNames(t, m), ","); got != "tabFirst,tabPage1,tabPage2,tabNew,tabPage3" {
		t.Errorf("tab pages = %s", got)
	}
}

func TestTabPage_InsertIntoTabPage(t *testing.T) {
	m, _ := tabFixture()
	txt := &pages.TextBox{BaseWidget: pages.BaseWidget{BaseElement: model.BaseElement{TypeName: "Forms$TextBox"}, Name: "txtNew"}}
	if err := m.InsertWidget("tabPage3", "", "into", []pages.Widget{txt}); err != nil {
		t.Fatalf("insert into tabPage3: %v", err)
	}
	r := m.widgetFinder(m.rawData, "txtNew")
	if r == nil {
		t.Fatal("txtNew not found after insert into tabPage3")
	}
	if bsonnav.DGetString(r.parentDoc, "Name") != "tabPage3" {
		t.Errorf("txtNew landed in %q, want tabPage3", bsonnav.DGetString(r.parentDoc, "Name"))
	}
}

func TestTabPage_InsertIntoTabContainer(t *testing.T) {
	m, _ := tabFixture()
	if err := m.InsertWidget("tabContainer1", "", "into", []pages.Widget{newTabPage("tabLast")}); err != nil {
		t.Fatalf("insert tabpage into tabContainer1: %v", err)
	}
	if got := strings.Join(tabPageNames(t, m), ","); got != "tabPage1,tabPage2,tabPage3,tabLast" {
		t.Errorf("tab pages = %s", got)
	}
}

// Only tab pages may sit in a tab container's TabPages list, and a tab page may
// sit nowhere else. Either mix gives a document Studio Pro cannot load.
func TestTabPage_MixingRefused(t *testing.T) {
	m, _ := tabFixture()
	txt := &pages.TextBox{BaseWidget: pages.BaseWidget{BaseElement: model.BaseElement{TypeName: "Forms$TextBox"}, Name: "txtX"}}
	if err := m.InsertWidget("tabPage2", "", "after", []pages.Widget{txt}); err == nil {
		t.Error("inserting a text box beside a tab page was accepted")
	}
	if err := m.InsertWidget("txtAfter", "", "after", []pages.Widget{newTabPage("tabStray")}); err == nil {
		t.Error("inserting a tab page beside an ordinary widget was accepted")
	}
	if err := m.InsertWidget("tabPage2", "", "into", []pages.Widget{newTabPage("tabNested")}); err == nil {
		t.Error("inserting a tab page into a tab page was accepted")
	}
	if err := m.ReplaceWidget("tabPage2", "", []pages.Widget{txt}); err == nil {
		t.Error("replacing a tab page with a text box was accepted")
	}
	if got := strings.Join(tabPageNames(t, m), ","); got != "tabPage1,tabPage2,tabPage3" {
		t.Errorf("a refused op still changed the tab pages: %s", got)
	}
}

func TestTabPage_ReplaceDefaultRepointsDefault(t *testing.T) {
	m, _ := tabFixture()
	if err := m.ReplaceWidget("tabPage1", "", []pages.Widget{newTabPage("tabPage1b")}); err != nil {
		t.Fatalf("replace tabPage1: %v", err)
	}
	if got := strings.Join(tabPageNames(t, m), ","); got != "tabPage1b,tabPage2,tabPage3" {
		t.Errorf("tab pages = %s", got)
	}
	newID := bsonnav.ExtractBinaryIDFromDoc(bsonnav.DGet(m.widgetFinder(m.rawData, "tabPage1b").widget, "$ID"))
	if got := defaultPageID(t, m); got != newID {
		t.Errorf("DefaultPagePointer = %s, want the replacement %s", got, newID)
	}
}

// The not-found message must not blame DataGrid2 addressing for a name that is
// simply absent; the column hint is secondary.
func TestWidgetNotFoundError_LeadsWithTheWidget(t *testing.T) {
	m, _ := tabFixture()
	err := m.DropWidget([]backend.WidgetRef{{Widget: "noSuchWidget"}})
	if err == nil {
		t.Fatal("expected not-found")
	}
	msg := err.Error()
	if !strings.HasPrefix(msg, `widget "noSuchWidget" not found`) {
		t.Errorf("message = %q", msg)
	}
	if strings.Contains(msg, "DataGrid2") {
		t.Errorf("page has no DataGrid2 columns, yet the message mentions them: %q", msg)
	}
}
