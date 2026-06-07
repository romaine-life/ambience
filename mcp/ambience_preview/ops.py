from __future__ import annotations

import os
import re
import ssl
import subprocess
import time
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path

from .agent_runtime import resolve_stage_runtime


DEFAULT_REGISTRY_NAME = "romainecr"
DEFAULT_IMAGE_REPOSITORY = "ambience"
DEFAULT_RELEASE_NAME = "ambience-agent"
DEFAULT_PR_RELEASE_NAME = "ambience-pr"
DEFAULT_SERVICE_NAME = "ambience"
DEFAULT_HOST_SUFFIX = "ambience.dev.romaine.life"
GIT_IMAGE_TAG_PREFIX = "git-"
GIT_SHA_RE = re.compile(r"^[0-9a-f]{40}$")
MUTABLE_VALIDATION_TAG_RE = re.compile(r"^ambience-slot-\d+(?:-r\d+)?$")


class CommandError(RuntimeError):
    """Raised when an underlying command fails."""


def repo_root() -> Path:
    candidate = os.environ.get("AMBIENCE_REPO_ROOT")
    if candidate:
        return Path(candidate).resolve()
    return Path.cwd().resolve()


def chart_path() -> Path:
    return repo_root() / "chart" / "ambience"


def resolve_output_path(output_path: str) -> Path:
    path = Path(output_path)
    if path.is_absolute():
        return path
    return repo_root() / path


def repo_relative_path(path: Path) -> str:
    try:
        return path.resolve().relative_to(repo_root()).as_posix()
    except ValueError:
        return str(path.resolve())


def run_command(command: list[str], *, cwd: Path | None = None) -> str:
    result = subprocess.run(
        command,
        cwd=str(cwd or repo_root()),
        capture_output=True,
        text=True,
        check=False,
    )
    if result.returncode != 0:
        raise CommandError(
            "\n".join(
                [
                    f"Command failed: {' '.join(command)}",
                    f"exit_code={result.returncode}",
                    result.stdout.strip(),
                    result.stderr.strip(),
                ]
            ).strip()
        )
    return result.stdout.strip()


def canonical_git_revision(source_revision: str) -> str:
    revision = source_revision.strip().lower()
    if not GIT_SHA_RE.fullmatch(revision):
        raise ValueError("source_revision must be a 40-character git SHA")
    return revision


def image_tag_for_revision(source_revision: str) -> str:
    return f"{GIT_IMAGE_TAG_PREFIX}{canonical_git_revision(source_revision)}"


def reject_mutable_validation_tag(image_tag: str) -> None:
    if MUTABLE_VALIDATION_TAG_RE.fullmatch(image_tag.strip()):
        raise ValueError(
            "mutable slot-scoped validation image tags are retired; use git-<40-character-sha>"
        )


def validate_revision_image_tag(image_tag: str, source_revision: str | None) -> None:
    reject_mutable_validation_tag(image_tag)
    if source_revision is None or source_revision.strip() == "":
        return
    expected = image_tag_for_revision(source_revision)
    if image_tag != expected:
        raise ValueError(f"image_tag must be {expected} for source_revision {source_revision}")


def image_parts(image: str) -> tuple[str, str]:
    repository, tag = image.rsplit(":", 1)
    return repository, tag


def acr_repository_tag(*, registry_name: str, image_repository: str, image_tag: str) -> str:
    try:
        return run_command(
            [
                "az",
                "acr",
                "repository",
                "show-tags",
                "--name",
                registry_name,
                "--repository",
                image_repository,
                "--query",
                f"[?@=='{image_tag}'] | [0]",
                "--output",
                "tsv",
            ]
        )
    except CommandError:
        return ""


def preview_namespace(pr_number: int) -> str:
    return f"ambience-pr-{pr_number}"


def preview_tls_secret(pr_number: int) -> str:
    return f"ambience-pr-{pr_number}-tls"


def preview_host(pr_number: int, host_suffix: str = DEFAULT_HOST_SUFFIX) -> str:
    return f"pr-{pr_number}.{host_suffix}"


def preview_url(pr_number: int, host_suffix: str = DEFAULT_HOST_SUFFIX) -> str:
    return f"https://{preview_host(pr_number, host_suffix)}"


def build_preview_image(
    *,
    image_tag: str,
    source_revision: str | None = None,
    registry_name: str = DEFAULT_REGISTRY_NAME,
    image_repository: str = DEFAULT_IMAGE_REPOSITORY,
) -> dict:
    validate_revision_image_tag(image_tag, source_revision)
    registry_server = os.environ.get("REGISTRY_SERVER", f"{registry_name}.azurecr.io")
    image = f"{registry_server}/{image_repository}:{image_tag}"

    existing_tag = acr_repository_tag(
        registry_name=registry_name,
        image_repository=image_repository,
        image_tag=image_tag,
    )

    if existing_tag != image_tag:
        run_command(
            [
                "az",
                "acr",
                "build",
                "--registry",
                registry_name,
                "--image",
                f"{image_repository}:{image_tag}",
                str(repo_root()),
            ]
        )

    verified_tag = acr_repository_tag(
        registry_name=registry_name,
        image_repository=image_repository,
        image_tag=image_tag,
    )

    if verified_tag != image_tag:
        raise CommandError(f"Image tag '{image_tag}' was not found in {registry_server}/{image_repository}.")

    return {
        "image": image,
        "image_tag": image_tag,
        "image_repository": image_repository,
        "registry_name": registry_name,
        "registry_server": registry_server,
        "skipped_build": existing_tag == image_tag,
        **({"source_revision": canonical_git_revision(source_revision)} if source_revision else {}),
    }


def deploy_preview(
    *,
    namespace: str,
    image: str,
    release: str = DEFAULT_RELEASE_NAME,
    public_host: str | None = None,
    create_namespace: bool = True,
    external_dns: bool = True,
    render_mode: str | None = None,
    test_env_slot_name: str | None = None,
    tls_secret_name: str | None = None,
    timeout: str = "10m",
    rollout_timeout: str = "180s",
) -> dict:
    image_repository, image_tag = image_parts(image)
    command = [
        "helm",
        "upgrade",
        "--install",
        release,
        str(chart_path()),
        "--namespace",
        namespace,
        "--wait",
        "--timeout",
        timeout,
    ]
    if create_namespace:
        command.append("--create-namespace")

    string_values = {
        "image.repository": image_repository,
        "image.tag": image_tag,
        "image.pullPolicy": "Always",
        "edge.shutdownDrain": "1s",
        "edge.terminationGracePeriodSeconds": "3",
        "authority.terminationGracePeriodSeconds": "5",
    }
    if render_mode:
        string_values["renderMode"] = render_mode
    if test_env_slot_name:
        string_values["testEnv.slotName"] = test_env_slot_name
    bool_values = {
        "edge.replicas": True,
        "authority.replicas": True,
        "pdb.enabled": False,
    }

    if public_host:
        # Ephemeral envs share the wildcard cert + XListenerSet provisioned
        # in the ambience-dev namespace (see chart/ambience/values-dev.yaml).
        # The HTTPRoute attaches there; no per-env Certificate or
        # XListenerSet is created.
        string_values["domain.host"] = public_host
        string_values["route.attachListenerSet.name"] = "ambience-wildcard"
        string_values["route.attachListenerSet.namespace"] = "ambience-dev"
        bool_values["route.enabled"] = True
        bool_values["route.externalDns.enabled"] = external_dns
        bool_values["certificate.enabled"] = False
        bool_values["gateway.listenerSetEnabled"] = False
        bool_values["wildcardCertificate.enabled"] = False
    else:
        bool_values["route.enabled"] = False
        bool_values["certificate.enabled"] = False
        bool_values["gateway.listenerSetEnabled"] = False
        bool_values["wildcardCertificate.enabled"] = False

    for key, value in string_values.items():
        command.extend(["--set-string", f"{key}={value}"])
    for key, value in bool_values.items():
        if key.endswith(".replicas"):
            command.extend(["--set", f"{key}={1 if value else 0}"])
            continue
        command.extend(["--set", f"{key}={'true' if value else 'false'}"])

    run_command(command)
    run_command(["kubectl", "rollout", "status", f"deployment/{DEFAULT_SERVICE_NAME}-edge", "-n", namespace, f"--timeout={rollout_timeout}"])
    run_command(["kubectl", "rollout", "status", f"statefulset/{DEFAULT_SERVICE_NAME}-authority", "-n", namespace, f"--timeout={rollout_timeout}"])

    result = {
        "namespace": namespace,
        "release": release,
        "image": image,
    }
    if public_host:
        result["url"] = f"https://{public_host}"
    else:
        result["service_url"] = f"http://{DEFAULT_SERVICE_NAME}.{namespace}.svc.cluster.local"
    return result


def wait_http_ready(
    *,
    url: str,
    timeout_seconds: int = 900,
    interval_seconds: int = 5,
) -> dict:
    deadline = time.time() + timeout_seconds
    last_error = ""
    while time.time() < deadline:
        try:
            request = urllib.request.Request(url, method="GET")
            with urllib.request.urlopen(request, timeout=10, context=ssl.create_default_context()) as response:
                status = response.getcode()
                if 200 <= status < 400:
                    return {"ready": True, "status": status, "url": url}
                last_error = f"unexpected status {status}"
        except urllib.error.URLError as error:
            last_error = str(error)
        time.sleep(interval_seconds)

    raise CommandError(f"Timed out waiting for {url}: {last_error}")


def wait_public_preview(*, url: str, health_path: str = "/healthz", timeout_seconds: int = 900) -> dict:
    health_url = urllib.parse.urljoin(url.rstrip("/") + "/", health_path.lstrip("/"))
    return wait_http_ready(url=health_url, timeout_seconds=timeout_seconds)


def capture_validation_screenshot(
    *,
    namespace: str,
    page_path: str,
    output_path: str,
    wait_ms: int = 5000,
    service_name: str = DEFAULT_SERVICE_NAME,
    local_port: int = 18080,
    health_path: str = "/healthz",
) -> dict:
    final_output_path = resolve_output_path(output_path)
    final_output_path.parent.mkdir(parents=True, exist_ok=True)

    log_path = final_output_path.parent / "kubectl-port-forward.log"
    with log_path.open("w", encoding="utf-8") as log_file:
        process = subprocess.Popen(
            [
                "kubectl",
                "port-forward",
                "-n",
                namespace,
                f"service/{service_name}",
                f"{local_port}:80",
            ],
            cwd=str(repo_root()),
            stdout=log_file,
            stderr=subprocess.STDOUT,
            text=True,
        )
        try:
            wait_http_ready(url=f"http://127.0.0.1:{local_port}{health_path}", timeout_seconds=60, interval_seconds=2)
            page_url = urllib.parse.urljoin(f"http://127.0.0.1:{local_port}/", page_path.lstrip("/"))
            run_command(
                [
                    "node",
                    str(repo_root() / "scripts" / "agent" / "capture-screenshot.mjs"),
                    "--url",
                    page_url,
                    "--output",
                    str(final_output_path),
                    "--wait-ms",
                    str(wait_ms),
                    "--full-page",
                ]
            )
        finally:
            process.terminate()
            try:
                process.wait(timeout=10)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait(timeout=10)

    return {
        "namespace": namespace,
        "page_url": page_url,
        "output_path": repo_relative_path(final_output_path),
        "port_forward_log": repo_relative_path(log_path),
    }


def rebuild_validation_image(
    *,
    namespace: str,
    branch: str,
    image_tag: str,
    source_revision: str,
    registry_name: str = DEFAULT_REGISTRY_NAME,
    image_repository: str = DEFAULT_IMAGE_REPOSITORY,
    repo_slug: str = "romaine-life/ambience",
    rollout_timeout: str = "300s",
    service_name: str = DEFAULT_SERVICE_NAME,
) -> dict:
    """Build a fresh image from the pushed agent branch and roll the
    validation env onto it.

    The validation env was deployed up-front (before the agent ran) so the
    agent has a URL to reference while it works. Once the agent pushes its
    commit, that env is stale relative to the diff that's about to be
    proposed in the PR. We resolve the pushed branch to a concrete commit
    and rebuild from that immutable ref (`az acr build https://...#<sha>`)
    instead of re-cloning locally, then `kubectl set image` flips the
    workloads onto the new tag without redeploying the chart (avoids
    re-running route/cert plumbing)."""
    revision = canonical_git_revision(source_revision)
    validate_revision_image_tag(image_tag, revision)
    registry_server = os.environ.get("REGISTRY_SERVER", f"{registry_name}.azurecr.io")
    image = f"{registry_server}/{image_repository}:{image_tag}"

    existing_tag = acr_repository_tag(
        registry_name=registry_name,
        image_repository=image_repository,
        image_tag=image_tag,
    )

    if existing_tag != image_tag:
        run_command(
            [
                "az",
                "acr",
                "build",
                "--registry",
                registry_name,
                "--image",
                f"{image_repository}:{image_tag}",
                f"https://github.com/{repo_slug}.git#{revision}",
            ]
        )

    for kind, workload in (
        ("deployment", f"{service_name}-edge"),
        ("statefulset", f"{service_name}-authority"),
    ):
        run_command(
            [
                "kubectl",
                "-n",
                namespace,
                "set",
                "image",
                f"{kind}/{workload}",
                f"{service_name}={image}",
            ]
        )
        run_command(
            [
                "kubectl",
                "-n",
                namespace,
                "rollout",
                "status",
                f"{kind}/{workload}",
                f"--timeout={rollout_timeout}",
            ]
        )

    return {
        "namespace": namespace,
        "branch": branch,
        "source_revision": revision,
        "image": image,
        "image_tag": image_tag,
        "skipped_build": existing_tag == image_tag,
    }


def upsert_pr_preview(
    *,
    pr_number: int,
    image: str,
    release: str = DEFAULT_PR_RELEASE_NAME,
    host_suffix: str = DEFAULT_HOST_SUFFIX,
) -> dict:
    host = preview_host(pr_number, host_suffix)
    return deploy_preview(
        namespace=preview_namespace(pr_number),
        image=image,
        release=release,
        public_host=host,
        tls_secret_name=preview_tls_secret(pr_number),
        rollout_timeout="300s",
    )


def destroy_preview(*, namespace: str, release: str) -> dict:
    subprocess.run(
        ["helm", "uninstall", release, "--namespace", namespace],
        cwd=str(repo_root()),
        capture_output=True,
        text=True,
        check=False,
    )
    subprocess.run(
        ["kubectl", "delete", "namespace", namespace, "--ignore-not-found=true", "--wait=false"],
        cwd=str(repo_root()),
        capture_output=True,
        text=True,
        check=False,
    )
    return {"namespace": namespace, "release": release, "destroyed": True}


def destroy_pr_preview(*, pr_number: int, release: str = DEFAULT_PR_RELEASE_NAME) -> dict:
    return destroy_preview(namespace=preview_namespace(pr_number), release=release)


# ---------------------------------------------------------------------------
# Agent Job orchestration. Replaces the previous envsubst-templated YAML
# approach, which silently over-substituted shell vars on the GHA runner's
# gettext build. Building the Job spec as a Python dict gives us:
#   - run-specific values as named function arguments (no string templating)
#   - bash-side `${VAR}` refs in _AGENT_BASH_SCRIPT stay literal because
#     this file is plain Python, not an f-string
#   - status-field polling that fast-fails on either succeeded or failed,
#     instead of waiting for a `Complete` condition that never fires for
#     failed Jobs (the old `kubectl wait --for=condition=complete --timeout=30m`
#     blocked for the full timeout on every failure).
# ---------------------------------------------------------------------------

# Bash fragments that run inside the agent container. Plain Python strings —
# no Python-side interpolation. All `${VAR}` references are evaluated by the
# container's bash from the env block defined in apply_agent_job below
# (REPO_SLUG, GITHUB_TOKEN_FILE, ISSUE_NUMBER, BRANCH_NAME, etc).
#
# The flow is split across K8s Job pods to reduce per-step LLM context
# burden (see docs/issue-agent-stage-split.md and tank-operator's
# docs/agent-llm-task-splitting.md):
#
#   issue-contract — canonicalizes the issue target before test-plan and
#   implement run. It does not plan evidence or edit code.
#
#   test-plan / implement — independent sibling jobs. Both read the issue
#   contract, but neither reads the other's artifact.
#
#   verify — one `claude --print` invocation that reads the
#   prior stages' JSON+MD handoff artifacts (mounted via configmap) and
#   captures evidence against the rebuilt validation env. It does NOT
#   touch git.
#
# Both share the same bootstrap (claude state seed + git auth header).

_AGENT_BOOTSTRAP_BASH = r"""set -euo pipefail

mkdir -p /workspace/evidence/screenshots /workspace/evidence/videos
CA_FILES="/etc/ssl/certs/ca-certificates.crt"
if [ -f /etc/claude-ca/ca.crt ]; then
  CA_FILES="${CA_FILES} /etc/claude-ca/ca.crt"
fi
if [ -f /etc/github-policy-ca/ca.crt ]; then
  CA_FILES="${CA_FILES} /etc/github-policy-ca/ca.crt"
fi
if [ -f /etc/ssl/certs/ca-certificates.crt ]; then
  cat ${CA_FILES} > /workspace/provider-ca-bundle.crt
  export SSL_CERT_FILE=/workspace/provider-ca-bundle.crt
  export REQUESTS_CA_BUNDLE=/workspace/provider-ca-bundle.crt
  export GIT_SSL_CAINFO=/workspace/provider-ca-bundle.crt
fi
mkdir -p $HOME/.codex
cat > $HOME/.codex/auth.json <<'EOF'
{
  "auth_mode": "chatgptAuthTokens",
  "tokens": {
    "id_token": "eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0.eyJlbWFpbCI6ImdsaW1tdW5nQGxvY2FsIiwiZXhwIjo0MTAyNDQ0ODAwLCJodHRwczovL2FwaS5vcGVuYWkuY29tL2F1dGgiOnsiY2hhdGdwdF9wbGFuX3R5cGUiOiJwcm8iLCJjaGF0Z3B0X3VzZXJfaWQiOiJtYW5hZ2VkLWJ5LWdsaW1tdW5nIiwiY2hhdGdwdF9hY2NvdW50X2lkIjoibWFuYWdlZC1ieS1nbGltbXVuZyJ9fQ.signature",
    "access_token": "managed-by-glimmung",
    "refresh_token": "",
    "account_id": "managed-by-glimmung"
  },
  "last_refresh": "2099-01-01T00:00:00Z"
}
EOF
chmod 600 $HOME/.codex/auth.json
cat > $HOME/.codex/config.toml <<'EOF'
cli_auth_credentials_store = "file"
EOF

# Seed claude state — placeholder credentials so claude never tries to
# refresh, project trust + onboarding flags so it boots straight into the run.
mkdir -p $HOME/.claude
cat > $HOME/.claude/.credentials.json <<'EOF'
{
  "claudeAiOauth": {
    "accessToken": "managed-by-tank-operator",
    "refreshToken": "managed-by-tank-operator",
    "expiresAt": 9999999999000,
    "scopes": ["user:inference", "user:profile"],
    "subscriptionType": "max",
    "rateLimitTier": "max"
  }
}
EOF
chmod 600 $HOME/.claude/.credentials.json
cat > $HOME/.claude/settings.json <<'EOF'
{"theme":"dark","permissions":{"defaultMode":"bypassPermissions"},"skipDangerousModePermissionPrompt":true}
EOF
cat > $HOME/.claude.json <<'EOF'
{
  "hasCompletedOnboarding": true,
  "officialMarketplaceAutoInstallAttempted": true,
  "officialMarketplaceAutoInstalled": true,
  "projects": {
    "/workspace/repo": {
      "allowedTools": [],
      "hasTrustDialogAccepted": true,
      "projectOnboardingSeenCount": 1
    }
  }
}
EOF

git config --global user.name "ambience-agent[bot]"
git config --global user.email "ambience-agent@romaine.life"

GITHUB_CREDENTIAL_USERNAME="${GITHUB_CREDENTIAL_USERNAME:-x-access-token}"
if [ -z "${GITHUB_TOKEN_FILE:-}" ] || [ ! -f "${GITHUB_TOKEN_FILE}" ]; then
  echo "GITHUB_TOKEN_FILE is required for GitHub git access" >&2
  exit 2
fi
git config --global credential.https://github.com.username "${GITHUB_CREDENTIAL_USERNAME}"
git config --global credential.https://github.com.helper \
  "!f() { if [ \"\$1\" = get ]; then echo username=${GITHUB_CREDENTIAL_USERNAME}; printf 'password='; cat ${GITHUB_TOKEN_FILE}; echo; fi; }; f"

issue_heading="# Glimmung issue ${ISSUE_REFERENCE}: ${ISSUE_TITLE}"
close_line="Glimmung issue: ${ISSUE_REFERENCE}"

cat > /tmp/issue-context.md <<EOF
${issue_heading}
URL: ${ISSUE_URL}
Validation env: ${VALIDATION_URL}
EOF
if [ -n "${GLIMMUNG_EVIDENCE_REQUIREMENTS_JSON:-}" ]; then
  {
    echo ""
    echo "## Run evidence requirements"
    echo ""
    echo '```json'
    printf '%s\n' "${GLIMMUNG_EVIDENCE_REQUIREMENTS_JSON}"
    echo '```'
  } >> /tmp/issue-context.md
fi

run_agent_prompt() {
  input_file="$1"
  log_file="$2"
  echo "=== agent runtime: stage=${AGENT_STAGE} slot=${AGENT_RUNTIME_SLOT} profile=${AGENT_RUNTIME_PROFILE_ID} provider=${AGENT_PROVIDER} model=${AGENT_MODEL} reasoning=${AGENT_REASONING_EFFORT:-} source=${AGENT_RUNTIME_SOURCE:-} ==="
  case "${AGENT_PROVIDER}" in
    claude)
      cat "${input_file}" | claude \
        --print \
        --model "${AGENT_MODEL}" \
        --output-format stream-json \
        --verbose \
        --dangerously-skip-permissions \
        2>&1 | tee "${log_file}"
      ;;
    codex)
      args=(
        exec
        --cd /workspace/repo
        --model "${AGENT_MODEL}"
        --dangerously-bypass-approvals-and-sandbox
        --json
        --output-last-message /tmp/ambience-codex-last-message.md
        -
      )
      if [ -n "${AGENT_REASONING_EFFORT:-}" ]; then
        args=(exec -c "model_reasoning_effort=\"${AGENT_REASONING_EFFORT}\"" "${args[@]:1}")
      fi
      cat "${input_file}" | codex "${args[@]}" 2>&1 | tee "${log_file}"
      ;;
    *)
      echo "unsupported agent provider: ${AGENT_PROVIDER}" >&2
      return 2
      ;;
  esac
}
"""

# Common tail used by both pod scripts. Stages have already written their
# JSON+MD into /workspace/evidence/; the tar is the side-channel back to
# the wrapper (kubectl logs) since exec-tar fails on Succeeded pods.
_AGENT_EVIDENCE_TAIL_BASH = r"""
# Stream the evidence dir as a base64-encoded tarball to stdout between
# clear markers the workflow extracts via sed. We can't kubectl cp from
# a Succeeded pod (exec-tar requires a live container; the bash exit
# transitions the pod to Succeeded immediately), so we use kubectl logs
# as the side-channel — it preserves stdout regardless of pod state.
#
# Two failure modes we observed in earlier runs and now defend against:
#
#   1. Stderr from tar/gzip (e.g. "file changed as we read it" warnings,
#      or harmless info messages) was interleaving with the base64
#      stdout, corrupting the byte stream the workflow then tries to
#      decode. Fix: write the tarball to a temp file first, with stderr
#      fully redirected, then base64-encode the file in a separate step.
#
#   2. Lingering writers from the agent's own bash subprocesses (servers
#      it spawned, playwright finalizing evidence writes, etc.) were
#      still emitting stdout when the EVIDENCE-TAR markers were printed,
#      so other text could land between them. Fix: `wait` for any bash
#      jobs and `sync` the filesystem before running tar.
#
# Empty evidence dir is fine; the markers just frame an empty tar.
if [ -d /workspace/evidence ]; then
  wait 2>/dev/null || true
  sync || true

  if ! tar -czf /tmp/evidence.tgz -C /workspace/evidence . 2>/tmp/tar.err; then
    echo "evidence tar FAILED; stderr was:" >&2
    cat /tmp/tar.err >&2 || true
  fi

  {
    echo "===EVIDENCE-TAR-START==="
    base64 < /tmp/evidence.tgz
    echo "===EVIDENCE-TAR-END==="
  } 2>/dev/null
fi
"""

# Pod: issue-contract. Read-only target canonicalization — no code edits,
# no evidence planning, no git mutations.
_ISSUE_CONTRACT_BASH = _AGENT_BOOTSTRAP_BASH + r"""
git clone "https://github.com/${REPO_SLUG}.git" /workspace/repo
cd /workspace/repo

echo "=== prewarm: go mod download ==="
go mod download 2>&1 | tail -20 || true

echo "=== STAGE: issue-contract ==="
cat /agent-config/prompt-issue-contract.md /tmp/issue-context.md > /tmp/contract-input.md
run_agent_prompt /tmp/contract-input.md /tmp/issue-contract-stream.log

if [ ! -f /workspace/evidence/issue-agent-contract.json ]; then
  echo "issue-contract stage exited without writing issue-agent-contract.json" >&2
  exit 1
fi
contract_status="$(jq -r '.status // "missing"' /workspace/evidence/issue-agent-contract.json)"
if [ "$contract_status" != "pass" ]; then
  echo "issue-contract aborted: status=${contract_status} reason=$(jq -r '.abort_reason // ""' /workspace/evidence/issue-agent-contract.json)" >&2
  exit 1
fi

# Issue-contract stage may not modify the working tree.
if [ -n "$(git status --porcelain)" ]; then
  echo "issue-contract stage modified files (forbidden):" >&2
  git status --short >&2
  exit 1
fi
""" + _AGENT_EVIDENCE_TAIL_BASH

# Pod: test-plan. Read-only analysis — no code edits, no git mutations.
# Clones main, runs the test-plan LLM, emits issue-agent-test-plan.json.
_TEST_PLAN_BASH = _AGENT_BOOTSTRAP_BASH + r"""
git clone "https://github.com/${REPO_SLUG}.git" /workspace/repo
cd /workspace/repo

echo "=== prewarm: go mod download ==="
go mod download 2>&1 | tail -20 || true

echo "=== STAGE: test-plan ==="
{
  cat /agent-config/prompt-test-plan.md
  cat /tmp/issue-context.md
  if [ -f /agent-config/issue-agent-contract.json ]; then
    echo ""
    echo "## Issue contract"
    echo ""
    echo '```json'
    cat /agent-config/issue-agent-contract.json
    echo '```'
  fi
  if [ -f /agent-config/issue-agent-contract.md ]; then
    echo ""
    cat /agent-config/issue-agent-contract.md
  fi
} > /tmp/plan-input.md
run_agent_prompt /tmp/plan-input.md /tmp/test-plan-stream.log

if [ ! -f /workspace/evidence/issue-agent-test-plan.json ]; then
  echo "test-plan stage exited without writing issue-agent-test-plan.json" >&2
  exit 1
fi
plan_status="$(jq -r '.status // "missing"' /workspace/evidence/issue-agent-test-plan.json)"
if [ "$plan_status" != "pass" ]; then
  echo "test-plan aborted: status=${plan_status} reason=$(jq -r '.abort_reason // ""' /workspace/evidence/issue-agent-test-plan.json)" >&2
  exit 1
fi

# Test-plan stage may not modify the working tree.
if [ -n "$(git status --porcelain)" ]; then
  echo "test-plan stage modified files (forbidden):" >&2
  git status --short >&2
  exit 1
fi
""" + _AGENT_EVIDENCE_TAIL_BASH

# Pod: implement. Works directly from the issue spec — does NOT receive the
# test plan as input (runs in parallel with test-plan). Commits + pushes the
# implementation branch.
_IMPLEMENT_BASH = _AGENT_BOOTSTRAP_BASH + r"""
git clone "https://github.com/${REPO_SLUG}.git" /workspace/repo
cd /workspace/repo
if git show-ref --verify --quiet "refs/remotes/origin/${BRANCH_NAME}"; then
  git checkout -B "${BRANCH_NAME}" "origin/${BRANCH_NAME}"
else
  git checkout -B "${BRANCH_NAME}"
fi

echo "=== prewarm: go mod download ==="
go mod download 2>&1 | tail -20 || true

echo "=== STAGE: implementation ==="
{
  cat /agent-config/prompt-implementation.md
  cat /tmp/issue-context.md
  if [ -f /agent-config/issue-agent-contract.json ]; then
    echo ""
    echo "## Issue contract"
    echo ""
    echo '```json'
    cat /agent-config/issue-agent-contract.json
    echo '```'
  fi
  if [ -f /agent-config/issue-agent-contract.md ]; then
    echo ""
    cat /agent-config/issue-agent-contract.md
  fi
} > /tmp/impl-input.md
run_agent_prompt /tmp/impl-input.md /tmp/impl-stream.log

if [ ! -f /workspace/evidence/issue-agent-implementation.json ]; then
  echo "implementation stage exited without writing issue-agent-implementation.json" >&2
  exit 1
fi
impl_status="$(jq -r '.status // "missing"' /workspace/evidence/issue-agent-implementation.json)"
if [ "$impl_status" != "pass" ]; then
  echo "implementation aborted: status=${impl_status} reason=$(jq -r '.abort_reason // ""' /workspace/evidence/issue-agent-implementation.json)" >&2
  exit 1
fi

# Refuse to publish runner-local config files, whether committed or still in the worktree.
git fetch origin main
BLOCKED="$(
  {
    git diff --name-only origin/main...HEAD -- .github/workflows .github/agent .mcp.json 2>/dev/null || true
    git status --porcelain -- .github/workflows .github/agent .mcp.json 2>/dev/null | awk '{print $2}' || true
  } | sort -u
)"
if [ -n "$BLOCKED" ]; then
  echo "implementation stage modified runner-local config files (forbidden):" >&2
  echo "$BLOCKED" >&2
  exit 1
fi

if [ -n "$(git status --porcelain)" ]; then
  git add -A
  if ! git diff --cached --quiet; then
    git commit -m "agent: address ${ISSUE_REFERENCE}

${ISSUE_TITLE}

${close_line}"
  fi
fi

# Sync onto current main before pushing.
git rebase origin/main
if git diff --quiet origin/main...HEAD; then
  echo "implementation stage produced no branch changes; failing job so the workflow doesn't open an empty PR" >&2
  exit 1
fi

git push origin "HEAD:${BRANCH_NAME}"
""" + _AGENT_EVIDENCE_TAIL_BASH

# Pod: verify. Reads test-plan + implementation handoff artifacts from the
# agent-config configmap (wrapper populates it from the two upstream phase
# outputs before launching this pod).
_VERIFY_BASH = _AGENT_BOOTSTRAP_BASH + r"""
# Verify starts from the pushed implementation branch.
git clone --branch "${BRANCH_NAME}" \
  "https://github.com/${REPO_SLUG}.git" /workspace/repo \
  || git clone "https://github.com/${REPO_SLUG}.git" /workspace/repo
cd /workspace/repo
if git show-ref --verify --quiet "refs/remotes/origin/${BRANCH_NAME}"; then
  git checkout -B "${BRANCH_NAME}" "origin/${BRANCH_NAME}"
fi

echo "=== prewarm: go mod download ==="
go mod download 2>&1 | tail -20 || true

# Make prior-phase handoff artifacts visible under /workspace/evidence/.
for f in issue-agent-contract.json issue-agent-contract.md \
         issue-agent-test-plan.json issue-agent-test-plan.md \
         issue-agent-implementation.json issue-agent-implementation.md \
         verification-case.json; do
  if [ -f "/agent-config/${f}" ]; then
    cp "/agent-config/${f}" "/workspace/evidence/${f}"
  fi
done

echo "=== STAGE: verification ==="
{
  cat /agent-config/prompt-verification.md
  cat /tmp/issue-context.md
  echo ""
  echo "## Issue contract"
  echo ""
  echo '```json'
  cat /workspace/evidence/issue-agent-contract.json 2>/dev/null || echo '{}'
  echo '```'
  echo ""
  cat /workspace/evidence/issue-agent-contract.md 2>/dev/null || true
  echo ""
  echo "## Verification case"
  echo ""
  echo '```json'
  cat /workspace/evidence/verification-case.json 2>/dev/null || echo '{}'
  echo '```'
  echo ""
  echo "## Test plan"
  echo ""
  echo '```json'
  cat /workspace/evidence/issue-agent-test-plan.json 2>/dev/null || echo '{}'
  echo '```'
  echo ""
  cat /workspace/evidence/issue-agent-test-plan.md 2>/dev/null || true
  echo ""
  echo "## Implementation result"
  echo ""
  echo '```json'
  cat /workspace/evidence/issue-agent-implementation.json 2>/dev/null || echo '{}'
  echo '```'
  echo ""
  cat /workspace/evidence/issue-agent-implementation.md 2>/dev/null || true
} > /tmp/verify-input.md

run_agent_prompt /tmp/verify-input.md /tmp/verify-stream.log

if [ ! -f /workspace/evidence/issue-agent-verification.json ]; then
  echo "verification stage exited without writing issue-agent-verification.json" >&2
  exit 1
fi

# Verification stage may not modify the working tree.
if [ -n "$(git status --porcelain)" ]; then
  echo "verification stage modified files (forbidden):" >&2
  git status --short >&2
  exit 1
fi
""" + _AGENT_EVIDENCE_TAIL_BASH

_STAGE_BASH_SCRIPTS = {
    "issue-contract": _ISSUE_CONTRACT_BASH,
    "test-plan": _TEST_PLAN_BASH,
    "implement": _IMPLEMENT_BASH,
    "verify": _VERIFY_BASH,
}


def _agent_job_spec(
    *,
    namespace: str,
    job_name: str,
    issue_number: str,
    issue_title: str,
    issue_url: str,
    issue_reference: str | None,
    validation_url: str,
    branch_name: str,
    proxy_ip: str,
    agent_container_tag: str,
    claude_proxy_ip: str | None = None,
    codex_proxy_ip: str | None = None,
    github_proxy_ip: str | None = None,
    agent_container_image: str | None = None,
    repo_slug: str = "romaine-life/ambience",
    stage: str = "test-plan",
    config_map_name: str = "agent-config",
    agent_runtime_json: str = "",
) -> dict:
    """Build the Job spec as a Python dict. No templating; values land directly
    in their typed positions. `stage` selects which bash script the agent
    container runs (see _STAGE_BASH_SCRIPTS). `config_map_name` allows
    parallel phases to use distinct configmaps in the same namespace."""
    if stage not in _STAGE_BASH_SCRIPTS:
        raise ValueError(
            f"unknown stage {stage!r}; expected one of {sorted(_STAGE_BASH_SCRIPTS)}"
        )
    runtime = resolve_stage_runtime(agent_runtime_json, stage)
    issue_ref = issue_reference or (f"#{issue_number}" if issue_number else "glimmung-run")
    image = (agent_container_image or "").strip()
    claude_proxy_ip = (claude_proxy_ip or proxy_ip).strip()
    codex_proxy_ip = (codex_proxy_ip or proxy_ip).strip()
    github_proxy_ip = (github_proxy_ip or "").strip()
    if not image:
        tag = agent_container_tag.strip()
        if not tag:
            raise ValueError("agent_container_image or agent_container_tag is required")
        image = f"romainecr.azurecr.io/ambience-agent-runner:{tag}"

    host_aliases = [
        {"ip": claude_proxy_ip, "hostnames": ["api.anthropic.com"]},
        {"ip": codex_proxy_ip, "hostnames": ["chatgpt.com", "api.openai.com"]},
    ]
    volumes = [
        {
            "name": "claude-ca",
            "configMap": {
                "name": "claude-oauth-ca",
                "items": [{"key": "ca.crt", "path": "ca.crt"}],
            },
        },
        {"name": "workspace", "emptyDir": {}},
        {
            "name": "agent-config",
            "configMap": {"name": config_map_name},
        },
    ]
    volume_mounts = [
        {"name": "claude-ca", "mountPath": "/etc/claude-ca", "readOnly": True},
        {"name": "workspace", "mountPath": "/workspace"},
        {"name": "agent-config", "mountPath": "/agent-config", "readOnly": True},
    ]
    env = [
        {"name": "NODE_EXTRA_CA_CERTS", "value": "/etc/claude-ca/ca.crt"},
        {"name": "HOME", "value": "/workspace"},
        {"name": "ISSUE_NUMBER", "value": str(issue_number)},
        {"name": "ISSUE_REFERENCE", "value": issue_ref},
        {"name": "ISSUE_TITLE", "value": issue_title},
        {"name": "ISSUE_URL", "value": issue_url},
        {"name": "VALIDATION_URL", "value": validation_url},
        {"name": "BRANCH_NAME", "value": branch_name},
        {"name": "REPO_SLUG", "value": repo_slug},
        {"name": "AGENT_STAGE", "value": stage},
        {"name": "AGENT_RUNTIME_SLOT", "value": runtime.slot},
        {"name": "AGENT_RUNTIME_PROFILE_ID", "value": runtime.profile_id},
        {"name": "AGENT_PROVIDER", "value": runtime.provider},
        {"name": "AGENT_MODEL", "value": runtime.model},
        {"name": "AGENT_REASONING_EFFORT", "value": runtime.reasoning_effort},
        {"name": "AGENT_RUNTIME_SOURCE", "value": runtime.source},
        {
            "name": "GLIMMUNG_EVIDENCE_REQUIREMENTS_JSON",
            "value": os.environ.get("GLIMMUNG_EVIDENCE_REQUIREMENTS_JSON", ""),
        },
        {"name": "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC", "value": "1"},
    ]
    if github_proxy_ip:
        host_aliases.append({"ip": github_proxy_ip, "hostnames": ["github.com"]})
        volumes.extend(
            [
                {
                    "name": "github-policy-token",
                    "secret": {
                        "secretName": "agent-github-policy-token",
                        "items": [{"key": "token", "path": "token"}],
                    },
                },
                {
                    "name": "github-policy-ca",
                    "configMap": {
                        "name": "glimmung-provider-api-proxy-ca",
                        "items": [{"key": "ca.crt", "path": "ca.crt"}],
                    },
                },
            ]
        )
        volume_mounts.extend(
            [
                {
                    "name": "github-policy-token",
                    "mountPath": "/var/run/ambience-github-token",
                    "readOnly": True,
                },
                {"name": "github-policy-ca", "mountPath": "/etc/github-policy-ca", "readOnly": True},
            ]
        )
        env.extend(
            [
                {"name": "GITHUB_TOKEN_FILE", "value": "/var/run/ambience-github-token/token"},
                {"name": "GITHUB_CREDENTIAL_USERNAME", "value": "glimmung-policy"},
            ]
        )
    else:
        volumes.append(
            {
                "name": "github-token",
                "secret": {
                    "secretName": "agent-github-token",
                    "items": [{"key": "token", "path": "token"}],
                },
            }
        )
        volume_mounts.append(
            {"name": "github-token", "mountPath": "/var/run/ambience-github-token", "readOnly": True}
        )
        env.extend(
            [
                {"name": "GITHUB_TOKEN_FILE", "value": "/var/run/ambience-github-token/token"},
                {"name": "GITHUB_CREDENTIAL_USERNAME", "value": "x-access-token"},
            ]
        )

    return {
        "apiVersion": "batch/v1",
        "kind": "Job",
        "metadata": {
            "name": job_name,
            "namespace": namespace,
            "labels": {
                "app.kubernetes.io/name": "ambience-agent",
                "app.kubernetes.io/managed-by": "glimmung-inner",
                "ambience.io/issue": str(issue_number),
            },
        },
        "spec": {
            "backoffLimit": 0,
            "ttlSecondsAfterFinished": 1800,
            "template": {
                "metadata": {
                    "labels": {
                        "app.kubernetes.io/name": "ambience-agent",
                        "app.kubernetes.io/managed-by": "glimmung-inner",
                        "ambience.io/issue": str(issue_number),
                    },
                },
                "spec": {
                    "restartPolicy": "Never",
                    # claude --dangerously-skip-permissions refuses to run as root
                    # (matches tank-operator session pods' securityContext).
                    "securityContext": {
                        "runAsUser": 1000,
                        "runAsGroup": 1000,
                        "fsGroup": 1000,
                        "runAsNonRoot": True,
                    },
                    "hostAliases": host_aliases,
                    "volumes": volumes,
                    "containers": [
                        {
                            "name": "agent",
                            # The Ambience native runner image is also the inner
                            # issue-agent image: it carries Playwright, Claude,
                            # Codex, kubectl, Helm, and the staged MCP package.
                            "image": image,
                            "imagePullPolicy": "IfNotPresent",
                            "command": ["/bin/bash", "-c", _STAGE_BASH_SCRIPTS[stage]],
                            "env": env,
                            "volumeMounts": volume_mounts,
                        },
                    ],
                },
            },
        },
    }


def apply_agent_job(
    *,
    namespace: str,
    job_name: str,
    issue_number: str,
    issue_title: str,
    issue_url: str,
    validation_url: str,
    branch_name: str,
    proxy_ip: str,
    agent_container_tag: str,
    claude_proxy_ip: str | None = None,
    codex_proxy_ip: str | None = None,
    github_proxy_ip: str | None = None,
    agent_container_image: str | None = None,
    repo_slug: str = "romaine-life/ambience",
    issue_reference: str | None = None,
    stage: str = "test-plan",
    config_map_name: str = "agent-config",
    agent_runtime_json: str = "",
) -> dict:
    """Render the agent Job spec and `kubectl apply -f -` it. `stage` selects
    the bash script (test-plan / implement / verify). `config_map_name` is the
    configmap mounted at /agent-config — use a per-stage name when phases run
    in parallel so they don't clobber each other."""
    import json as _json
    spec = _agent_job_spec(
        namespace=namespace,
        job_name=job_name,
        issue_number=issue_number,
        issue_title=issue_title,
        issue_url=issue_url,
        issue_reference=issue_reference,
        validation_url=validation_url,
        branch_name=branch_name,
        proxy_ip=proxy_ip,
        claude_proxy_ip=claude_proxy_ip,
        codex_proxy_ip=codex_proxy_ip,
        github_proxy_ip=github_proxy_ip,
        agent_container_tag=agent_container_tag,
        agent_container_image=agent_container_image,
        repo_slug=repo_slug,
        stage=stage,
        config_map_name=config_map_name,
        agent_runtime_json=agent_runtime_json,
    )
    proc = subprocess.run(
        ["kubectl", "apply", "-f", "-"],
        input=_json.dumps(spec),
        capture_output=True,
        text=True,
        check=False,
    )
    if proc.returncode != 0:
        raise CommandError(
            f"kubectl apply failed: {(proc.stderr or proc.stdout).strip()}"
        )
    return {
        "namespace": namespace,
        "job": job_name,
        "agent_runtime": _agent_runtime_result(spec),
        "applied": proc.stdout.strip(),
    }


def _agent_runtime_result(spec: dict) -> dict:
    env = {
        item.get("name"): item.get("value")
        for item in spec["spec"]["template"]["spec"]["containers"][0]["env"]
        if isinstance(item, dict) and "value" in item
    }
    return {
        "stage": env.get("AGENT_STAGE", ""),
        "slot": env.get("AGENT_RUNTIME_SLOT", ""),
        "profile_id": env.get("AGENT_RUNTIME_PROFILE_ID", ""),
        "provider": env.get("AGENT_PROVIDER", ""),
        "model": env.get("AGENT_MODEL", ""),
        "reasoning_effort": env.get("AGENT_REASONING_EFFORT", ""),
        "source": env.get("AGENT_RUNTIME_SOURCE", ""),
    }


def wait_agent_job(
    *,
    namespace: str,
    job_name: str,
    timeout_seconds: int = 1800,
    poll_interval_seconds: int = 3,
) -> dict:
    """Wait for an agent Job to reach a Job-level terminal condition.

    Authoritative signal: the Job's status.conditions[].type == Complete
    (or Failed) — set atomically by the Job controller. We do NOT rely on
    `.status.succeeded` / `.status.failed` counts as the sole signal: those
    can be 0/0 transiently after pod termination and a previous version of
    this function misread that window as a job failure (ambience#170 root
    cause). Pod phase Succeeded is honoured as a final fallback for the
    case where the controller is slow to stamp the condition.

    Log streaming is a UX optimisation, not a correctness signal. We run
    `kubectl logs -f` as a background subprocess so that an apiserver-side
    stream hang (which has been observed at >29 min) cannot block the
    wait. The streamer is unconditionally torn down on exit.

    Raises CommandError on:
      - pod never appeared before the deadline,
      - Job condition Failed=True,
      - deadline reached without a terminal condition,
      - pod phase Failed without a Job-level condition (defensive).

    Returns on success."""
    deadline = time.time() + timeout_seconds

    pod_name = _wait_for_agent_pod(
        namespace=namespace,
        job_name=job_name,
        deadline=deadline,
        poll_interval_seconds=poll_interval_seconds,
    )

    print(f"agent pod {pod_name} — streaming logs", flush=True)
    log_proc = subprocess.Popen(
        ["kubectl", "-n", namespace, "logs", "-f", pod_name],
    )
    try:
        terminal_phase = _wait_for_agent_terminal(
            namespace=namespace,
            job_name=job_name,
            pod_name=pod_name,
            deadline=deadline,
            poll_interval_seconds=poll_interval_seconds,
        )
    finally:
        _tear_down_streamer(log_proc)

    return {
        "namespace": namespace,
        "job": job_name,
        "pod": pod_name,
        "pod_phase": terminal_phase,
    }


def _wait_for_agent_pod(
    *,
    namespace: str,
    job_name: str,
    deadline: float,
    poll_interval_seconds: int,
) -> str:
    """Poll until the agent Job's pod appears and reaches a non-Pending
    phase. Returns the pod name. Raises if the deadline expires before
    the pod can be observed."""
    pod_name = ""
    phase = ""
    while time.time() < deadline:
        pod_name = run_command(
            [
                "kubectl",
                "-n",
                namespace,
                "get",
                "pods",
                "-l",
                f"job-name={job_name}",
                "-o",
                "jsonpath={.items[0].metadata.name}",
            ],
        )
        if pod_name:
            phase = run_command(
                [
                    "kubectl",
                    "-n",
                    namespace,
                    "get",
                    "pod",
                    pod_name,
                    "-o",
                    "jsonpath={.status.phase}",
                ],
            )
            if phase in ("Running", "Succeeded", "Failed"):
                return pod_name
        time.sleep(poll_interval_seconds)
    if not pod_name:
        raise CommandError(f"agent pod for Job {job_name!r} never appeared")
    raise CommandError(
        f"agent pod {pod_name} for Job {job_name!r} stayed in phase={phase!r}"
        f" past timeout"
    )


def _wait_for_agent_terminal(
    *,
    namespace: str,
    job_name: str,
    pod_name: str,
    deadline: float,
    poll_interval_seconds: int,
) -> str:
    """Poll Job conditions and pod phase until a terminal state is
    observed. Returns the pod's final phase on success ("Succeeded").
    Raises CommandError on Failed condition / Failed pod / deadline."""
    while time.time() < deadline:
        # Job conditions are the authoritative completion signal. We
        # query both Complete and Failed in the same poll so a race
        # between them is impossible from our side.
        complete = run_command(
            [
                "kubectl",
                "-n",
                namespace,
                "get",
                "job",
                job_name,
                "-o",
                'jsonpath={.status.conditions[?(@.type=="Complete")].status}',
            ],
        )
        if complete == "True":
            return _final_pod_phase(namespace=namespace, pod_name=pod_name)
        failed_cond = run_command(
            [
                "kubectl",
                "-n",
                namespace,
                "get",
                "job",
                job_name,
                "-o",
                'jsonpath={.status.conditions[?(@.type=="Failed")].status}',
            ],
        )
        if failed_cond == "True":
            reason = run_command(
                [
                    "kubectl",
                    "-n",
                    namespace,
                    "get",
                    "job",
                    job_name,
                    "-o",
                    'jsonpath={.status.conditions[?(@.type=="Failed")].reason}',
                ],
            )
            raise CommandError(
                f"agent Job {job_name!r} condition Failed=True reason={reason!r}"
            )
        # Fallback: if the Job controller is slow stamping a condition
        # but the pod itself has reached terminal phase, honour that.
        # With the Job spec we ship (backoffLimit=0), pod terminal phase
        # determines Job outcome unambiguously.
        pod_phase = run_command(
            [
                "kubectl",
                "-n",
                namespace,
                "get",
                "pod",
                pod_name,
                "-o",
                "jsonpath={.status.phase}",
            ],
        )
        if pod_phase == "Succeeded":
            return pod_phase
        if pod_phase == "Failed":
            raise CommandError(
                f"agent pod {pod_name} phase=Failed without Job-level Failed condition"
            )
        time.sleep(poll_interval_seconds)
    raise CommandError(
        f"agent Job {job_name!r} did not reach a terminal Job condition"
        f" before timeout"
    )


def _final_pod_phase(*, namespace: str, pod_name: str) -> str:
    """Best-effort read of the pod's final phase after Job Complete.
    Returns "Succeeded" as the optimistic default if the pod is gone
    (TTL or upstream cleanup) since Complete=True already implies the
    pod ran to success."""
    try:
        phase = run_command(
            [
                "kubectl",
                "-n",
                namespace,
                "get",
                "pod",
                pod_name,
                "-o",
                "jsonpath={.status.phase}",
            ],
        )
        return phase or "Succeeded"
    except CommandError:
        return "Succeeded"


def _tear_down_streamer(proc: "subprocess.Popen[bytes]") -> None:
    """Terminate the background `kubectl logs -f` cleanly. The streamer
    is purely a UX hook; we never let it influence the wait outcome and
    never let it survive past wait_agent_job's return."""
    if proc.poll() is not None:
        return
    proc.terminate()
    try:
        proc.wait(timeout=10)
    except subprocess.TimeoutExpired:
        proc.kill()
        try:
            proc.wait(timeout=5)
        except subprocess.TimeoutExpired:
            pass
