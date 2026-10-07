// Package config loads the hookmux YAML config; a missing file yields defaults.
package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

type Rule struct {
	Name       string   `yaml:"name"`
	When       string   `yaml:"when"`        // CEL, bool; empty = always
	Exec       string   `yaml:"exec"`        // command line, split like a shell would
	ExecArgs   string   `yaml:"exec_args"`   // CEL, list of strings appended to exec
	SampleRate *float64 `yaml:"sample_rate"` // share of successful runs recorded; nil = 1
}

// Sample is the recording rate for successful runs; failures are always recorded.
func (r Rule) Sample() float64 {
	if r.SampleRate == nil {
		return 1
	}
	return *r.SampleRate
}

type Config struct {
	Rules        []Rule        `yaml:"rules"`
	ViewPort     int           `yaml:"view_port"`
	HistDir      string        `yaml:"hist_dir"`
	HistKeepDays int           `yaml:"hist_keep_days"`
	HistMaxKB    int           `yaml:"hist_max_kb"`
	ExecTimeout  time.Duration `yaml:"exec_timeout"`
}

// DefaultPath is where Load looks when no path is given; only this one may be absent.
func DefaultPath() string { return "~/.config/hookmux/config.yaml" }

// Load reads path. The default path may be missing (defaults, no rules); any
// other missing path is an error, since it was asked for explicitly.
func Load(path string) (Config, error) {
	c := Config{
		ViewPort:     4665, // HOOK on a phone keypad
		HistDir:      "~/.local/state/hookmux/hist",
		HistKeepDays: 14,
		HistMaxKB:    16,
	}
	err := load(path, &c)
	c.HistDir = ExpandHome(c.HistDir)
	return c, err
}

func load(path string, c *Config) error {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) && path == ExpandHome(DefaultPath()) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)
	if err := dec.Decode(c); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if t := c.ExecTimeout; t < 0 || (t > 0 && t < time.Millisecond) {
		return fmt.Errorf("exec_timeout: %v is not a usable duration; write it like 5s", t)
	}
	seen := map[string]bool{}
	for i, r := range c.Rules {
		switch {
		case r.Name == "":
			return fmt.Errorf("rule #%d: name is required", i+1)
		case r.Exec == "" && r.ExecArgs == "":
			return fmt.Errorf("rule %q: exec or exec_args is required", r.Name)
		case seen[r.Name]:
			return fmt.Errorf("rule %q: duplicate name", r.Name)
		case !(r.Sample() >= 0 && r.Sample() <= 1): // also rejects NaN
			return fmt.Errorf("rule %q: sample_rate must be between 0 and 1", r.Name)
		}
		seen[r.Name] = true
	}
	return nil
}

func ExpandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		if h, err := os.UserHomeDir(); err == nil {
			return filepath.Join(h, p[2:])
		}
	}
	return p
}
