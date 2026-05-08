from __future__ import annotations

import argparse
import json
import os
import sys

from . import ops


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="Exact ambience preview operations")
    subparsers = parser.add_subparsers(dest="command", required=True)

    build = subparsers.add_parser("build-preview-image")
    build.add_argument("--image-tag", required=True)

    deploy_validation = subparsers.add_parser("deploy-validation-preview")
    deploy_validation.add_argument("--image", required=True)
    deploy_validation.add_argument("--namespace", default="")
    deploy_validation.add_argument("--release", default="")
    deploy_validation.add_argument(
        "--public-host",
        default="",
        help="Optional public hostname for the validation env (attached to the wildcard listener).",
    )
    deploy_validation.add_argument(
        "--no-create-namespace",
        action="store_false",
        default=True,
        dest="create_namespace",
        help="Do not pass Helm --create-namespace; use when the namespace is pre-created.",
    )
    deploy_validation.add_argument(
        "--skip-external-dns",
        action="store_true",
        help="Annotate the validation HTTPRoute so external-dns leaves pre-provisioned host records alone.",
    )

    screenshot = subparsers.add_parser("capture-validation-screenshot")
    screenshot.add_argument("--page-path", required=True)
    screenshot.add_argument("--output-path", required=True)
    screenshot.add_argument("--namespace", default="")
    screenshot.add_argument("--wait-ms", type=int, default=5000)

    rebuild_validation = subparsers.add_parser(
        "rebuild-validation-image",
        help="Build a fresh image from a pushed branch ref and roll the "
        "validation env's edge/authority workloads onto it.",
    )
    rebuild_validation.add_argument("--namespace", required=True)
    rebuild_validation.add_argument("--branch", required=True)
    rebuild_validation.add_argument("--image-tag", required=True)
    rebuild_validation.add_argument("--repo-slug", default="nelsong6/ambience")

    upsert_pr = subparsers.add_parser("upsert-pr-preview")
    upsert_pr.add_argument("--pr-number", type=int, required=True)
    upsert_pr.add_argument("--image", required=True)

    wait_pr = subparsers.add_parser("wait-public-preview")
    wait_pr.add_argument("--url", required=True)
    wait_pr.add_argument("--timeout-seconds", type=int, default=900)

    destroy_validation = subparsers.add_parser("destroy-validation-preview")
    destroy_validation.add_argument("--namespace", default="")
    destroy_validation.add_argument("--release", default="")

    destroy_pr = subparsers.add_parser("destroy-pr-preview")
    destroy_pr.add_argument("--pr-number", type=int, required=True)

    apply_agent = subparsers.add_parser(
        "apply-agent-job",
        help="Render and `kubectl apply` the agent Job for one issue run.",
    )
    apply_agent.add_argument("--namespace", required=True)
    apply_agent.add_argument("--job-name", required=True)
    apply_agent.add_argument("--issue-number", required=True)
    apply_agent.add_argument("--issue-title", required=True)
    apply_agent.add_argument("--issue-url", required=True)
    apply_agent.add_argument("--issue-reference", default=None)
    apply_agent.add_argument("--validation-url", required=True)
    apply_agent.add_argument("--branch-name", required=True)
    apply_agent.add_argument("--proxy-ip", required=True)
    apply_agent.add_argument("--agent-container-tag", required=True)
    apply_agent.add_argument("--repo-slug", default="nelsong6/ambience")
    apply_agent.add_argument(
        "--stage",
        default="test-plan",
        choices=["test-plan", "implement", "verify"],
        help="Which stage's bash script the agent container runs.",
    )
    apply_agent.add_argument(
        "--config-map-name",
        default="agent-config",
        help="Name of the configmap mounted at /agent-config in the agent pod.",
    )

    wait_agent = subparsers.add_parser(
        "wait-agent-job",
        help="Wait for an agent Job's Pod to reach a terminal state, streaming "
        "logs to stdout. Exits non-zero on Job failure.",
    )
    wait_agent.add_argument("--namespace", required=True)
    wait_agent.add_argument("--job-name", required=True)
    wait_agent.add_argument("--timeout-seconds", type=int, default=1800)

    return parser


def dump(result: dict) -> None:
    json.dump(result, sys.stdout, indent=2)
    sys.stdout.write("\n")


def main() -> int:
    parser = build_parser()
    args = parser.parse_args()
    try:
        if args.command == "build-preview-image":
            dump(ops.build_preview_image(image_tag=args.image_tag))
        elif args.command == "deploy-validation-preview":
            namespace = args.namespace or str(get_required_env("EPHEMERAL_NAMESPACE"))
            release = args.release or get_optional_env("EPHEMERAL_RELEASE", ops.DEFAULT_RELEASE_NAME)
            public_host = args.public_host or None
            dump(
                ops.deploy_preview(
                    namespace=namespace,
                    image=args.image,
                    release=release,
                    public_host=public_host,
                    create_namespace=args.create_namespace,
                    external_dns=not args.skip_external_dns,
                )
            )
        elif args.command == "capture-validation-screenshot":
            namespace = args.namespace or str(get_required_env("EPHEMERAL_NAMESPACE"))
            dump(
                ops.capture_validation_screenshot(
                    namespace=namespace,
                    page_path=args.page_path,
                    output_path=args.output_path,
                    wait_ms=args.wait_ms,
                )
            )
        elif args.command == "rebuild-validation-image":
            dump(
                ops.rebuild_validation_image(
                    namespace=args.namespace,
                    branch=args.branch,
                    image_tag=args.image_tag,
                    repo_slug=args.repo_slug,
                )
            )
        elif args.command == "upsert-pr-preview":
            dump(ops.upsert_pr_preview(pr_number=args.pr_number, image=args.image))
        elif args.command == "wait-public-preview":
            dump(ops.wait_public_preview(url=args.url, timeout_seconds=args.timeout_seconds))
        elif args.command == "destroy-validation-preview":
            namespace = args.namespace or str(get_required_env("EPHEMERAL_NAMESPACE"))
            release = args.release or get_optional_env("EPHEMERAL_RELEASE", ops.DEFAULT_RELEASE_NAME)
            dump(ops.destroy_preview(namespace=namespace, release=release))
        elif args.command == "destroy-pr-preview":
            dump(ops.destroy_pr_preview(pr_number=args.pr_number))
        elif args.command == "apply-agent-job":
            dump(
                ops.apply_agent_job(
                    namespace=args.namespace,
                    job_name=args.job_name,
                    issue_number=args.issue_number,
                    issue_title=args.issue_title,
                    issue_url=args.issue_url,
                    issue_reference=args.issue_reference,
                    validation_url=args.validation_url,
                    branch_name=args.branch_name,
                    proxy_ip=args.proxy_ip,
                    agent_container_tag=args.agent_container_tag,
                    repo_slug=args.repo_slug,
                    stage=args.stage,
                    config_map_name=args.config_map_name,
                )
            )
        elif args.command == "wait-agent-job":
            dump(
                ops.wait_agent_job(
                    namespace=args.namespace,
                    job_name=args.job_name,
                    timeout_seconds=args.timeout_seconds,
                )
            )
        else:
            parser.error(f"unknown command: {args.command}")
    except Exception as error:
        print(
            json.dumps(
                {
                    "success": False,
                    "error": f"{type(error).__name__}: {error}",
                    "command": args.command,
                },
                indent=2,
            ),
            file=sys.stderr,
        )
        return 1
    return 0


def get_required_env(name: str) -> str:
    value = get_optional_env(name, "")
    if not value:
        raise ValueError(f"{name} is required when the CLI flag is omitted")
    return value


def get_optional_env(name: str, default: str) -> str:
    return str(os.environ.get(name, default))


if __name__ == "__main__":
    raise SystemExit(main())
