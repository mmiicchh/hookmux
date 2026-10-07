package cli

import (
	"fmt"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/mmiicchh/hookmux/internal/rules"
)

const templateText = `
{{- define "hist_list" -}}
{{range $i, $r := . -}}
{{printf "%3d" (inc $i)}}  {{local $r.TS "01-02 15:04:05"}}  {{printf "%-14s" (trunc $r.Matched 14)}}  exit {{$r.Exit}}{{if $r.Error}} ⚠{{else}}  {{end}}  {{printf "%4d" $r.DurationMS}}ms  {{join $r.Cmd " "}}
{{end}}
{{- end}}

{{- define "hist_detail" -}}
id        {{.ID}}
time      {{local .TS "2006-01-02 15:04:05 MST"}}
pid       {{.PID}}
hookmux   {{.Version}}
argv      {{printf "%q" .Argv}}
cmd       {{printf "%q" .Cmd}}
cwd       {{.Cwd}}
config    {{.Config}}
{{- with .ConfigErr}}
configerr {{.}}
{{- end}}
{{- with .Ancestors}}
ancestors {{join . " ← "}}
{{- end}}
rules
{{- range .Rules}}
  {{printf "%-20s" .Name}} {{state .}}
{{- else}}
  (none)
{{- end}}
matched   {{.Matched}}
exec      {{printf "%q" .Exec}}
exit      {{.Exit}}  ({{.DurationMS}}ms)
{{- with .Error}}
error     {{.}}
{{- end}}
--- stdin{{shown .Stdin .StdinLen}}
{{.Stdin}}
--- stdout{{shown .Stdout .StdoutLen}}
{{.Stdout}}
--- stderr{{shown .Stderr .StderrLen}}
{{.Stderr}}
{{end}}

{{- define "check" -}}
config: {{.ConfigPath}}
{{- with .ConfigErr}}
  ✗ {{.}}
{{- else}}
{{- range .Rules}}
  {{if .Note}}✗{{else}}✓{{end}} {{printf "%-20s" .Name}} {{or .Note "ok"}}
{{- end}}
{{- range .Problems}}
  ✗ {{.}}
{{- end}}
{{- end}}
{{end}}`

var funcs = template.FuncMap{
	"local": func(t time.Time, layout string) string { return t.Local().Format(layout) },
	"join":  strings.Join,
	"inc":   func(i int) int { return i + 1 },
	"trunc": func(s string, n int) string {
		if len(s) > n {
			return s[:n-1] + "…"
		}
		return s
	},
	"state": func(r rules.Result) string {
		switch {
		case r.Err != "":
			return "error: " + r.Err
		case r.Match:
			return "MATCH"
		}
		return "false"
	},
	"shown": func(body string, full int) string {
		if len(body) < full {
			return fmt.Sprintf(" (showing %d of %d bytes)", len(body), full)
		}
		return ""
	},
}

// templates parses on first use so `run` never pays for it.
var templates = sync.OnceValue(func() *template.Template {
	return template.Must(template.New("").Funcs(funcs).Parse(templateText))
})
