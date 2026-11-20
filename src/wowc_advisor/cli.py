"""Console entry point: `wowc-advisor serve`."""

from __future__ import annotations

import argparse
from collections.abc import Sequence

from wowc_advisor.server import create_server


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(prog="wowc-advisor")
    sub = parser.add_subparsers(dest="command", required=True)
    serve = sub.add_parser("serve", help="run the MCP server")
    mode = serve.add_mutually_exclusive_group()
    mode.add_argument("--stdio", action="store_true", help="serve over stdio (default)")
    mode.add_argument("--http", action="store_true", help="serve streamable HTTP")
    serve.add_argument("--host", default="127.0.0.1", help="HTTP bind address")
    serve.add_argument("--port", type=int, default=8000, help="HTTP port")
    serve.add_argument("--dataset", help="dataset directory (default: $WOWC_DATASET)")
    serve.add_argument(
        "--savedvariables", help="ForeverScribe.lua file (default: $WOWC_SAVEDVARIABLES)"
    )
    return parser


def main(argv: Sequence[str] | None = None) -> None:
    args = build_parser().parse_args(argv)
    server = create_server(args.dataset, args.savedvariables)
    if args.http:
        server.run("streamable-http", host=args.host, port=args.port)
    else:
        server.run("stdio")
