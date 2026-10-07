package cli

import (
	"fmt"
	"os/exec"
	"slices"
	"strings"

	"github.com/mmiicchh/hookmux/internal/config"
	"github.com/mmiicchh/hookmux/internal/rules"
)

type CheckCmd struct{}

type checkReport struct {
	ConfigPath string
	ConfigErr  string
	Rules      []checkRow
	Problems   []string
}

type checkRow struct {
	Name string
	Note string // "" when the rule is fine
}

func (r checkReport) failed() bool {
	return r.ConfigErr != "" || len(r.Problems) > 0 ||
		slices.ContainsFunc(r.Rules, func(row checkRow) bool { return row.Note != "" })
}

func (c *CheckCmd) Run(app *App) error {
	report := check(app.Config, app.ConfigPath, app.ConfigErr)
	if err := templates().ExecuteTemplate(app.Out, "check", report); err != nil {
		return err
	}
	if report.failed() {
		return ExitCode(1)
	}
	return nil
}

func check(cfg config.Config, path string, loadErr error) checkReport {
	report := checkReport{ConfigPath: path}
	if loadErr != nil {
		report.ConfigErr = loadErr.Error()
		return report
	}
	if len(cfg.Rules) == 0 {
		report.Problems = append(report.Problems, "no rules: every run fails")
	}
	set, err := rules.Compile(cfg.Rules)
	if err != nil {
		report.Problems = append(report.Problems, "CEL environment: "+err.Error())
		return report
	}
	compiled := set.Rules()
	for i, r := range compiled {
		var notes []string
		if r.Err != nil {
			notes = append(notes, r.Err.Error())
		} else {
			if bin := r.Exec0(); bin != "" {
				if _, err := exec.LookPath(bin); err != nil {
					notes = append(notes, "exec not found: "+bin)
				}
			}
			if n := reachableAfter(compiled[i+1:]); r.Always() && n > 0 {
				notes = append(notes, fmt.Sprintf("always matches; the %d rule(s) after it can never run", n))
			}
		}
		report.Rules = append(report.Rules, checkRow{Name: r.Name, Note: strings.Join(notes, "; ")})
	}
	return report
}

// reachableAfter counts rules that are not parked (`when: "false"`).
func reachableAfter(rs []rules.Compiled) int {
	n := 0
	for i := range rs {
		if !rs[i].Never() {
			n++
		}
	}
	return n
}
