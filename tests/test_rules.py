"""One invocation per test against tests/rules.yaml. Read the two files side by side."""


def test_a_payload_field_routes(hm):
    assert hm(stdin='{"cwd": "/x"}') == "PAYLOAD_CWD_X"


def test_rules_are_tried_top_to_bottom_and_the_first_match_wins(hm):
    assert hm(stdin='{"cwd": "/x", "tool_input": {"command": "go run x"}}') == "PAYLOAD_CWD_X"


def test_optional_chaining_tolerates_missing_fields(hm):
    assert hm(stdin='{"tool_input": {"command": "go run ./x"}}') == "GO_RUN_GUARD"
    assert hm(stdin='{"hook_event_name": "Stop"}') == ""


def test_an_environment_variable_switches_the_route(hm):
    assert hm(stdin="{}", env={"HOOKMUX_TEST_PROFILE": "v2"}) == "PROFILE_V2"


def test_the_parent_process_name_is_visible(hm):
    assert hm(stdin="{}", parent="fakeclaude") == "UNDER_FAKECLAUDE"
    assert hm(stdin="{}", parent="fakecodex") == ""


def test_the_parent_is_found_through_the_shell_agents_spawn_hooks_with(hm):
    assert hm(stdin="{}", parent="fakeclaude", via_shell=True) == "UNDER_FAKECLAUDE"


def test_exec_args_cmd1n_keeps_the_hooks_own_arguments(hm):
    assert hm("statusline", "cfg.sh", "--fast") == "STATUSLINE cfg.sh --fast"


def test_exec_args_can_reshape_the_arguments(hm):
    assert hm("/old/tool", "hook", "pre") == "NEW_TOOL hook --foo pre"


def test_exec_args_can_mix_literals_env_and_payload_fields(hm):
    assert hm("mixed", stdin='{"cwd": "/w"}', env={"HOOKMUX_TEST_USER": "u"}) == "MIXED --user u mixed /w"
    assert hm("mixed", stdin="{}", env={"HOOKMUX_TEST_USER": "u"}) == "MIXED --user u mixed ."


def test_exec_args_can_drop_an_argument(hm):
    assert hm("filtered", "--verbose", "keep") == "FILTERED keep"


def test_exec_is_split_like_a_shell_command_line(hm):
    assert hm("quote") == "literal $1 two words"


def test_the_catch_all_passthrough_runs_whatever_command_was_wrapped(hm):
    assert hm("echo", "PASSTHROUGH") == "PASSTHROUGH"
    assert hm("printf", "%s-%s", "a", "b") == "a-b"
    assert hm("true") == ""


def test_flags_of_the_wrapped_command_are_not_parsed_by_hookmux(hm):
    assert hm("echo", "--version", "-x") == "--version -x"


def test_the_targets_exit_code_and_stderr_come_back(hookmux):
    r = hookmux("run", "fail")
    assert r.returncode == 3 and r.stderr == "oops\n" and r.stdout == ""


def test_a_rule_without_when_catches_everything_left(hm):
    assert hm(stdin='{"hook_event_name": "Stop"}') == ""          # other-events: exec true
    assert hm(stdin='{"hook_event_name": "Notification"}') == ""


def test_when_true_is_the_same_as_omitting_it(hm, custom):
    custom('rules: [{name: all, when: "true", exec: echo ALL}]')
    assert hm("anything") == "ALL"
    assert hm(stdin="{}") == "ALL"


def test_when_false_disables_a_rule(hm, custom):
    custom('rules: [{name: parked, when: "false", exec: echo NO}, {name: rest, exec: echo REST}]')
    assert hm("x") == "REST"


def test_non_json_stdin_makes_in_json_rules_false_and_in_raw_rules_work(hm):
    assert hm(stdin="plain text") == "RAW_TEXT"
    assert hm(stdin="not json") == ""


def test_sample_rate_zero_skips_successful_records_but_keeps_failures(hm, hookmux, hist_dir):
    assert hm("quiet") == "QUIET"
    assert not list(hist_dir.glob("*.jsonl"))
    assert hookmux("run", "quiet-fail").returncode == 4
    assert "quiet-fail" in hookmux("hist").stdout


# --- hookmux's own problems are loud: exit 2 and one line on stderr -----------


def test_no_rules_is_an_error(hookmux, custom):
    from conftest import RULES_NONE
    custom(RULES_NONE)
    r = hookmux("run", "pretool", stdin="{}")
    assert r.returncode == 2 and "no rules in" in r.stderr


def test_no_matching_rule_is_an_error_naming_the_cmd(hookmux, custom):
    custom("rules: [{name: never, when: 'false', exec: echo NEVER}]")
    r = hookmux("run", "pretool", "--strict", stdin='{"hook_event_name": "PreToolUse"}')
    assert r.returncode == 2
    assert 'no rule matched cmd ["pretool" "--strict"] (hook_event_name "PreToolUse")' in r.stderr
    r = hookmux("run", stdin="")
    assert r.returncode == 2 and "no rule matched cmd []" in r.stderr


def test_check_flags_a_rule_that_shadows_the_ones_after_it(hookmux, custom):
    custom("rules: [{name: all, exec: echo all}, {name: unreachable, exec: echo x}]")
    r = hookmux("check")
    assert r.returncode == 1 and "✗ all" in r.stdout and "can never run" in r.stdout


def test_an_evaluation_error_fails_loudly_naming_the_rule(hookmux):
    r = hookmux("run", "evalerr")
    assert r.returncode == 2 and 'rule "eval-error": when:' in r.stderr


def test_a_missing_target_fails_loudly(hookmux):
    r = hookmux("run", "/no/such/hook")
    assert r.returncode == 2 and 'rule "passthrough"' in r.stderr and "/no/such/hook" in r.stderr


def test_a_rule_that_does_not_compile_fails_loudly(hookmux, custom):
    custom("rules: [{name: typo, when: 'cmd0 ====', exec: echo x}]")
    r = hookmux("run", "echo", "x")
    assert r.returncode == 2 and 'rule "typo" does not compile' in r.stderr


def test_a_broken_config_fails_loudly(hookmux, custom):
    custom("rules: [{name: x, exec: y, bogus: 1}]")
    r = hookmux("run", "echo", "x")
    assert r.returncode == 2 and "bogus" in r.stderr
