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

    screenshot = subparsers.add_parser("capture-validation-screenshot")
    screenshot.add_argument("--page-path", required=True)
    screenshot.add_argument("--output-path", required=True)
    screenshot.add_argument("--namespace", default="")
    screenshot.add_argument("--wait-ms", type=int, default=5000)

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
            dump(ops.deploy_preview(namespace=namespace, image=args.image, release=release))
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
