# hookmux

hookmux routes one command invocation to another command, by rules, and
records every call. It exists for the hooks of AI coding agents. Claude Code,
Codex CLI, Gemini CLI and Cursor all run a hook as a shell command with JSON
on stdin and read the result from stdout. hookmux sits in that path: the
agent calls hookmux with a name for the hook, hookmux records the call, picks
the first rule that matches, and runs that rule's target with the same stdin.
The target's stdout and exit code return to the agent unchanged.

A typical setup names each hook in the agent's configuration and decides in
hookmux's configuration which script that name runs. With a v1 script in
daily use and a v2 script under development, one rule sends the sessions you
choose to v2 and a second rule sends everything else to v1:

```
  agent config:  "command": "hookmux run pretool"

                    ┌─────────────────────────┐
  Claude Code ─────▶│ hookmux run pretool     │   env HOOKMUX_PROFILE=v2
  (JSON on stdin)   │                         ├────────────────────────▶ ~/hooks/pretool-v2.sh
                    │ 1. record the call      │
                    │ 2. first matching rule  │   otherwise
                    │ 3. run it, same stdin   ├────────────────────────▶ ~/hooks/pretool-v1.sh
                    └────────────┬────────────┘
                                 ▼
                 ~/.local/state/hookmux/hist/  (hookmux hist, hookmux view)
```

Nothing in hookmux depends on agents. Any program that calls a command with
input on stdin and reads its output can run through it: a git hook, an
editor's formatter command, a cron job whose implementation differs per
machine.

hookmux runs on macOS and Linux.

## Install

```sh
go install github.com/mmiicchh/hookmux/cmd/hookmux@latest
```

## Setting it up

Give each hook a name in the agent's configuration. For Claude Code that is
`~/.claude/settings.json`:

```json
"hooks": {
  "PreToolUse":       [{ "matcher": "Bash", "hooks": [{ "type": "command", "command": "hookmux run pretool" }] }],
  "UserPromptSubmit": [{ "hooks": [{ "type": "command", "command": "hookmux run prompt" }] }]
},
"statusLine": { "type": "command", "command": "hookmux run statusline" }
```

Write one rule per name in `~/.config/hookmux/config.yaml`:

```yaml
rules:
  - name: pretool-v2
    when: cmd0 == "pretool" && env("HOOKMUX_PROFILE") == "v2"
    exec: ~/hooks/pretool-v2.sh
  - name: pretool-v1
    when: cmd0 == "pretool"
    exec: ~/hooks/pretool-v1.sh
  - name: prompt
    when: cmd0 == "prompt"
    exec: ~/hooks/prompt.sh
  - name: statusline
    when: cmd0 == "statusline"
    exec: ~/.claude/statusline.sh
    sample_rate: 0            # do not record these calls; failures are still recorded
```

Run `hookmux check` after editing the rules. It compiles them and confirms
that every target exists.

## Configuration

hookmux reads `~/.config/hookmux/config.yaml`. The `--config` flag or the
`HOOKMUX_CONFIG` variable selects another file. Every key is optional.

```yaml
rules: []               # see below
view_port: 4665         # hookmux view listens on 127.0.0.1:<view_port>
hist_dir: ~/.local/state/hookmux/hist
hist_keep_days: 14      # older day files are deleted
hist_max_kb: 16         # per stream: stdin, stdout and stderr are each cut to this
exec_timeout: 0s        # 0 relies on the agent's own hook timeout
```

## Rules

A rule has a name, an optional `when` condition, a target made of `exec` and
`exec_args`, and an optional `sample_rate`.

hookmux evaluates the rules from top to bottom and runs the first one whose
`when` is true. A rule without `when` matches every call, so it belongs at
the end of the list as a catch-all; `check` reports a rule that matches every
call while other rules follow it. A rule with `when: "false"` never matches,
which keeps it in the file without running it.

```yaml
rules:
  - name: deny-force-push
    when: in_json().?tool_input.?command.orValue("").contains("push --force")
    exec: ~/hooks/deny.sh "force pushes go through a human"

  - name: pretool
    when: cmd0 == "pretool"
    exec: ~/hooks/pretool.sh
    exec_args: cmd1n

  - name: everything-else
    exec: true
```

### `when`

Conditions are written in [CEL](https://cel.dev), a small expression language
with C-like syntax. Four variables describe the invocation and five functions
look at its surroundings. The functions run only when a rule reaches them,
and each result is cached for the rest of the invocation.

| Expression | Meaning | Example |
|---|---|---|
| `argv` | hookmux's own argument list, exactly as received | `argv[0].endsWith("/hookmux")` |
| `cmd` | everything after `run`: the hook name or the wrapped command | `cmd.size() > 1` |
| `cmd0` | the first element of `cmd`, or `""` | `cmd0 == "pretool"` |
| `cmd1n` | `cmd` without its first element | `"--strict" in cmd1n` |
| `in_json()` | stdin parsed as JSON; empty or non-JSON stdin reads as `{}` | `in_json().hook_event_name == "Stop"` |
| `in_raw()` | stdin as a string | `in_raw().startsWith("refs/")` |
| `env("K")` | a variable from hookmux's environment | `env("HOOKMUX_PROFILE") == "v2"` |
| `tmux_env("K")` | a variable from the enclosing tmux session, read live | `tmux_env("HOOKMUX_PROFILE") == "v2"` |
| `ancestors()` | the parent process chain as argv[0] basenames, nearest first | `"codex" in ancestors()` |

Reading a field that is absent from the payload is an evaluation error. For
a field that may be missing, use optional chaining with a default:
`in_json().?tool_input.?command.orValue("")`. Lists from the payload take
the CEL macros: `in_json().tool_input.files.exists(f, f.endsWith(".go"))`.

`env` reads the environment the agent process started with, which stays
fixed for that process. `tmux_env` reads the tmux session that contains the
agent each time a rule evaluates it, so it can change while the agent runs:

```sh
tmux display-message -p '#S'                       # the current session's name, e.g. s3
tmux set-environment -t s3 HOOKMUX_PROFILE v2      # takes effect on the session's next hook call
tmux show-environment -t s3 HOOKMUX_PROFILE        # read it back
tmux set-environment -t s3 -u HOOKMUX_PROFILE      # unset
```

Outside tmux, `tmux_env` returns `""`. It is the only function that starts a
process.

### `exec` and `exec_args`

`exec` is a command line. hookmux splits it the way a shell splits words, so
quotes group arguments, but no shell runs and nothing in it is substituted. A
leading `~/` expands to the home directory. The command `true` makes a rule
that allows the call and does nothing else.

`exec_args` is a CEL expression whose value is a list of strings; hookmux
appends the list to `exec` as separate arguments. It uses the same variables
and functions as `when`:

| `exec_args` | Result |
|---|---|
| `cmd1n` | the hook's own arguments, unchanged |
| `cmd` | the whole wrapped command (a passthrough when `exec` is omitted) |
| `'["--strict"] + cmd1n'` | a literal inserted before them |
| `'["--user", env("USER"), cmd0]'` | literals and values mixed |
| `cmd.slice(1, 2) + ["--foo"] + cmd.slice(2, cmd.size())` | the first argument, then `--foo`, then the rest |
| `cmd1n.filter(a, a != "--verbose")` | an argument removed |
| `'[in_json().?cwd.orValue(".")]'` | a payload field as an argument |

Quote an expression that starts with `[` in the YAML file, as in the table;
unquoted, YAML reads it as a list rather than as text.

Either field may be omitted, but together they must produce at least one
word.

### `sample_rate`

`sample_rate` is the share of runs that exit 0 which hookmux writes to the
history, from `0` to `1` (the default). hookmux records every other run
regardless of the rate: a failure on its own side, and a target that exits
non-zero, such as a deny hook. A status line that refreshes every few seconds
can carry `sample_rate: 0` and still leave every problem in the history.

## When something goes wrong

Every call to `hookmux run` ends in one of two ways: a rule's target runs,
and its exit code and output return to the agent; or hookmux exits 2 and
prints one line to stderr that names the rule and the problem. hookmux does
not exit 0 on its own account. These situations exit 2:

- the configuration file does not load, or a rule does not compile;
- no rule matches the call, or the configuration has no rules;
- a `when` or `exec_args` expression fails to evaluate, for example by
  reading a field that the payload does not have;
- the target does not exist, is killed by a signal, or runs past
  `exec_timeout`.

Agents treat exit 2 as a blocked hook and show the message. A hook that
guards against dangerous actions therefore stops them when it is broken,
instead of letting them through quietly. hookmux records every failed call
in the history, whatever the rule's `sample_rate`. `hookmux check` reports
configuration problems before any hook runs.

## Examples

Block a dangerous command when the agent is Codex, allow it elsewhere:

```yaml
rules:
  - name: codex-no-prod
    when: >
      "codex" in ancestors() &&
      in_json().?tool_input.?command.orValue("").matches("kubectl .*prod")
    exec: ~/hooks/deny.sh "production commands are not allowed from Codex"
```

Wrap an existing hook. Put `hookmux run` in front of the script and add a
rule that runs the wrapped command as given. The agent sees no difference,
and hookmux records every call and can redirect it later:

```json
"command": "hookmux run ~/hooks/master.sh --verbose"
```

```yaml
rules:
  - name: passthrough
    exec_args: cmd
```

Route by event. `hookmux run` with nothing after it leaves routing to the
rules; the payload names the event, so one agent-config line serves every
hook slot:

```json
"command": "hookmux run"
```

```yaml
rules:
  - name: prompt
    when: in_json().hook_event_name == "UserPromptSubmit"
    exec: ~/hooks/context.sh
  - name: stop
    when: in_json().hook_event_name == "Stop"
    exec: ~/hooks/notify.sh
  - name: other-events
    exec: true
```

## Commands

`hookmux run [cmd…]` is the command the agent calls. Everything after `run`
is available to the rules as `cmd`.

`hookmux hist` lists recent invocations, newest first. `hookmux hist 3` shows
the third row in full: the arguments, the result of every rule that was
evaluated, the command that ran, and the stdin, stdout and stderr it saw. A
`⚠` after the exit code marks a call where hookmux itself failed.
`hookmux hist --clear` deletes the history.

`hookmux view` serves the same history as a local web page on port 4665 and
opens it in the browser; `--no-browser` only prints the URL. The page shows
the latest rows and loads more on request. The filter matches rule names and
commands; `stdin:text`, `stdout:`, `stderr:`, `error:`, `cwd:` and `exit:2`
reach other fields, and `-word` excludes. A `pretty` toggle formats JSON
payloads.

`hookmux check` compiles every rule and reports the ones that do not
type-check, do not evaluate to a boolean, point at a target that does not
exist, or match every call while other rules follow them. It also reports an
empty rule list.

## Development

The project uses Go, [Task](https://taskfile.dev) and
[uv](https://docs.astral.sh/uv/). `task build` builds the binary, `task test`
runs the Go unit tests, `task e2e` runs the black-box suite in `tests/`
against the built binary, `task check` runs `gofmt` and `go vet`, and
`task install` copies a build that passed all three to `~/.local/bin`. The
black-box tests share one rule file, `tests/rules.yaml`; each test is a
single invocation with an expected line of output.
