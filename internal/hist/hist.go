// Package hist records invocations as JSONL, one file per day.
package hist

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mmiicchh/hookmux/internal/rules"
)

const DefaultLimit = 50

type Record struct {
	ID         string         `json:"id"`
	TS         time.Time      `json:"ts"`
	PID        int            `json:"pid"`
	Version    string         `json:"hookmux_version"`
	Argv       []string       `json:"argv"`
	Cmd        []string       `json:"cmd"`
	Cwd        string         `json:"cwd"`
	Ancestors  []string       `json:"ancestors,omitempty"`
	Config     string         `json:"config"`
	ConfigErr  string         `json:"config_err,omitempty"`
	Rules      []rules.Result `json:"rules"`
	Matched    string         `json:"matched"` // rule name, or "" when none matched
	Exec       []string       `json:"exec,omitempty"`
	Stdin      string         `json:"stdin"`
	Stdout     string         `json:"stdout"`
	Stderr     string         `json:"stderr"`
	StdinLen   int            `json:"stdin_len"`
	StdoutLen  int            `json:"stdout_len"`
	StderrLen  int            `json:"stderr_len"`
	Exit       int            `json:"exit"`
	Error      string         `json:"error,omitempty"` // hookmux's own failure
	DurationMS int64          `json:"duration_ms"`
}

func NewID(ts time.Time, pid int) string {
	return ts.UTC().Format("20060102T150405.000000Z") + "-" + strconv.Itoa(pid)
}

// Store is the history directory and its retention policy.
type Store struct {
	Dir      string
	MaxKB    int // per stream: stdin, stdout and stderr are each cut to this
	KeepDays int
}

// CaptureLimit is how much of each stream is worth keeping while the target runs.
func (s Store) CaptureLimit() int { return s.MaxKB * 1024 }

// Append writes one record; the first record of a new day also prunes old files.
func (s Store) Append(r *Record) error {
	r.StdinLen, r.StdoutLen, r.StderrLen = len(r.Stdin), len(r.Stdout), len(r.Stderr)
	for _, f := range []*string{&r.Stdin, &r.Stdout, &r.Stderr} {
		*f = (*f)[:min(len(*f), s.CaptureLimit())]
	}
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return err
	}
	line, err := json.Marshal(r)
	if err != nil {
		return err
	}
	path := filepath.Join(s.Dir, r.TS.UTC().Format(time.DateOnly)+".jsonl")
	_, statErr := os.Stat(path)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		return err
	}
	if errors.Is(statErr, os.ErrNotExist) {
		s.prune()
	}
	return nil
}

// List returns up to limit records, newest first, decoding only the lines it keeps.
func (s Store) List(limit int) ([]Record, error) {
	files := s.dayFiles()
	slices.Reverse(files)
	var out []Record
	for _, p := range files {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		lines := bytes.Split(bytes.TrimRight(data, "\n"), []byte{'\n'})
		for i := len(lines) - 1; i >= 0 && len(out) < limit; i-- {
			var r Record
			if json.Unmarshal(lines[i], &r) == nil {
				out = append(out, r)
			}
		}
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

// Clear deletes every day file.
func (s Store) Clear() error {
	for _, p := range s.dayFiles() {
		if err := os.Remove(p); err != nil {
			return err
		}
	}
	return nil
}

func (s Store) prune() {
	if s.KeepDays <= 0 {
		return
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -s.KeepDays).Format(time.DateOnly)
	for _, p := range s.dayFiles() {
		if strings.TrimSuffix(filepath.Base(p), ".jsonl") < cutoff {
			os.Remove(p)
		}
	}
}

// dayFiles lists the day files oldest first (names sort chronologically).
func (s Store) dayFiles() []string {
	files, _ := filepath.Glob(filepath.Join(s.Dir, "*.jsonl"))
	slices.Sort(files)
	return files
}
