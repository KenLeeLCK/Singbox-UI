"""Unrelated loopback listener deliberately impersonating the health response."""
from http.server import BaseHTTPRequestHandler, HTTPServer
class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200); self.end_headers(); self.wfile.write(b'{"ok":true}')
    def log_message(self,*args): pass
HTTPServer(('127.0.0.1',5031),Handler).serve_forever()
