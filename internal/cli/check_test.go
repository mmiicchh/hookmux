package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/mmiicchh/hookmux/internal/config"
)

func TestCheckReportsEachKindOfProblem(t *testing.T) {
	cfg := config.Config{Rules: []config.Rule{
		{Name: "ok", When: "cmd.size() > 0", Exec: "echo ok"},
		{Name: "args-only", When: `cmd0 == "a"`, ExecArgs: "cmd"},
		{Name: "dyn-args", When: `cmd0 == "d"`, Exec: "echo", ExecArgs: `[in_json().?cwd.orValue(".")]`}, // list(dyn) is fine
		{Name: "typo", When: "cmd0 +", Exec: "echo x"},
		{Name: "not-bool", When: "cmd0", Exec: "echo x"},
		{Name: "bad-args", Exec: "echo", ExecArgs: "cmd0"},
		{Name: "missing", Exec: "/no/such/binary"}, // last: no `when` is fine here
	}}
	report := check(cfg, "c.yaml", nil)
	if !report.failed() || len(report.Problems) != 0 {
		t.Fatalf("report should fail on rules only: %+v", report)
	}
	notes := map[string]string{}
	for _, r := range report.Rules {
		notes[r.Name] = r.Note
	}
	want := map[string]string{
		"ok":        "",
		"args-only": "",
		"dyn-args":  "",
		"not-bool":  "when: must be bool, got string",
		"bad-args":  "exec_args: must be list(string), got string",
		"missing":   "exec not found: /no/such/binary",
	}
	for name, w := range want {
		if notes[name] != w {
			t.Errorf("%s: got %q want %q", name, notes[name], w)
		}
	}
	if !strings.HasPrefix(notes["typo"], "when: ") {
		t.Errorf("typo: %q", notes["typo"])
	}
}

func TestCheckFlagsAnAlwaysMatchingRuleThatShadowsLaterOnes(t *testing.T) {
	for _, when := range []string{"", "true", `true || cmd0 == "x"`, "1 == 1", "!false"} {
		r := check(config.Config{Rules: []config.Rule{
			{Name: "all", When: when, Exec: "/no/such/bin"},
			{Name: "parked", When: "false", Exec: "echo p"}, // parked rules are not counted
			{Name: "never-reached", Exec: "echo x"},
			{Name: "nor-this", Exec: "echo y"},
		}}, "c.yaml", nil)
		if r.Rules[0].Note != "exec not found: /no/such/bin; always matches; the 2 rule(s) after it can never run" {
			t.Errorf("when=%q: %q", when, r.Rules[0].Note)
		}
		if r.Rules[3].Note != "" {
			t.Errorf("when=%q: a last always-matching rule is fine, got %q", when, r.Rules[3].Note)
		}
	}
	r := check(config.Config{Rules: []config.Rule{
		{Name: "all", Exec: "echo a"},
		{Name: "parked", When: "false", Exec: "echo p"},
	}}, "c.yaml", nil)
	if r.failed() {
		t.Errorf("a catch-all followed only by parked rules is fine: %+v", r.Rules)
	}
}

func TestCheckPassesOnACleanConfigAndReportsLoadErrors(t *testing.T) {
	if r := check(config.Config{Rules: []config.Rule{{Name: "a", Exec: "echo a"}}}, "c.yaml", nil); r.failed() {
		t.Errorf("clean config failed: %+v", r)
	}
	if r := check(config.Config{}, "c.yaml", errors.New("bogus")); !r.failed() || r.ConfigErr != "bogus" {
		t.Errorf("load error not reported: %+v", r)
	}
	if r := check(config.Config{}, "c.yaml", nil); len(r.Problems) != 1 {
		t.Errorf("empty rules should warn: %+v", r)
	}
}
