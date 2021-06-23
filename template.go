package main

import (
	"text/template"
)

var packageTemplate = template.Must(template.New("").Parse(`// Code generated by go-localize; DO NOT EDIT.
// This file was generated by robots at
// {{ .Timestamp }}

package {{ .Package }}

import (
	"mimir/corelib/i18n"
)


var l = i18n.New("en", "en", localizations)

func GetWithLocale(locale string, key i18n.Key, replacements ...*i18n.Replacements) string {
	return l.GetWithLocale(locale, key, replacements...)
}

const (
{{- range $key, $element := .Keys }}
	{{ $key }} i18n.Key = "{{ $element }}"
{{- end }}
)

var localizations = map[string]string{
{{- range $key, $element := .Localizations }}
	"{{ $key }}": "{{ $element }}",
{{- end }}
}
`,
))

var packageTemplateUtil = template.Must(template.New("").Parse(`// Code generated by go-localize; DO NOT EDIT.
// This file was generated by robots at
// {{ .Timestamp }}

package {{ .Package }}

import (
	"fmt"
	"mimir/corelib/i18n"
)

func GetListWithLocale(locale string, keys []i18n.Key, fallback bool, replacements ...*i18n.Replacements) []string {
	results := []string{}
	for _, key := range keys {
		_, ok := localizations[fmt.Sprintf("%v.%v", locale, key)]
		if !ok && !fallback {
			continue
		}
		results = append(results, l.GetWithLocale(locale, key, replacements...))
	}

	return results
}

{{- range $key, $list := .ListKeys }}

func List{{ $key }}(locale string, fallback bool, replacements ...*i18n.Replacements) []string {
	keys := []i18n.Key{
		{{- range $i, $k := $list }}
		{{ $k }},
		{{- end }}
	}
	return GetListWithLocale(locale, keys, fallback, replacements...)
}

{{- end }}
`,
))
