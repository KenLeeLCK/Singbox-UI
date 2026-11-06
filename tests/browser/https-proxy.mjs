import fs from "node:fs/promises";
import path from "node:path";
import http from "node:http";
import https from "node:https";
import { execFileSync } from "node:child_process";
// Tailscale Serve's TCP target preserves Host and Origin, terminates HTTPS,
// and supplies forwarded headers. Manager intentionally ignores those headers.
export async function httpsProxy(target, directory) {
  const key = path.join(directory, "proxy-key.pem"),
    cert = path.join(directory, "proxy-cert.pem");
  execFileSync(
    "openssl",
    [
      "req",
      "-x509",
      "-newkey",
      "rsa:2048",
      "-nodes",
      "-keyout",
      key,
      "-out",
      cert,
      "-subj",
      "/CN=localhost",
      "-days",
      "1",
    ],
    { stdio: "ignore" },
  );
  const server = https.createServer(
    { key: await fs.readFile(key), cert: await fs.readFile(cert) },
    (req, res) => {
      const upstream = http.request(
        target + req.url,
        {
          method: req.method,
          headers: {
            ...req.headers,
            "x-forwarded-proto": "https",
            "x-forwarded-host": req.headers.host,
          },
        },
        (r) => {
          res.writeHead(r.statusCode, r.headers);
          r.pipe(res);
        },
      );
      upstream.on("error", () => {
        res.writeHead(502);
        res.end();
      });
      res.on("close", () => upstream.destroy());
      req.pipe(upstream);
    },
  );
  await new Promise((r) => server.listen(0, "127.0.0.1", r));
  return {
    url: `https://localhost:${server.address().port}`,
    close() {
      server.closeAllConnections();
      server.close();
    },
  };
}
