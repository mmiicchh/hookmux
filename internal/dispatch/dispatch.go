// Package dispatch runs one hookmux invocation: select a rule, exec its target, record it.
package dispatch

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"os"
	"os/exec"
	"time"

	"github.com/mmiicchh/hookmux/internal/config"
	"github.com/mmiicchh/hookmux/internal/hist"
	"github.com/mmiicchh/hookmux/internal/rules"
)

const (
	failExit  = 2 // what agents read as "blocked hook": a broken guard stops the action
	pipeGrace = 500 * time.Millisecond
)

type Request struct {
	Version    string
	Config     config.Config
	ConfigPath string
	ConfigErr  error
	Cmd        []string
	Hist       hist.Store
	Log        *log.Logger
	Stdin      io.Reader
	Stdout     io.Writer
	Stderr     io.Writer
}

// Run is fail-closed: every invocation either runs a target or exits 2 with
// one line on stderr saying why, including "no rules" and "no rule matched".
func Run(req Request) *hist.Record {
	start := time.Now()
	rec := &hist.Record{TS: start, PID: os.Getpid(), Version: req.Version, Argv: os.Args, Cmd: req.Cmd,
		Config: req.ConfigPath, Rules: []rules.Result{}}
	rec.ID = hist.NewID(start, rec.PID)
	rec.Cwd, _ = os.Getwd()
	ctx := &rules.Ctx{Cmd: req.Cmd, Stdin: readStdin(req.Stdin)}
	rec.Stdin = string(ctx.Stdin)

	match, err := dispatch(req, rec, ctx)
	if err != nil {
		rec.Error, rec.Exit = err.Error(), failExit
		req.Log.Println(err)
	}
	rec.Ancestors = ctx.AncestorNames()
	rec.DurationMS = time.Since(start).Milliseconds()
	// Sampling thins records of runs that exited 0; everything else is kept.
	if rec.Exit == 0 && rand.Float64() >= match.Sample() {
		return rec
	}
	if err := req.Hist.Append(rec); err != nil {
		req.Log.Println("hist:", err)
	}
	return rec
}

// dispatch runs the invocation and returns the rule that ran.
func dispatch(req Request, rec *hist.Record, ctx *rules.Ctx) (*rules.Compiled, error) {
	if req.ConfigErr != nil {
		rec.ConfigErr = req.ConfigErr.Error()
		return nil, fmt.Errorf("config %s: %w", req.ConfigPath, req.ConfigErr)
	}
	if len(req.Config.Rules) == 0 {
		return nil, fmt.Errorf("no rules in %s; nothing to run", req.ConfigPath)
	}
	set, err := rules.Compile(req.Config.Rules)
	if err != nil {
		return nil, fmt.Errorf("rules: %w", err)
	}
	var broken error
	for _, r := range set.Rules() {
		if r.Err != nil {
			rec.Rules = append(rec.Rules, rules.Result{Name: r.Name, Err: "compile: " + r.Err.Error()})
			if broken == nil {
				broken = fmt.Errorf("rule %q does not compile: %w", r.Name, r.Err)
			}
		}
	}
	if broken != nil {
		return nil, broken
	}
	match, results, err := set.Select(ctx)
	rec.Rules = append(rec.Rules, results...)
	if err != nil {
		return nil, err
	}
	if match == nil {
		note := ""
		if ev := ctx.Field("hook_event_name"); ev != "" {
			note = fmt.Sprintf(" (hook_event_name %q)", ev)
		}
		return nil, fmt.Errorf("no rule matched cmd %q%s", req.Cmd, note)
	}
	rec.Matched = match.Name
	target, err := set.Target(match, ctx)
	if err != nil {
		return nil, fmt.Errorf("rule %q: %w", match.Name, err)
	}
	rec.Exec = target
	res := execute(target, ctx.Stdin, req.Config.ExecTimeout, req.Hist.CaptureLimit(), req.Stdout, req.Stderr)
	rec.Exit, rec.Stdout, rec.Stderr = res.exit, res.stdout, res.stderr
	if res.err != nil {
		return nil, fmt.Errorf("rule %q: %w", match.Name, res.err)
	}
	return match, nil
}

// readStdin returns nil on an interactive terminal so a manual `hookmux run` never blocks.
func readStdin(r io.Reader) []byte {
	if f, ok := r.(*os.File); ok {
		if st, err := f.Stat(); err == nil && st.Mode()&os.ModeCharDevice != 0 {
			return nil
		}
	}
	b, _ := io.ReadAll(r)
	return b
}

type result struct {
	exit           int
	stdout, stderr string
	err            error // target could not start or timed out
}

// execute runs target with stdin, teeing its output to ours. The target's own
// exit code is returned as is.
func execute(target []string, stdin []byte, timeout time.Duration, capBytes int, stdout, stderr io.Writer) result {
	ctx := context.Background()
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	out, errOut := &capped{limit: capBytes}, &capped{limit: capBytes}
	c := exec.CommandContext(ctx, target[0], target[1:]...)
	c.Stdin = bytes.NewReader(stdin)
	c.Stdout = io.MultiWriter(stdout, out)
	c.Stderr = io.MultiWriter(stderr, errOut)
	c.WaitDelay = pipeGrace // a backgrounded helper holding our pipes must not hang the hook
	err := c.Run()
	res := result{stdout: out.String(), stderr: errOut.String()}
	var exitErr *exec.ExitError
	switch {
	case err == nil:
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		res.err = fmt.Errorf("timeout after %s", timeout)
	case errors.As(err, &exitErr) && exitErr.ExitCode() >= 0:
		res.exit = exitErr.ExitCode()
	case errors.As(err, &exitErr):
		res.err = fmt.Errorf("target %v", exitErr.ProcessState) // killed by a signal
	default:
		res.err = err
	}
	return res
}

// capped keeps the first limit bytes and discards the rest without erroring.
type capped struct {
	bytes.Buffer
	limit int
}

func (c *capped) Write(p []byte) (int, error) {
	if room := c.limit - c.Len(); room > 0 {
		c.Buffer.Write(p[:min(room, len(p))])
	}
	return len(p), nil
}
