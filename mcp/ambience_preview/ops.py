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
        "--create-namespace",
        "--wait",
        "--timeout",
        timeout,
    ]

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
