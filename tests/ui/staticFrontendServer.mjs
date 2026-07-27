// A stand-in for the operator's Go UI server, used by the browser tests.
//
// It deliberately mirrors src/ui/ui.go and src/ui/server.go rather than being a
// generic file server: the frontend only learns about its sub-path through the
// <base> tag and window.__BASE_PATH__ that serveHTML splices into the markup, so
// a plain file server would silently test a different application than the one
// that ships. Keep this file in sync with serveHTML / registerUiRoutes.
//
// Environment:
//   STATIC_FRONTEND_PORT  port to listen on (default 8099)
//   BASE_PATH             sub-path to serve under, same semantics as the operator
//   INDEX_HTML_PATH       override for index.html, used to run a spec against a
//                         different revision of the page (see the README)
//   MOCK_API              serve mock /api/v1 data so the dashboard can be used by
//                         hand without a cluster (`just ui-dev`)

import { createServer } from "node:http";
import { readFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

import {
  createMockOperatorApi,
  describeMockOperatorApi,
} from "./mockOperatorApi.mjs";

const testsDirectory = path.dirname(fileURLToPath(import.meta.url));
const staticRootDirectory = path.resolve(testsDirectory, "../../src/static");

const contentTypesByExtension = {
  ".html": "text/html; charset=utf-8",
  ".js": "text/javascript; charset=utf-8",
  ".mjs": "text/javascript; charset=utf-8",
  ".css": "text/css; charset=utf-8",
  ".json": "application/json; charset=utf-8",
  ".png": "image/png",
  ".svg": "image/svg+xml",
  ".ico": "image/x-icon",
  ".woff2": "font/woff2",
};

// Mirrors ui.normalizeBasePath: leading slash, never a trailing one, "" for root.
export function normalizeBasePath(rawBasePath) {
  const trimmed = (rawBasePath ?? "").trim().replace(/^\/+|\/+$/g, "");
  return trimmed === "" ? "" : `/${trimmed}`;
}

// Mirrors ui.serveHTML: splices <base href> and window.__BASE_PATH__ into <head>.
function injectBasePath(html, basePath) {
  const injection =
    `<head>\n    <base href="${basePath}/">\n` +
    `    <script>window.__BASE_PATH__ = ${JSON.stringify(basePath)};</script>`;
  return html.replace("<head>", injection);
}

async function serveHtmlPage(response, absolutePath, basePath) {
  try {
    const html = await readFile(absolutePath, "utf8");
    response.writeHead(200, {
      "Content-Type": contentTypesByExtension[".html"],
      "Cache-Control": "no-cache",
    });
    response.end(injectBasePath(html, basePath));
  } catch {
    response.writeHead(404, { "Content-Type": "text/plain" });
    response.end("not found");
  }
}

async function serveStaticAsset(response, relativePath) {
  // Resolve first, then confirm the result is still inside the static root, so a
  // traversal like /../../go.mod cannot escape it.
  const absolutePath = path.resolve(staticRootDirectory, `.${relativePath}`);
  if (
    absolutePath !== staticRootDirectory &&
    !absolutePath.startsWith(staticRootDirectory + path.sep)
  ) {
    response.writeHead(403, { "Content-Type": "text/plain" });
    response.end("forbidden");
    return;
  }

  try {
    const body = await readFile(absolutePath);
    response.writeHead(200, {
      "Content-Type":
        contentTypesByExtension[path.extname(absolutePath).toLowerCase()] ??
        "application/octet-stream",
      "Cache-Control": "no-cache",
    });
    response.end(body);
  } catch {
    response.writeHead(404, { "Content-Type": "text/plain" });
    response.end("not found");
  }
}

export function createStaticFrontendServer({
  basePath = normalizeBasePath(process.env.BASE_PATH),
  indexHtmlPath = process.env.INDEX_HTML_PATH ||
    path.join(staticRootDirectory, "index.html"),
  // Off by default: the Playwright specs stub the API themselves via page.route.
  mockApi = process.env.MOCK_API === "1" || process.env.MOCK_API === "true",
} = {}) {
  const handleMockApiRequest = mockApi ? createMockOperatorApi() : null;

  return createServer(async (request, response) => {
    const requestPath = new URL(request.url, "http://127.0.0.1").pathname;

    // server.go redirects the bare root to the base path when one is configured.
    if (basePath !== "" && requestPath === "/") {
      response.writeHead(302, { Location: `${basePath}/` });
      response.end();
      return;
    }

    const relativePath = requestPath.startsWith(basePath)
      ? requestPath.slice(basePath.length)
      : requestPath;

    if (handleMockApiRequest &&
        (await handleMockApiRequest(request, response, relativePath))) {
      return;
    }

    if (relativePath === "/logs") {
      await serveHtmlPage(
        response,
        path.join(staticRootDirectory, "pages/logs.html"),
        basePath,
      );
      return;
    }

    if (relativePath === "" || relativePath === "/" || relativePath === "/index.html") {
      await serveHtmlPage(response, indexHtmlPath, basePath);
      return;
    }

    await serveStaticAsset(response, relativePath);
  });
}

// Started as a child process by playwright.config.mjs (webServer).
if (process.argv[1] === fileURLToPath(import.meta.url)) {
  const port = Number(process.env.STATIC_FRONTEND_PORT || 8099);
  const mockApi = process.env.MOCK_API === "1" || process.env.MOCK_API === "true";
  createStaticFrontendServer().listen(port, "127.0.0.1", () => {
    const basePath = normalizeBasePath(process.env.BASE_PATH);
    console.log(`static frontend listening on http://127.0.0.1:${port}${basePath}/`);
    console.log(mockApi ? describeMockOperatorApi() : "mock API off (set MOCK_API=1)");
  });
}
