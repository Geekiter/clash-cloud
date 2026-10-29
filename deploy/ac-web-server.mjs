import { createServer, request as proxyRequest } from "node:http";
import { createReadStream, statSync } from "node:fs";
import { extname, resolve, sep } from "node:path";

const root = process.env.CLASH_CLOUD_WEB_ROOT || "";
const backend = new URL(process.env.CLASH_CLOUD_BACKEND_URL || "http://127.0.0.1:18082");
const secret = process.env.CLASH_CLOUD_INGRESS_SECRET || "";
if (!root) throw new Error("CLASH_CLOUD_WEB_ROOT is required");
const mime = { ".html":"text/html; charset=utf-8", ".js":"text/javascript; charset=utf-8", ".css":"text/css; charset=utf-8", ".svg":"image/svg+xml", ".png":"image/png", ".ico":"image/x-icon", ".woff":"font/woff", ".woff2":"font/woff2" };

createServer((req, res) => {
  if (!secret || req.headers["x-clash-cloud-ingress"] !== secret) {
    res.writeHead(403, { "content-type": "text/plain; charset=utf-8", "cache-control": "no-store" });
    return res.end("forbidden");
  }
  const pathname = new URL(req.url || "/", "http://local").pathname;
  if (pathname.startsWith("/api/v1/") || pathname === "/api/v1" || pathname.startsWith("/auth/") || pathname.startsWith("/s/")) {
    const headers = { ...req.headers, host: req.headers.host || "clash-cloud.chenkeyan.com", "x-forwarded-proto": "https" };
    const upstream = proxyRequest({ hostname: backend.hostname, port: backend.port, method: req.method, path: req.url, headers }, response => {
      res.writeHead(response.statusCode || 502, response.headers);
      response.pipe(res);
    });
    upstream.on("error", () => { if (!res.headersSent) res.writeHead(502); res.end(); });
    req.pipe(upstream);
    return;
  }
  let decoded;
  try { decoded = decodeURIComponent(pathname); } catch { res.writeHead(400); return res.end(); }
  let file = resolve(root, `.${decoded}`);
  if (file !== root && !file.startsWith(root + sep)) { res.writeHead(403); return res.end(); }
  try { if (statSync(file).isDirectory()) file = resolve(file, "index.html"); statSync(file); }
  catch { file = resolve(root, "index.html"); }
  res.setHeader("content-type", mime[extname(file)] || "application/octet-stream");
  res.setHeader("cache-control", file.endsWith("index.html") ? "no-cache" : "public, max-age=2592000, immutable");
  createReadStream(file).on("error", () => { if (!res.headersSent) res.writeHead(404); res.end(); }).pipe(res);
}).listen(Number(process.env.CLASH_CLOUD_WEB_PORT || 18081), process.env.CLASH_CLOUD_WEB_HOST || "127.0.0.1");
