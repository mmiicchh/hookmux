package hist

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEachStreamIsCappedOnItsOwn(t *testing.T) {
	s := Store{Dir: t.TempDir(), MaxKB: 1}
	r := &Record{TS: time.Now(), Stdin: strings.Repeat("i", 3000), Stdout: "small", Stderr: strings.Repeat("e", 3000)}
	if err := s.Append(r); err != nil {
		t.Fatal(err)
	}
	if len(r.Stdin) != 1024 || len(r.Stderr) != 1024 || r.Stdout != "small" {
		t.Fatalf("stdin=%d stdout=%q stderr=%d", len(r.Stdin), r.Stdout, len(r.Stderr))
	}
	if r.StdinLen != 3000 || r.StderrLen != 3000 {
		t.Fatalf("original sizes lost: %d %d", r.StdinLen, r.StderrLen)
	}
}

func TestAppendThenListNewestFirstWithOriginalSizes(t *testing.T) {
	s := Store{Dir: t.TempDir(), MaxKB: 8}
	now := time.Now()
	for i, cmd := range []string{"one", "two"} {
		r := &Record{TS: now.Add(time.Duration(i) * time.Second), Cmd: []string{cmd}, Stdin: strings.Repeat("s", 9000)}
		r.ID = NewID(r.TS, i)
		if err := s.Append(r); err != nil {
			t.Fatal(err)
		}
	}
	recs, err := s.List(10)
	if err != nil || len(recs) != 2 {
		t.Fatalf("list: %v %d", err, len(recs))
	}
	if recs[0].Cmd[0] != "two" || recs[1].Cmd[0] != "one" {
		t.Errorf("order: %v %v", recs[0].Cmd, recs[1].Cmd)
	}
	if recs[0].StdinLen != 9000 || len(recs[0].Stdin) >= 9000 {
		t.Errorf("stdin should be truncated but its size kept: len=%d orig=%d", len(recs[0].Stdin), recs[0].StdinLen)
	}
	if recs, _ := s.List(1); len(recs) != 1 {
		t.Errorf("limit ignored: %d", len(recs))
	}
}

func TestFirstRecordOfADayPrunesOldFiles(t *testing.T) {
	s := Store{Dir: t.TempDir(), MaxKB: 8, KeepDays: 7}
	old := filepath.Join(s.Dir, "2020-01-01.jsonl")
	os.WriteFile(old, []byte("{}\n"), 0o644)
	if err := s.Append(&Record{TS: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(old); err == nil {
		t.Error("old file survived")
	}
	if files := s.dayFiles(); len(files) != 1 {
		t.Errorf("expected only today's file, got %v", files)
	}
	if err := s.Clear(); err != nil || len(s.dayFiles()) != 0 {
		t.Errorf("clear: %v, left %v", err, s.dayFiles())
	}
}
