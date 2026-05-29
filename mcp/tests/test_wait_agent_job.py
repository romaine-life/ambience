"""Regression tests for ambience_preview.ops.wait_agent_job.

The function previously read .status.succeeded / .status.failed as the
sole completion signal and could mislabel a Completed Job as a failure
when the controller had not yet stamped the count by the time `kubectl
logs -f` returned — or when `kubectl logs -f` itself stalled past the
wait deadline, leaving the second polling loop with zero iterations and
both fields empty. That mis-classification dropped the inner agent's
real verdict on ambience#170/runs/1.1. These tests pin the
condition-driven contract that fixes it.
"""
from __future__ import annotations

import sys
import time
import types
from pathlib import Path
from typing import Any, Iterable

import pytest

# Add the mcp/ directory to sys.path so the package imports without an
# editable install. Tests run from any working directory.
REPO_ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(REPO_ROOT))

from ambience_preview import ops  # noqa: E402


@pytest.fixture(autouse=True)
def stub_subprocess_popen(monkeypatch: pytest.MonkeyPatch) -> "list[StubPopen]":
    """Replace subprocess.Popen so wait_agent_job's `kubectl logs -f`
    streamer is a no-op. Returns the list of Popen calls so tests can
    assert tear-down happened."""
    calls: list[StubPopen] = []

    def _factory(cmd: list[str], *args: Any, **kwargs: Any) -> "StubPopen":
        stub = StubPopen(cmd)
        calls.append(stub)
        return stub

    monkeypatch.setattr(ops.subprocess, "Popen", _factory)
    return calls


@pytest.fixture(autouse=True)
def fast_sleep(monkeypatch: pytest.MonkeyPatch) -> None:
    """time.sleep -> no-op so polling loops advance immediately."""
    monkeypatch.setattr(ops.time, "sleep", lambda _: None)


class StubPopen:
    """Stand-in for subprocess.Popen that records terminate/kill calls."""

    def __init__(self, cmd: list[str]) -> None:
        self.cmd = cmd
        self.terminated = False
        self.killed = False
        self._alive = True

    def poll(self) -> int | None:
        return None if self._alive else 0

    def terminate(self) -> None:
        self.terminated = True
        self._alive = False

    def kill(self) -> None:
        self.killed = True
        self._alive = False

    def wait(self, timeout: float | None = None) -> int:
        self._alive = False
        return 0


class FakeKubectl:
    """Replays a scripted set of `kubectl get ... -o jsonpath` responses
    keyed on the field path the production code asks for. Each value is
    either a constant or a callable producing the next reply per call,
    so a test can simulate "empty for N polls, then populated"."""

    def __init__(self, replies: dict[str, Any]) -> None:
        self._replies = replies
        self._counters: dict[str, int] = {}

    def replace(self) -> Any:
        def _run(command: list[str], *_, **__) -> str:
            key = self._classify(command)
            value = self._replies.get(key)
            if value is None:
                raise AssertionError(f"unhandled kubectl call: {command} key={key}")
            if callable(value):
                count = self._counters.get(key, 0)
                self._counters[key] = count + 1
                return value(count)
            return value

        return _run

    @staticmethod
    def _classify(command: list[str]) -> str:
        # Each call ends with `-o jsonpath={...}`. Use the jsonpath as
        # the key; pod lookup uses items[0] so we lump those together.
        if "-o" in command:
            idx = command.index("-o")
            spec = command[idx + 1] if idx + 1 < len(command) else ""
            return spec
        return " ".join(command)


def _install_kubectl(monkeypatch: pytest.MonkeyPatch, fake: FakeKubectl) -> None:
    monkeypatch.setattr(ops, "run_command", fake.replace())


def test_wait_agent_job_returns_on_complete_condition(
    monkeypatch: pytest.MonkeyPatch,
    stub_subprocess_popen: list[StubPopen],
) -> None:
    """Happy path: pod becomes Running, then Job condition Complete=True.
    Must return immediately and tear down the log streamer."""
    fake = FakeKubectl(
        {
            "jsonpath={.items[0].metadata.name}": "agent-pod",
            "jsonpath={.status.phase}": "Running",
            'jsonpath={.status.conditions[?(@.type=="Complete")].status}': "True",
            'jsonpath={.status.conditions[?(@.type=="Failed")].status}': "",
        }
    )
    _install_kubectl(monkeypatch, fake)

    result = ops.wait_agent_job(
        namespace="ambience-slot-3",
        job_name="agent-x",
        timeout_seconds=60,
        poll_interval_seconds=1,
    )

    assert result == {
        "namespace": "ambience-slot-3",
        "job": "agent-x",
        "pod": "agent-pod",
        "pod_phase": "Running",
    }
    assert stub_subprocess_popen and stub_subprocess_popen[0].terminated, (
        "log streamer must be torn down"
    )


def test_wait_agent_job_does_not_misread_zero_succeeded_as_failure(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """Regression for ambience#170: the old code raised
    'agent Job ... failed (succeeded=0, failed=0)' when the Job had
    actually Completed. Drive that exact shape — Complete=True with
    empty succeeded/failed counts — and assert the new code returns."""
    fake = FakeKubectl(
        {
            "jsonpath={.items[0].metadata.name}": "agent-pod",
            "jsonpath={.status.phase}": "Succeeded",
            'jsonpath={.status.conditions[?(@.type=="Complete")].status}': "True",
            'jsonpath={.status.conditions[?(@.type=="Failed")].status}': "",
        }
    )
    _install_kubectl(monkeypatch, fake)

    result = ops.wait_agent_job(
        namespace="ambience-slot-3",
        job_name="agent-incident-170",
        timeout_seconds=60,
        poll_interval_seconds=1,
    )

    assert result["pod_phase"] == "Succeeded"


def test_wait_agent_job_raises_on_failed_condition(
    monkeypatch: pytest.MonkeyPatch,
    stub_subprocess_popen: list[StubPopen],
) -> None:
    """Job condition Failed=True must raise with the controller's
    reason, and the streamer must still be torn down."""
    fake = FakeKubectl(
        {
            "jsonpath={.items[0].metadata.name}": "agent-pod",
            "jsonpath={.status.phase}": "Running",
            'jsonpath={.status.conditions[?(@.type=="Complete")].status}': "",
            'jsonpath={.status.conditions[?(@.type=="Failed")].status}': "True",
            'jsonpath={.status.conditions[?(@.type=="Failed")].reason}': "BackoffLimitExceeded",
        }
    )
    _install_kubectl(monkeypatch, fake)

    with pytest.raises(ops.CommandError, match="BackoffLimitExceeded"):
        ops.wait_agent_job(
            namespace="ambience-slot-3",
            job_name="agent-bad",
            timeout_seconds=60,
            poll_interval_seconds=1,
        )

    assert stub_subprocess_popen[0].terminated


def test_wait_agent_job_falls_back_to_pod_phase_when_condition_slow(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """Pod reaches Succeeded but the Job controller hasn't yet stamped
    Complete. We honour pod phase as a secondary signal (backoffLimit=0
    makes pod phase definitive for Job outcome)."""
    phases = ["Running", "Succeeded"]
    counter = {"i": 0}

    def _phase(_call: int) -> str:
        value = phases[min(counter["i"], len(phases) - 1)]
        counter["i"] += 1
        return value

    fake = FakeKubectl(
        {
            "jsonpath={.items[0].metadata.name}": "agent-pod",
            "jsonpath={.status.phase}": _phase,
            'jsonpath={.status.conditions[?(@.type=="Complete")].status}': "",
            'jsonpath={.status.conditions[?(@.type=="Failed")].status}': "",
        }
    )
    _install_kubectl(monkeypatch, fake)

    result = ops.wait_agent_job(
        namespace="ambience-slot-3",
        job_name="agent-slow-controller",
        timeout_seconds=60,
        poll_interval_seconds=1,
    )

    assert result["pod_phase"] == "Succeeded"


def test_wait_agent_job_times_out_when_no_terminal_state(
    monkeypatch: pytest.MonkeyPatch,
    stub_subprocess_popen: list[StubPopen],
) -> None:
    """Pod stays Running forever; deadline must trip and the streamer
    must be torn down. Previously this case was masked because the wait
    loop was bypassed entirely when `kubectl logs -f` hung."""
    fake = FakeKubectl(
        {
            "jsonpath={.items[0].metadata.name}": "agent-pod",
            "jsonpath={.status.phase}": "Running",
            'jsonpath={.status.conditions[?(@.type=="Complete")].status}': "",
            'jsonpath={.status.conditions[?(@.type=="Failed")].status}': "",
        }
    )
    _install_kubectl(monkeypatch, fake)

    # Drive deadline by patching time.time to advance on each call. The
    # first call sets the deadline; subsequent calls trip it.
    now = [1000.0]

    def _time() -> float:
        now[0] += 30
        return now[0]

    monkeypatch.setattr(ops.time, "time", _time)

    with pytest.raises(ops.CommandError, match="did not reach a terminal"):
        ops.wait_agent_job(
            namespace="ambience-slot-3",
            job_name="agent-hung",
            timeout_seconds=60,
            poll_interval_seconds=1,
        )

    assert stub_subprocess_popen[0].terminated


def test_wait_agent_job_raises_if_pod_never_appears(
    monkeypatch: pytest.MonkeyPatch,
    stub_subprocess_popen: list[StubPopen],
) -> None:
    """If the Job's pod never registers before the deadline, raise
    without starting a log streamer at all."""
    fake = FakeKubectl(
        {
            "jsonpath={.items[0].metadata.name}": "",
        }
    )
    _install_kubectl(monkeypatch, fake)

    now = [1000.0]

    def _time() -> float:
        now[0] += 30
        return now[0]

    monkeypatch.setattr(ops.time, "time", _time)

    with pytest.raises(ops.CommandError, match="never appeared"):
        ops.wait_agent_job(
            namespace="ambience-slot-3",
            job_name="agent-ghost",
            timeout_seconds=60,
            poll_interval_seconds=1,
        )

    assert stub_subprocess_popen == [], (
        "log streamer should not be created when pod never appears"
    )
