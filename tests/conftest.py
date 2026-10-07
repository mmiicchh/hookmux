import os
import subprocess
from pathlib import Path

import pytest

HERE = Path(__file__).parent
RULES = (HERE / "rules.yaml").read_text()
RULES_NONE = (HERE / "rules_none.yaml").read_text()
FAKEPARENT = str(HERE / "fakeparent.sh")
SHELL_LAYER = ["/bin/sh", "-c", '"$0" "$@"; exit $?']  # how agents spawn hooks


@pytest.fixture(scope="session")
def binary():
    p = os.environ.get("HOOKMUX_BIN", str(HERE.parent / "bin" / "hookmux"))
    assert Path(p).exists(), f"build first: {p}"
    return p


@pytest.fixture
def hist_dir(tmp_path):
    return tmp_path / "hist"


@pytest.fixture
def config(tmp_path, hist_dir):
    """The shared rules plus a per-test history dir. A test may overwrite it."""
    p = tmp_path / "config.yaml"
    p.write_text(f"hist_dir: {hist_dir}\n{RULES}")
    return p


@pytest.fixture
def custom(config, hist_dir):
    """custom(yaml) replaces the shared rules for one test; history stays in the test dir."""
    def write(text):
        config.write_text(f"hist_dir: {hist_dir}\n{text}")
    return write


@pytest.fixture
def hookmux(binary, config):
    """hookmux(*args) runs the binary with the test config; returns CompletedProcess."""
    def run(*args, stdin="", env=None, parent=None, via_shell=False):
        argv = [binary, "--config", str(config), *args]
        if via_shell:
            argv = [*SHELL_LAYER, *argv]
        if parent:
            argv = [FAKEPARENT, parent, *argv]
        return subprocess.run(argv, input=stdin, capture_output=True, text=True,
                              env={**os.environ, **(env or {})})
    return run


@pytest.fixture
def hm(hookmux):
    """hm(*cmd) runs `hookmux run <cmd>`; asserts the exit code, returns stdout stripped."""
    def run(*cmd, stdin="", env=None, parent=None, via_shell=False, exit=0):
        r = hookmux("run", *cmd, stdin=stdin, env=env, parent=parent, via_shell=via_shell)
        assert r.returncode == exit, r.stderr
        return r.stdout.strip()
    return run
