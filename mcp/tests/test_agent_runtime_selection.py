from __future__ import annotations

import json
import sys
from pathlib import Path

import pytest

REPO_ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(REPO_ROOT))

from ambience_preview.agent_runtime import resolve_stage_runtime  # noqa: E402
from ambience_preview import ops  # noqa: E402


def snapshot(**slot_profiles: dict[str, str]) -> str:
    slots = {
        slot: {
            "profile_id": values["profile_id"],
            "provider": values["provider"],
            "model": values["model"],
            "reasoning_effort": values.get("reasoning_effort", ""),
            "source": values.get("source", "issue"),
        }
        for slot, values in slot_profiles.items()
    }
    return json.dumps(
        {
            "default": {
                "profile_id": "default-codex",
                "provider": "codex",
                "model": "gpt-5.4",
                "reasoning_effort": "high",
                "source": "global",
            },
            "slots": slots,
        }
    )


@pytest.mark.parametrize(
    ("stage", "slot"),
    [
        ("issue-contract", "issue_contract"),
        ("test-plan", "test_plan"),
        ("implement", "implementation"),
        ("verify", "verification"),
    ],
)
def test_resolve_stage_runtime_uses_stable_stage_slots(stage: str, slot: str) -> None:
    runtime = resolve_stage_runtime(
        snapshot(
            **{
                slot: {
                    "profile_id": f"{slot}-claude",
                    "provider": "claude",
                    "model": "claude-sonnet-4-6",
                    "source": "issue",
                }
            }
        ),
        stage,
    )

    assert runtime.stage == stage
    assert runtime.slot == slot
    assert runtime.profile_id == f"{slot}-claude"
    assert runtime.provider == "claude"
    assert runtime.model == "claude-sonnet-4-6"


def test_resolve_stage_runtime_fails_when_snapshot_missing() -> None:
    with pytest.raises(ValueError, match="GLIMMUNG_AGENT_RUNTIME_JSON is required"):
        resolve_stage_runtime("", "implement")


def test_resolve_stage_runtime_fails_when_profile_missing() -> None:
    raw = json.dumps({"slots": {}})
    with pytest.raises(ValueError, match="no resolved profile for slot 'implementation'"):
        resolve_stage_runtime(raw, "implement")


def test_resolve_stage_runtime_fails_on_unsupported_provider() -> None:
    raw = snapshot(
        implementation={
            "profile_id": "bad",
            "provider": "unknown-ai",
            "model": "mystery",
        }
    )
    with pytest.raises(ValueError, match="unsupported provider 'unknown-ai'"):
        resolve_stage_runtime(raw, "implement")


def test_agent_job_spec_renders_selected_runtime() -> None:
    spec = ops._agent_job_spec(
        namespace="ambience-slot-1",
        job_name="agent-run-im-0",
        issue_number="12",
        issue_title="render selected runtime",
        issue_url="https://glimmung.example/issues/12",
        issue_reference="ambience#12",
        validation_url="https://slot.example",
        branch_name="glimmung/run-1",
        proxy_ip="10.0.0.5",
        claude_proxy_ip="10.0.0.6",
        codex_proxy_ip="10.0.0.7",
        agent_container_tag="native-runner-test",
        agent_container_image="romainecr.azurecr.io/ambience-agent-runner:native-runner-test",
        stage="implement",
        config_map_name="agent-config-implement",
        agent_runtime_json=snapshot(
            implementation={
                "profile_id": "impl-codex",
                "provider": "codex",
                "model": "gpt-5.5",
                "reasoning_effort": "xhigh",
                "source": "issue",
            }
        ),
    )

    env = {
        item["name"]: item["value"]
        for item in spec["spec"]["template"]["spec"]["containers"][0]["env"]
        if "value" in item
    }
    assert env["AGENT_RUNTIME_SLOT"] == "implementation"
    assert env["AGENT_RUNTIME_PROFILE_ID"] == "impl-codex"
    assert env["AGENT_PROVIDER"] == "codex"
    assert env["AGENT_MODEL"] == "gpt-5.5"
    assert env["AGENT_REASONING_EFFORT"] == "xhigh"
    assert (
        spec["spec"]["template"]["spec"]["containers"][0]["image"]
        == "romainecr.azurecr.io/ambience-agent-runner:native-runner-test"
    )
    pod_spec = spec["spec"]["template"]["spec"]
    assert pod_spec["hostAliases"] == [
        {"ip": "10.0.0.6", "hostnames": ["api.anthropic.com"]},
        {"ip": "10.0.0.7", "hostnames": ["chatgpt.com", "api.openai.com"]},
    ]
    assert all(volume["name"] != "codex-credentials" for volume in pod_spec["volumes"])
    assert all(
        mount["name"] != "codex-credentials"
        for mount in spec["spec"]["template"]["spec"]["containers"][0]["volumeMounts"]
    )
    assert "GH_TOKEN" not in env


def test_implementation_agent_job_can_route_git_through_github_policy_proxy() -> None:
    spec = ops._agent_job_spec(
        namespace="ambience-slot-1",
        job_name="agent-run-im-0",
        issue_number="12",
        issue_title="render selected runtime",
        issue_url="https://glimmung.example/issues/12",
        issue_reference="ambience#12",
        validation_url="https://slot.example",
        branch_name="glimmung/issue-12/run-1",
        proxy_ip="10.0.0.5",
        claude_proxy_ip="10.0.0.6",
        codex_proxy_ip="10.0.0.7",
        github_proxy_ip="10.0.0.8",
        agent_container_tag="native-runner-test",
        agent_container_image="romainecr.azurecr.io/ambience-agent-runner:native-runner-test",
        stage="implement",
        config_map_name="agent-config-implement",
        agent_runtime_json=snapshot(
            implementation={
                "profile_id": "impl-codex",
                "provider": "codex",
                "model": "gpt-5.5",
                "reasoning_effort": "xhigh",
                "source": "issue",
            }
        ),
    )

    pod_spec = spec["spec"]["template"]["spec"]
    assert {"ip": "10.0.0.8", "hostnames": ["github.com"]} in pod_spec["hostAliases"]
    volumes = {volume["name"]: volume for volume in pod_spec["volumes"]}
    assert volumes["github-policy-token"]["secret"]["secretName"] == "agent-github-policy-token"
    assert volumes["github-policy-ca"]["configMap"]["name"] == "glimmung-provider-api-proxy-ca"

    container = pod_spec["containers"][0]
    env = {
        item["name"]: item["value"]
        for item in container["env"]
        if "value" in item
    }
    assert env["GITHUB_CREDENTIAL_USERNAME"] == "glimmung-policy"
    assert env["GITHUB_TOKEN_FILE"] == "/var/run/ambience-github-token/token"
    assert "GH_TOKEN" not in env
    mounts = {mount["name"]: mount for mount in container["volumeMounts"]}
    assert mounts["github-policy-ca"]["mountPath"] == "/etc/github-policy-ca"


def test_verify_agent_job_receives_selected_case_context() -> None:
    script = ops._STAGE_BASH_SCRIPTS["verify"]

    assert "verification-case.json" in script
    assert "## Verification case" in script
    assert "cat /workspace/evidence/verification-case.json" in script
