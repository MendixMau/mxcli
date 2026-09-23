// SPDX-License-Identifier: Apache-2.0

package types

import "strings"

// ObjectListKeyword is the MDL keyword (upper case) for the entries of a
// pluggable widget's object-list property: `dropdownItems` → DROPDOWNITEM,
// `columns` → COLUMN. DESCRIBE names each entry `<keyword><N>` (lower case,
// 1-based), and ALTER PAGE resolves those positional names the same way, so
// the derivation lives here, once, for both the executor and the page mutator.
func ObjectListKeyword(propertyKey string) string {
	switch propertyKey {
	case "basicItems":
		return "ITEM"
	case "customItems":
		return "CUSTOMITEM"
	case "dynamicMarkers":
		return "DYNAMICMARKER"
	case "attributesList":
		return "ATTR"
	case "filterOptions":
		return "OPTION"
	case "series":
		return "SERIES" // Latin singular == plural
	}
	return strings.ToUpper(strings.TrimSuffix(strings.ToLower(propertyKey), "s"))
}
