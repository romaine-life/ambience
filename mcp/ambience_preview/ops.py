from __future__ import annotations

import os
import ssl
import subprocess
import time
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path


DEFAULT_REGISTRY_NAME = "romainecr"
DEFAULT_IMAGE_REPOSITORY = "ambience"
DEFAULT_RELEASE_NAME = "ambience-agent"
DEFAULT_PR_RELEASE_NAME = "ambience-pr"
DEFAULT_SERVICE_NAME = "ambience"
DEFAULT_HOST_SUFFIX = "ambience.dev.romaine.life"


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


def image_parts(image: str) -> tuple[str, str]:
    repository, tag = image.rsplit(":", 1)
    return repository, tag


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
    registry_name: str = DEFAULT_REGISTRY_NAME,
    image_repository: str = DEFAULT_IMAGE_REPOSITORY,
) -> dict:
    registry_server = os.environ.get("REGISTRY_SERVER", f"{registry_name}.azurecr.io")
    image = f"{registry_server}/{image_repository}:{image_tag}"

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

    verified_tag = run_command(
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

    if verified_tag != image_tag:
        raise CommandError(f"Image tag '{image_tag}' was not found in {registry_server}/{image_repository}.")

    return {
        "image": image,
        "image_tag": image_tag,
        "image_repository": image_repository,
        "registry_name": registry_name,
        "registry_server": registry_server,
    }


def deploy_preview(
    *,
    namespace: str,
    image: str,
    release: str = DEFAULT_RELEASE_NAME,
    public_host: str | None = None,
    create_namespace: bool = True,
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
        "authority.storage.size": "256Mi",
        "edge.shutdownDrain": "1s",
        "edge.terminationGracePeriodSeconds": "3",
        "authority.terminationGracePeriodSeconds": "5",
    }
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
    registry_name: str = DEFAULT_REGISTRY_NAME,
    image_repository: str = DEFAULT_IMAGE_REPOSITORY,
    repo_slug: str = "nelsong6/ambience",
    rollout_timeout: str = "300s",
    service_name: str = DEFAULT_SERVICE_NAME,
) -> dict:
    """Build a fresh image from the pushed agent branch and roll the
    validation env onto it.

    The validation env was deployed up-front (before the agent ran) so the
    agent has a URL to reference while it works. Once the agent pushes its
    commit, that env is stale relative to the diff that's about to be
    proposed in the PR. We rebuild from the branch ref directly (`az acr
    build https://...#branch`) instead of re-cloning locally, then `kubectl
    set image` flips the workloads onto the new tag without redeploying the
    chart (avoids re-running route/cert plumbing)."""
    registry_server = os.environ.get("REGISTRY_SERVER", f"{registry_name}.azurecr.io")
    image = f"{registry_server}/{image_repository}:{image_tag}"

    run_command(
        [
            "az",
            "acr",
            "build",
            "--registry",
            registry_name,
            "--image",
            f"{image_repository}:{image_tag}",
            f"https://github.com/{repo_slug}.git#{branch}",
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
        "image": image,
        "image_tag": image_tag,
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

# Bash that runs inside the agent container. Plain Python string — no
# interpolation happens here. All `${VAR}` references are evaluated by the
# container's bash from the env block defined in apply_agent_job below
# (REPO_SLUG, GH_TOKEN, ISSUE_NUMBER, BRANCH_NAME, etc).
_AGENT_BASH_SCRIPT = r"""set -euo pipefail

# Pre-create the evidence dirs the agent writes into. Sibling of
# /workspace/repo (the clone root) so `git add -A` in the repo doesn't
# pick PNGs/notes up. The post-Job workflow extracts /workspace/evidence
# from the agent pod's stdout (see end of script); the dirs need to
# exist for the tar to succeed even if the agent produced no evidence.
mkdir -p /workspace/evidence/screenshots

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
GIT_AUTH_HEADER="Authorization: Basic $(printf 'x-access-token:%s' "${GH_TOKEN}" | base64 | tr -d '\n')"

git -c "http.extraHeader=${GIT_AUTH_HEADER}" \
  clone "https://github.com/${REPO_SLUG}.git" /workspace/repo
cd /workspace/repo
if git show-ref --verify --quiet "refs/remotes/origin/${BRANCH_NAME}"; then
  git checkout -B "${BRANCH_NAME}" "origin/${BRANCH_NAME}"
else
  git checkout -B "${BRANCH_NAME}"
fi

if [ -n "${GITHUB_ISSUE_NUMBER:-}" ]; then
  issue_heading="# Issue #${GITHUB_ISSUE_NUMBER}: ${ISSUE_TITLE}"
  close_line="Closes #${GITHUB_ISSUE_NUMBER}"
else
  issue_heading="# Glimmung issue ${ISSUE_REFERENCE}: ${ISSUE_TITLE}"
  close_line="Glimmung-Issue: ${ISSUE_REFERENCE}"
fi

cat > /tmp/issue-context.md <<EOF
${issue_heading}
URL: ${ISSUE_URL}
Validation env: ${VALIDATION_URL}
EOF
cat /agent-config/prompt.md /tmp/issue-context.md > /tmp/agent-input.md

# stream-json + verbose so the GHA step surfaces tool calls + partial
# messages as they happen, instead of going silent for the whole agent
# run and dumping the final response at exit. Each event is one JSON
# object on its own line; the workflow's `kubectl logs -f` pipes them
# straight to the step output.
cat /tmp/agent-input.md | claude \
  --print \
  --output-format stream-json \
  --verbose \
  --dangerously-skip-permissions \
  2>&1 | tee /tmp/claude-stream.log

# Refuse to publish runner-local config files. The prompt tells the
# agent not to touch these; this is the second line of defense.
BLOCKED=$(git status --porcelain -- .github/workflows .github/agent .mcp.json 2>/dev/null || true)
if [ -n "$BLOCKED" ]; then
  echo "agent modified runner-local config files (forbidden by prompt):" >&2
  echo "$BLOCKED" >&2
  exit 1
fi

git add -A
if git diff --cached --quiet; then
  echo "agent produced no changes; failing job so the workflow doesn't open an empty PR" >&2
  exit 1
fi
git commit -m "agent: address ${ISSUE_REFERENCE}

${ISSUE_TITLE}

${close_line}"

# Sync onto current main before pushing. The agent ran for several minutes;
# main may have moved (e.g. someone merged a workflow tweak). Pushing a
# branch whose tip has a stale view of `.github/workflows/*` would be
# rejected by GitHub's workflow-permission check, even though the agent's
# commit didn't touch those files. Rebase replays the agent's single
# commit on top of current main; if there's a real conflict we fail loudly
# rather than ship a stale-base branch.
git -c "http.extraHeader=${GIT_AUTH_HEADER}" fetch origin main
git rebase origin/main

git -c "http.extraHeader=${GIT_AUTH_HEADER}" push origin "HEAD:${BRANCH_NAME}"

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
#      it spawned, playwright finalizing screenshot writes, etc.) were
#      still emitting stdout when the EVIDENCE-TAR markers were printed,
#      so other text could land between them. Fix: `wait` for any bash
#      jobs and `sync` the filesystem before running tar.
#
# Empty evidence dir is fine; the markers just frame an empty tar.
if [ -d /workspace/evidence ]; then
  # Settle background work the agent might have left behind.
  wait 2>/dev/null || true
  sync || true

  # Stage the tarball on disk first. Stderr from tar/gzip is captured
  # to a sidecar file so it can never interleave with the base64 stream.
  if ! tar -czf /tmp/evidence.tgz -C /workspace/evidence . 2>/tmp/tar.err; then
    echo "evidence tar FAILED; stderr was:" >&2
    cat /tmp/tar.err >&2 || true
    # Continue anyway with whatever (possibly partial) tarball was written.
  fi

  # Emit markers and base64 from the staged file. base64's input is the
  # local file (no pipes that could interleave), and stderr for the
  # whole block is suppressed so any stragglers can't poison the stream.
  {
    echo "===EVIDENCE-TAR-START==="
    base64 < /tmp/evidence.tgz
    echo "===EVIDENCE-TAR-END==="
  } 2>/dev/null
fi
"""


def _agent_job_spec(
    *,
    namespace: str,
    job_name: str,
    issue_number: str,
    issue_title: str,
    issue_url: str,
    issue_reference: str | None,
    github_issue_number: str | None,
    validation_url: str,
    branch_name: str,
    proxy_ip: str,
    agent_container_tag: str,
    repo_slug: str = "nelsong6/ambience",
) -> dict:
    """Build the Job spec as a Python dict. No templating; values land directly
    in their typed positions."""
    issue_ref = issue_reference or (f"#{issue_number}" if issue_number else "glimmung-run")
    gh_issue_number = issue_number if github_issue_number is None else github_issue_number

    return {
        "apiVersion": "batch/v1",
        "kind": "Job",
        "metadata": {
            "name": job_name,
            "namespace": namespace,
            "labels": {
                "app.kubernetes.io/name": "ambience-agent",
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
                    "hostAliases": [
                        {"ip": proxy_ip, "hostnames": ["api.anthropic.com"]},
                    ],
                    "volumes": [
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
                            "configMap": {"name": "agent-config"},
                        },
                    ],
                    "containers": [
                        {
                            "name": "agent",
                            # agent-container is the issue-runner image (playwright
                            # noble + Go + claude-code), distinct from
                            # claude-container (which is alpine + serves tank-operator
                            # interactive sessions). See tank-operator/agent-container/.
                            "image": f"romainecr.azurecr.io/agent-container:{agent_container_tag}",
                            "imagePullPolicy": "IfNotPresent",
                            "command": ["/bin/bash", "-c", _AGENT_BASH_SCRIPT],
                            "env": [
                                {"name": "NODE_EXTRA_CA_CERTS", "value": "/etc/claude-ca/ca.crt"},
                                {"name": "HOME", "value": "/workspace"},
                                {"name": "ISSUE_NUMBER", "value": str(issue_number)},
                                {"name": "ISSUE_REFERENCE", "value": issue_ref},
                                {"name": "GITHUB_ISSUE_NUMBER", "value": gh_issue_number or ""},
                                {"name": "ISSUE_TITLE", "value": issue_title},
                                {"name": "ISSUE_URL", "value": issue_url},
                                {"name": "VALIDATION_URL", "value": validation_url},
                                {"name": "BRANCH_NAME", "value": branch_name},
                                {"name": "REPO_SLUG", "value": repo_slug},
                                {
                                    "name": "GH_TOKEN",
                                    "valueFrom": {
                                        "secretKeyRef": {
                                            "name": "agent-github-token",
                                            "key": "token",
                                        },
                                    },
                                },
                                {"name": "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC", "value": "1"},
                            ],
                            "volumeMounts": [
                                {"name": "claude-ca", "mountPath": "/etc/claude-ca", "readOnly": True},
                                {"name": "workspace", "mountPath": "/workspace"},
                                {"name": "agent-config", "mountPath": "/agent-config", "readOnly": True},
                            ],
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
    repo_slug: str = "nelsong6/ambience",
    issue_reference: str | None = None,
    github_issue_number: str | None = None,
) -> dict:
    """Render the agent Job spec and `kubectl apply -f -` it."""
    import json as _json
    spec = _agent_job_spec(
        namespace=namespace,
        job_name=job_name,
        issue_number=issue_number,
        issue_title=issue_title,
        issue_url=issue_url,
        issue_reference=issue_reference,
        github_issue_number=github_issue_number,
        validation_url=validation_url,
        branch_name=branch_name,
        proxy_ip=proxy_ip,
        agent_container_tag=agent_container_tag,
        repo_slug=repo_slug,
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
        "applied": proc.stdout.strip(),
    }


def wait_agent_job(
    *,
    namespace: str,
    job_name: str,
    timeout_seconds: int = 1800,
    poll_interval_seconds: int = 3,
) -> dict:
    """Two-stage wait, status-field driven (no `kubectl wait` for `Complete`).

      1. Poll the Pod until it reaches Running | Succeeded | Failed.
      2. Stream its logs (`kubectl logs -f`) — blocks until pod terminates.
      3. Poll Job `.status.succeeded` / `.status.failed` until one is non-empty
         (the Job controller can race the Pod-finished transition).

    Raises CommandError on Job failure (non-zero `failed` count or pod-never-
    appeared timeout). Returns on success."""
    deadline = time.time() + timeout_seconds

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
                break
        time.sleep(poll_interval_seconds)

    if not pod_name:
        raise CommandError(f"agent pod for Job {job_name!r} never appeared")

    print(f"agent pod {pod_name} (phase={phase}) — streaming logs", flush=True)
    # Stream logs directly to our stdout. Subprocess inherits FDs.
    subprocess.run(
        ["kubectl", "-n", namespace, "logs", "-f", pod_name],
        check=False,
    )

    # Logs ended (pod terminated). Poll Job status to terminal.
    succeeded = ""
    failed = ""
    while time.time() < deadline:
        succeeded = run_command(
            [
                "kubectl",
                "-n",
                namespace,
                "get",
                "job",
                job_name,
                "-o",
                "jsonpath={.status.succeeded}",
            ],
        )
        failed = run_command(
            [
                "kubectl",
                "-n",
                namespace,
                "get",
                "job",
                job_name,
                "-o",
                "jsonpath={.status.failed}",
            ],
        )
        if succeeded or failed:
            break
        time.sleep(2)

    if (int(succeeded) if succeeded else 0) >= 1:
        return {
            "namespace": namespace,
            "job": job_name,
            "pod": pod_name,
            "succeeded": int(succeeded),
            "failed": int(failed) if failed else 0,
        }

    raise CommandError(
        f"agent Job {job_name!r} failed (succeeded={succeeded or 0}, failed={failed or 0})"
    )
