// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"strings"
	"testing"
)

// mendixlabs/mxcli#1135, route 1.
//
// `alter page … set 'Remove empty text' = off on lvThings` on a native List View
// dead-ends. The write is genuinely not supported on this path — but the message
// it dead-ends with describes mxcli's internals rather than the author's
// options:
//
//	property "Remove empty text" not found (widget has no pluggable Object)
//
// "pluggable Object" is not a thing the author wrote, cannot be made true by
// editing the script, and — most of the point — is not the whole truth: the
// design property IS writable, through ALTER STYLING. Measured on a blank
// 11.12.2 project, the statement the old message did not mention:
//
//	alter styling on page MyFirstModule.ThingList widget lvThings
//	  set 'Remove empty text' = on;
//	-> Updated styling on widget "lvThings" in page MyFirstModule.ThingList
//	describe styling -> DesignProperties: ['Remove empty text': on]
//
// So the error names that route. A widget carrying a Forms$Appearance and no
// pluggable Object is exactly a built-in one, which is when the advice applies.
func TestSetWidgetProperty_NativeWidgetErrorNamesTheStylingRoute(t *testing.T) {
	rawData := makeRawPage(makeStyleableWidget("lvThings"))
	m := &Mutator{rawData: rawData, widgetFinder: findBsonWidget}

	err := m.SetWidgetProperty("lvThings", "Remove empty text", false)
	if err == nil {
		t.Fatal("setting an unknown property on a built-in widget must still fail")
	}
	msg := err.Error()
	if strings.Contains(msg, "pluggable Object") {
		t.Errorf("the message still describes mxcli's internals: %q", msg)
	}
	if !strings.Contains(msg, "alter styling") {
		t.Errorf("the message does not name the route that works: %q", msg)
	}
	if !strings.Contains(msg, "Remove empty text") {
		t.Errorf("the message does not name the property: %q", msg)
	}
}

// The control: a widget that IS pluggable must keep the error that names its own
// vocabulary. Redirecting a mistyped pluggable key to ALTER STYLING would send
// the author to a command that cannot write it either.
func TestSetWidgetProperty_PluggableWidgetKeepsItsOwnError(t *testing.T) {
	rawData := makeRawPage(makePluggableWidget("dg1", "pageSize", "10"))
	m := &Mutator{rawData: rawData, widgetFinder: findBsonWidget}

	err := m.SetWidgetProperty("dg1", "NoSuchProperty", 1)
	if err == nil {
		t.Fatal("an unknown pluggable property must fail")
	}
	if strings.Contains(err.Error(), "alter styling") {
		t.Errorf("a pluggable property was redirected to ALTER STYLING: %q", err.Error())
	}
}
