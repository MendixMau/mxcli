// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"path/filepath"
	"strings"
	"sync"

	mwidgets "github.com/mendixlabs/mxcli/modelsdk/widgets"
	"github.com/mendixlabs/mxcli/modelsdk/widgets/mpk"
)

// explicitPassWritableTypes are the property value types the widget engine's
// explicit-property pass (PluggableWidgetEngine.Build, step 4.6) writes by
// storage key: Expression, TextTemplate and Attribute through their own
// setters, and the scalar types as a primitive. A value of any other type
// (objects, widgets, icons, images, actions, datasources…) has no correct
// route there, so MDL-WIDGET06 stays true for it.
var explicitPassWritableTypes = map[string]bool{
	"expression": true, "texttemplate": true, "attribute": true,
	"string": true, "boolean": true, "integer": true, "decimal": true, "enumeration": true,
}

var widgetPropertyTypesCache sync.Map // widgetID → map[lowercased key]lowercased type

// widgetPropertyTypes returns each property key's declared value type, from
// the widget's embedded template. Empty when there is no template.
func widgetPropertyTypes(widgetID string) map[string]string {
	if v, ok := widgetPropertyTypesCache.Load(widgetID); ok {
		return v.(map[string]string)
	}
	out := map[string]string{}
	if tmpl, err := mwidgets.GetTemplate(widgetID); err == nil && tmpl != nil {
		for _, p := range propsFromTemplate(tmpl.Type) {
			if p.Key != "" && p.Type != "" {
				out[strings.ToLower(p.Key)] = strings.ToLower(p.Type)
			}
		}
	}
	widgetPropertyTypesCache.Store(widgetID, out)
	return out
}

// persistedByExplicitPass reports whether a key MDL-WIDGET06 would call "not
// yet persisted" is in fact written by the explicit-property pass.
//
// It was not: `optionsSourceAssociationCaptionType: expression` and
// `optionsSourceAssociationCaptionExpression: '…'` on a ComboBox are both
// written, and the page builds clean (measured on Mendix 11.13.0, #664) — yet
// check warned that each "will be dropped". DESCRIBE now emits exactly those
// keys for an expression caption, so the false warning would land on every
// round trip of Administration.Account_New.
//
// An unknown type keeps the warning: saying "not persisted" for something that
// is, is the lesser error than the reverse.
//
// The type comes from the embedded template first and, failing that, from the
// project's INSTALLED package — the one the writer builds from. A property newer
// than the embedded template (File Uploader 2.5.0's `maxFilesPerUpload`, an
// expression) is written by the explicit pass, and was still warned "will be
// dropped" because only the embedded template was consulted.
func persistedByExplicitPass(widgetID, lowerKey, projectPath string) bool {
	t := widgetPropertyTypes(widgetID)[lowerKey]
	if t == "" {
		t = installedWidgetPropertyTypes(projectPath, widgetID)[lowerKey]
	}
	return explicitPassWritableTypes[t]
}

var installedPropertyTypesCache sync.Map // projectPath\x00widgetID → map[lowercased key]lowercased type

// installedWidgetPropertyTypes returns each top-level property's declared type
// from the widget's .mpk in the project's widgets/ folder. Empty without a
// project or a package.
func installedWidgetPropertyTypes(projectPath, widgetID string) map[string]string {
	if projectPath == "" || widgetID == "" {
		return nil
	}
	cacheKey := projectPath + "\x00" + widgetID
	if v, ok := installedPropertyTypesCache.Load(cacheKey); ok {
		return v.(map[string]string)
	}
	out := map[string]string{}
	projectDir := projectPath
	if strings.EqualFold(filepath.Ext(projectDir), ".mpr") {
		projectDir = filepath.Dir(projectDir)
	}
	if mpkPath, err := mpk.FindMPK(projectDir, widgetID); err == nil && mpkPath != "" {
		if wd, err := mpk.ParseMPKForWidget(mpkPath, widgetID); err == nil && wd != nil {
			for _, p := range wd.Properties {
				if p.Key != "" && p.Type != "" {
					out[strings.ToLower(p.Key)] = strings.ToLower(p.Type)
				}
			}
		}
	}
	installedPropertyTypesCache.Store(cacheKey, out)
	return out
}
