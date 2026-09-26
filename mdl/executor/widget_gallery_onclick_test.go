// SPDX-License-Identifier: Apache-2.0

// A Gallery's own click action. Gallery 3.x declares `onClick` (type action,
// dataSource="datasource" — the clicked item is the context object), the same
// slot a Data grid 2 row click lives in. The Gallery definition is hand-written
// and never regenerated per project, and it mapped only `onClickTrigger`: so
// `gallery g (…, OnClick: microflow M.Open(Item: $currentObject))` passed check
// with no diagnostic (OnClick is a built-in property name, so MDL-WIDGET01 never
// looks at it), exec said "Created page", and the stored widget carried
// Forms$NoAction. A project worked around it with a clickable container inside
// the template.
package executor

import "testing"

func TestGalleryDefinition_MapsOnClickAction(t *testing.T) {
	reg, err := NewWidgetRegistry()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	def, ok := reg.Get("GALLERY")
	if !ok {
		t.Fatal("no GALLERY definition")
	}
	for _, m := range def.PropertyMappings {
		if m.PropertyKey == "onClick" {
			if m.Operation != "action" || m.Source != "OnClick" {
				t.Fatalf("onClick mapping = %+v, want operation action from source OnClick", m)
			}
			return
		}
	}
	t.Fatal("the GALLERY definition has no onClick mapping — `OnClick:` on a gallery is accepted and written as Forms$NoAction")
}
