package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadRejectsInvalidRulesAndUnknownKeys(t *testing.T) {
	cases := map[string]string{
		"rules: [{exec: echo}]":                              "name is required",
		"rules: [{name: a}]":                                 "exec or exec_args is required",
		"rules: [{name: a, exec: x}, {name: a, exec: y}]":    "duplicate name",
		"rules: [{name: a, exec: x, bogus: 1}]":              "bogus",
		"exec_timeout: 30":                                   "time.Duration",
		"exec_timeout: 30us":                                 "exec_timeout",
		"rules: [{name: a, exec: x, sample_rate: 1.5}]":      "sample_rate",
		"rules: [{name: a, exec: x, sample_rate: .nan}]":     "sample_rate",
		"rules: [{name: a, exec: x, sample_rate: 0}]":        "",
		"exec_timeout: -1s":                                  "exec_timeout",
		"exec_timeout: 5s\nrules: [{name: a, exec: x}]":      "",
		"view_port: 1\nrules: [{name: a, exec_args: cmd}]\n": "",
	}
	for src, want := range cases {
		p := filepath.Join(t.TempDir(), "c.yaml")
		os.WriteFile(p, []byte(src), 0o644)
		_, err := Load(p)
		switch {
		case want == "" && err != nil:
			t.Errorf("%q: unexpected error %v", src, err)
		case want != "" && (err == nil || !strings.Contains(err.Error(), want)):
			t.Errorf("%q: got %v, want %q", src, err, want)
		}
	}
}

func TestEmptyFileYieldsDefaultsButAnExplicitMissingPathIsAnError(t *testing.T) {
	p := filepath.Join(t.TempDir(), "empty.yaml")
	os.WriteFile(p, nil, 0o644)
	c, err := Load(p)
	if err != nil || c.ViewPort != 4665 || c.HistMaxKB != 16 || strings.HasPrefix(c.HistDir, "~") {
		t.Errorf("defaults: %+v %v", c, err)
	}
	if _, err := Load(filepath.Join(t.TempDir(), "typo.yaml")); err == nil {
		t.Error("a missing non-default config must be an error")
	}
}
