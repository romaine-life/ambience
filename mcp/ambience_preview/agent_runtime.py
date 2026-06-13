from __future__ import annotations

import json
from dataclasses import dataclass
from typing import Any


STAGE_AGENT_SLOTS = {
    "test-plan": "test_plan",
    "implement": "implementation",
    "verify": "verification",
}

SUPPORTED_PROVIDERS = {"claude", "codex"}


@dataclass(frozen=True)
class ResolvedAgentRuntime:
    stage: str
    slot: str
    profile_id: str
    provider: str
    model: str
    reasoning_effort: str
    source: str


def slot_for_stage(stage: str) -> str:
    try:
        return STAGE_AGENT_SLOTS[stage]
    except KeyError as error:
        raise ValueError(
            f"unknown agent stage {stage!r}; expected one of {sorted(STAGE_AGENT_SLOTS)}"
        ) from error


def resolve_stage_runtime(raw_snapshot: str, stage: str) -> ResolvedAgentRuntime:
    slot = slot_for_stage(stage)
    snapshot = _parse_snapshot(raw_snapshot)
    profile = _profile_for_slot(snapshot, slot)
    provider = _required_string(profile, "provider", slot)
    model = _required_string(profile, "model", slot)
    profile_id = _required_string(profile, "profile_id", slot)
    if provider not in SUPPORTED_PROVIDERS:
        raise ValueError(
            f"agent runtime slot {slot!r} selected unsupported provider {provider!r}; "
            f"expected one of {sorted(SUPPORTED_PROVIDERS)}"
        )
    return ResolvedAgentRuntime(
        stage=stage,
        slot=slot,
        profile_id=profile_id,
        provider=provider,
        model=model,
        reasoning_effort=str(profile.get("reasoning_effort") or "").strip(),
        source=str(profile.get("source") or "").strip(),
    )


def _parse_snapshot(raw_snapshot: str) -> dict[str, Any]:
    if not raw_snapshot.strip():
        raise ValueError("GLIMMUNG_AGENT_RUNTIME_JSON is required for Ambience LLM stages")
    try:
        parsed = json.loads(raw_snapshot)
    except json.JSONDecodeError as error:
        raise ValueError(f"GLIMMUNG_AGENT_RUNTIME_JSON is not valid JSON: {error}") from error
    if not isinstance(parsed, dict):
        raise ValueError("GLIMMUNG_AGENT_RUNTIME_JSON must be a JSON object")
    return parsed


def _profile_for_slot(snapshot: dict[str, Any], slot: str) -> dict[str, Any]:
    slots = snapshot.get("slots") or {}
    if not isinstance(slots, dict):
        raise ValueError("agent runtime snapshot field 'slots' must be an object when present")
    raw_profile = slots.get(slot) or snapshot.get("default")
    if not isinstance(raw_profile, dict):
        raise ValueError(
            f"agent runtime snapshot has no resolved profile for slot {slot!r} "
            "and no default profile"
        )
    return raw_profile


def _required_string(profile: dict[str, Any], field: str, slot: str) -> str:
    value = str(profile.get(field) or "").strip()
    if not value:
        raise ValueError(f"agent runtime slot {slot!r} profile is missing {field!r}")
    return value
