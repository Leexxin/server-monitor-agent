#!/usr/bin/env python3
"""Tiny SFTPGo browse-path fixture for disposable Linux release tests."""

from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from urllib.parse import parse_qs, unquote, urlsplit
import sys


root = Path(sys.argv[1]).resolve()


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        url = urlsplit(self.path)
        if url.path == "/upload":
            body = b"<html>'X-CSRF-TOKEN': 'fixture-token'</html>"
            self.send_response(200)
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)
            return
        if url.path != "/browse":
            self.send_error(404)
            return
        remote = parse_qs(url.query).get("path", [""])[0]
        target = (root / remote.lstrip("/")).resolve()
        if not target.is_relative_to(root) or not target.is_file():
            # SFTPGo can return a successful browse page for a missing file.
            body = b"<!DOCTYPE html><html><body>Not a file</body></html>"
            self.send_response(200)
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)
            return
        body = target.read_bytes()
        self.send_response(200)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_POST(self):
        url = urlsplit(self.path)
        if not url.path.startswith("/%2F%2F") or self.headers.get("X-CSRF-TOKEN") != "fixture-token":
            self.send_error(403)
            return
        remote = unquote(url.path[len("/%2F") :])
        target = (root / remote.lstrip("/")).resolve()
        if not target.is_relative_to(root):
            self.send_error(403)
            return
        target.parent.mkdir(parents=True, exist_ok=True)
        size = int(self.headers["Content-Length"])
        with target.open("wb") as output:
            output.write(self.rfile.read(size))
        self.send_response(201)
        self.send_header("Content-Length", "0")
        self.end_headers()


server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
print(server.server_address[1], flush=True)
server.serve_forever()
