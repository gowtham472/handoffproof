import os
from http.server import BaseHTTPRequestHandler, HTTPServer


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path == "/health":
            self.send_response(200)
            self.end_headers()
            self.wfile.write(b'{"ok":true}')
            return
        if self.path == "/headers":
            if self.headers.get("X-Test-Key") == os.environ.get("MAIL_API_TOKEN"):
                self.send_response(200)
                self.end_headers()
                self.wfile.write(b'{"header":"ok"}')
            else:
                self.send_response(403)
                self.end_headers()
                self.wfile.write(b'{"header":"missing"}')
            return
        if self.path == "/mail":
            if os.environ.get("MAIL_API_TOKEN") == "test-token":
                self.send_response(200)
                self.end_headers()
                self.wfile.write(b'{"sent":true}')
            else:
                self.send_response(500)
                self.end_headers()
                self.wfile.write(b'{"sent":false}')
            return
        self.send_response(404)
        self.end_headers()

    def log_message(self, *args):
        pass


HTTPServer(("0.0.0.0", 3000), Handler).serve_forever()
