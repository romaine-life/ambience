from __future__ import annotations

import json
import os

import mcp.types as types
from mcp.server import Server
from mcp.server.stdio import stdio_server

from . import ops


server = Server("ambience-preview")


@server.list_tools()
async def list_tools() -> list[types.Tool]:
    return [
        types.Tool(
            name="build_preview_image",
            description="Build and push the exact ambience application image for the current workspace into ACR.",
            inputSchema={
                "type": "object",
                "properties": {
                    "image_tag": {
                        "type": "string",
                        "description": "Optional image tag. Defaults to the IMAGE_TAG environment variable.",
                    }
                },
            },
        ),
        types.Tool(
            name="deploy_validation_preview",
            description="Deploy the current image into the scratch validation namespace for this issue run.",
            inputSchema={
                "type": "object",
                "properties": {
                    "image": {
                        "type": "string",
                        "description": "Full container image reference returned from build_preview_image.",
                    },
                    "namespace": {
                        "type": "string",
                        "description": "Optional namespace override. Defaults to EPHEMERAL_NAMESPACE.",
                    },
                    "release": {
                        "type": "string",
                        "description": "Optional Helm release override. Defaults to EPHEMERAL_RELEASE.",
                    },
                },
                "required": ["image"],
            },
        ),
        types.Tool(
            name="capture_validation_screenshot",
            description="Capture a screenshot from the scratch validation preview by port-forwarding the service and using Playwright.",
            inputSchema={
                "type": "object",
                "properties": {
                    "page_path": {
                        "type": "string",
                        "description": "Route to capture, for example '/' or '/dev/waterfall'.",
                    },
                    "output_path": {
                        "type": "string",
                        "description": "Repo-relative path for the PNG output.",
                    },
                    "namespace": {
                        "type": "string",
                        "description": "Optional namespace override. Defaults to EPHEMERAL_NAMESPACE.",
                    },
                    "wait_ms": {
                        "type": "integer",
                        "description": "Additional browser settle time before the screenshot.",
                        "default": 5000,
                    },
                },
                "required": ["page_path", "output_path"],
            },
        ),
    ]


@server.call_tool()
async def call_tool(name: str, arguments: dict) -> types.CallToolResult:
    try:
        result = handle_tool(name, arguments or {})
        return types.CallToolResult(
            content=[types.TextContent(type="text", text=json.dumps(result, indent=2))],
            structuredContent=result,
            isError=False,
        )
    except Exception as error:
        payload = {
            "success": False,
            "tool": name,
            "error": f"{type(error).__name__}: {error}",
        }
        return types.CallToolResult(
            content=[types.TextContent(type="text", text=json.dumps(payload, indent=2))],
            structuredContent=payload,
            isError=True,
        )


def handle_tool(name: str, arguments: dict) -> dict:
    if name == "build_preview_image":
        image_tag = arguments.get("image_tag") or os.environ.get("IMAGE_TAG", "")
        if not image_tag:
            raise ValueError("image_tag is required when IMAGE_TAG is not set")
        return ops.build_preview_image(image_tag=image_tag)

    if name == "deploy_validation_preview":
        namespace = arguments.get("namespace") or os.environ.get("EPHEMERAL_NAMESPACE", "")
        release = arguments.get("release") or os.environ.get("EPHEMERAL_RELEASE", ops.DEFAULT_RELEASE_NAME)
        if not namespace:
            raise ValueError("namespace is required when EPHEMERAL_NAMESPACE is not set")
        return ops.deploy_preview(namespace=namespace, image=arguments["image"], release=release)

    if name == "capture_validation_screenshot":
        namespace = arguments.get("namespace") or os.environ.get("EPHEMERAL_NAMESPACE", "")
        if not namespace:
            raise ValueError("namespace is required when EPHEMERAL_NAMESPACE is not set")
        return ops.capture_validation_screenshot(
            namespace=namespace,
            page_path=arguments["page_path"],
            output_path=arguments["output_path"],
            wait_ms=int(arguments.get("wait_ms", 5000)),
        )

    raise ValueError(f"Unknown tool: {name}")


async def main() -> None:
    async with stdio_server() as streams:
        await server.run(
            streams[0],
            streams[1],
            server.create_initialization_options(),
        )


if __name__ == "__main__":
    import asyncio

    asyncio.run(main())
