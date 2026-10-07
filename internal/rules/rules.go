// Package rules compiles rules once and evaluates them against one invocation.
package rules

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"slices"
	"strings"

	"cel.dev/cel-go/cel"
	celast "cel.dev/cel-go/common/ast"
	"cel.dev/cel-go/common/types"
	"cel.dev/cel-go/common/types/ref"
	"cel.dev/cel-go/ext"
	shellquote "github.com/kballard/go-shellquote"

	"github.com/mmiicchh/hookmux/internal/config"
	"github.com/mmiicchh/hookmux/internal/procs"
)

// Ctx holds one invocation's facts. Derived values are computed on first use and memoised.
type Ctx struct {
	Cmd   []string
	Stdin []byte

	vars      map[string]any
	payload   map[string]any
	raw       *string
	ancestors []string
	tmuxEnv   map[string]string
}

func (c *Ctx) variables() map[string]any {
	if c.vars == nil {
		cmd0, cmd1n := "", []string{}
		if len(c.Cmd) > 0 {
			cmd0, cmd1n = c.Cmd[0], c.Cmd[1:]
		}
		c.vars = map[string]any{"argv": os.Args, "cmd": c.Cmd, "cmd0": cmd0, "cmd1n": cmd1n}
	}
	return c.vars
}

// json is stdin parsed once. Anything that is not a JSON object reads as {}:
// it is not a payload, so payload rules are false rather than broken.
func (c *Ctx) json() map[string]any {
	if c.payload == nil {
		if json.Unmarshal(c.Stdin, &c.payload) != nil || c.payload == nil {
			c.payload = map[string]any{}
		}
	}
	return c.payload
}

// Field is a top-level string field of the payload, or "".
func (c *Ctx) Field(name string) string {
	s, _ := c.json()[name].(string)
	return s
}

func (c *Ctx) rawStdin() string {
	if c.raw == nil {
		s := string(c.Stdin)
		c.raw = &s
	}
	return *c.raw
}

// AncestorNames is what ancestors() saw: argv[0] basenames, parent first; nil until evaluated.
func (c *Ctx) AncestorNames() []string { return c.ancestors }

func (c *Ctx) ancestorNames() []string {
	if c.ancestors == nil {
		c.ancestors = procs.Ancestors()
	}
	return c.ancestors
}

// tmuxEnvVar reads one variable from the enclosing tmux session's environment.
// This is the only place hookmux spawns a process besides the target; once per key, lazily.
func (c *Ctx) tmuxEnvVar(key string) string {
	if v, ok := c.tmuxEnv[key]; ok {
		return v
	}
	if c.tmuxEnv == nil {
		c.tmuxEnv = map[string]string{}
	}
	v := ""
	if pane := os.Getenv("TMUX_PANE"); pane != "" {
		if out, err := exec.Command("tmux", "show-environment", "-t", pane, key).Output(); err == nil {
			_, v, _ = strings.Cut(strings.TrimSpace(string(out)), "=")
		}
	}
	c.tmuxEnv[key] = v
	return v
}

// Result is one rule's outcome for the history.
type Result struct {
	Name  string `json:"name"`
	Match bool   `json:"match"`
	Err   string `json:"err,omitempty"`
}

type Compiled struct {
	config.Rule
	Err      error       // compile error; the rule cannot be evaluated
	when     cel.Program // nil: the condition is constant (see always/never)
	never    bool        // constant false: parked
	execArgs cel.Program // nil: no appended arguments
	exec     []string    // Exec split, home expanded
}

// Always reports a rule that matches every invocation: no `when`, or a constant true.
func (r *Compiled) Always() bool { return r.Err == nil && r.when == nil && !r.never }

// Never reports a rule whose `when` is constant false: parked, it can never run.
func (r *Compiled) Never() bool { return r.Err == nil && r.never }

// Exec0 is the program `exec` names, or "" when the rule relies on exec_args alone.
func (r *Compiled) Exec0() string {
	if len(r.exec) == 0 {
		return ""
	}
	return r.exec[0]
}

// Set is a compiled rule list. The CEL functions read the invocation from
// ctx, which Select and Target bind before evaluating.
type Set struct {
	rules []Compiled
	ctx   *Ctx
}

func (s *Set) Rules() []Compiled { return s.rules }

// Compile type-checks every rule. The returned error is for the CEL
// environment itself; per-rule problems are on Compiled.Err, in order.
func Compile(rs []config.Rule) (*Set, error) {
	s := &Set{}
	env, err := s.env()
	if err != nil {
		return nil, err
	}
	folder, err := constantFolder()
	if err != nil {
		return nil, err
	}
	for _, r := range rs {
		s.rules = append(s.rules, compile(env, folder, r))
	}
	return s, nil
}

func (s *Set) env() (*cel.Env, error) {
	adapt := types.DefaultTypeAdapter
	nullary := func(name string, ret *cel.Type, f func(*Ctx) ref.Val) cel.EnvOption {
		return cel.Function(name, cel.Overload(name, nil, ret,
			cel.FunctionBinding(func(...ref.Val) ref.Val { return f(s.ctx) })))
	}
	unary := func(name string, f func(*Ctx, string) string) cel.EnvOption {
		return cel.Function(name, cel.Overload(name+"_string", []*cel.Type{cel.StringType}, cel.StringType,
			cel.UnaryBinding(func(a ref.Val) ref.Val {
				k, ok := a.Value().(string)
				if !ok {
					return types.NewErr("%s: argument must be a string", name)
				}
				return types.String(f(s.ctx, k))
			})))
	}
	return cel.NewEnv(
		cel.OptionalTypes(),
		ext.Lists(),
		ext.Strings(),
		cel.Variable("argv", cel.ListType(cel.StringType)),
		cel.Variable("cmd", cel.ListType(cel.StringType)),
		cel.Variable("cmd0", cel.StringType),
		cel.Variable("cmd1n", cel.ListType(cel.StringType)),
		nullary("in_json", cel.DynType, func(c *Ctx) ref.Val { return adapt.NativeToValue(c.json()) }),
		nullary("in_raw", cel.StringType, func(c *Ctx) ref.Val { return types.String(c.rawStdin()) }),
		nullary("ancestors", cel.ListType(cel.StringType), func(c *Ctx) ref.Val {
			return adapt.NativeToValue(c.ancestorNames())
		}),
		unary("env", func(_ *Ctx, k string) string { return os.Getenv(k) }),
		unary("tmux_env", (*Ctx).tmuxEnvVar),
	)
}

// constantFolder detects conditions that are constant regardless of the invocation.
func constantFolder() (*cel.StaticOptimizer, error) {
	folding, err := cel.NewConstantFoldingOptimizer()
	if err != nil {
		return nil, err
	}
	return cel.NewStaticOptimizer(folding)
}

func compile(env *cel.Env, folder *cel.StaticOptimizer, r config.Rule) Compiled {
	cr := Compiled{Rule: r}
	var err error
	if cr.when, cr.never, err = condition(env, folder, r.When); err != nil {
		cr.Err = fmt.Errorf("when: %w", err)
		return cr
	}
	if cr.execArgs, err = program(env, r.ExecArgs, cel.ListType(cel.StringType)); err != nil {
		cr.Err = fmt.Errorf("exec_args: %w", err)
		return cr
	}
	if cr.exec, err = shellquote.Split(r.Exec); err != nil {
		cr.Err = fmt.Errorf("exec: %w", err)
		return cr
	}
	if len(cr.exec) > 0 {
		cr.exec[0] = config.ExpandHome(cr.exec[0])
	}
	return cr
}

// condition compiles `when`. A constant condition yields no program: true is
// "always" (nil, false), false is "never" (nil, true).
func condition(env *cel.Env, folder *cel.StaticOptimizer, src string) (cel.Program, bool, error) {
	ast, err := typed(env, src, cel.BoolType)
	if err != nil || ast == nil {
		return nil, false, err
	}
	if folded, iss := folder.Optimize(env, ast); iss.Err() == nil {
		if e := folded.NativeRep().Expr(); e.Kind() == celast.LiteralKind {
			if b, ok := e.AsLiteral().Value().(bool); ok {
				return nil, !b, nil
			}
		}
	}
	prg, err := env.Program(ast)
	return prg, false, err
}

// program compiles src into a runnable program; empty src → nil.
func program(env *cel.Env, src string, want *cel.Type) (cel.Program, error) {
	ast, err := typed(env, src, want)
	if err != nil || ast == nil {
		return nil, err
	}
	return env.Program(ast)
}

// typed compiles src and checks its type against want; empty src → nil AST.
// dyn (e.g. a list built from in_json() values) passes either way; the
// elements are verified when the value is used.
func typed(env *cel.Env, src string, want *cel.Type) (*cel.Ast, error) {
	if strings.TrimSpace(src) == "" {
		return nil, nil
	}
	ast, issues := env.Compile(src)
	if issues.Err() != nil {
		return nil, issues.Err()
	}
	if got := ast.OutputType(); !want.IsAssignableType(got) && !got.IsAssignableType(want) {
		return nil, fmt.Errorf("must be %s, got %s", want, got)
	}
	return ast, nil
}

// Select evaluates rules in order and returns the first match with every result so far.
// An evaluation error stops the search: a rule that cannot be judged must not be skipped.
func (s *Set) Select(c *Ctx) (*Compiled, []Result, error) {
	s.ctx = c
	var results []Result
	for i := range s.rules {
		r := &s.rules[i]
		res := Result{Name: r.Name}
		switch {
		case r.never:
		case r.when == nil:
			res.Match = true
		default:
			out, _, err := r.when.Eval(c.variables())
			if err != nil {
				res.Err = err.Error()
			} else if b, ok := out.Value().(bool); !ok {
				res.Err = fmt.Sprintf("when returned %T, not bool", out.Value())
			} else {
				res.Match = b
			}
		}
		results = append(results, res)
		if res.Err != "" {
			return nil, results, fmt.Errorf("rule %q: when: %s", r.Name, res.Err)
		}
		if res.Match {
			return r, results, nil
		}
	}
	return nil, results, nil
}

// Target builds the argv to run: exec followed by the evaluated exec_args.
func (s *Set) Target(r *Compiled, c *Ctx) ([]string, error) {
	s.ctx = c
	target := slices.Clone(r.exec)
	if r.execArgs != nil {
		out, _, err := r.execArgs.Eval(c.variables())
		if err != nil {
			return nil, fmt.Errorf("exec_args: %w", err)
		}
		args, err := out.ConvertToNative(reflect.TypeOf([]string{}))
		if err != nil {
			return nil, fmt.Errorf("exec_args: must be a list of strings: %w", err)
		}
		target = append(target, args.([]string)...)
	}
	if len(target) == 0 {
		return nil, fmt.Errorf("exec and exec_args produced nothing to run")
	}
	return target, nil
}
