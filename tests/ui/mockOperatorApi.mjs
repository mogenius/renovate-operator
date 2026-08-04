// An in-memory stand-in for the operator's /api/v1 surface, so the dashboard can
// be driven by hand without a Kubernetes cluster.
//
// Only staticFrontendServer.mjs uses this, and only when MOCK_API is set. The
// Playwright specs stub the API through page.route instead, which keeps each test
// in control of its own data — this module exists for manual UI work.
//
// The dashboard calls exactly eight endpoints (see the authFetch calls in
// index.html); the five POST ones only have their `ok` flag inspected, so an empty
// 200 is enough to exercise the buttons without a backend.

import { buildMultiJobDashboard } from "./fixtures/renovateJobsFixture.mjs";

const MOCK_VERSION = "dev-mock";

function sendJson(response, body, status = 200) {
  response.writeHead(status, {
    "Content-Type": "application/json; charset=utf-8",
    "Cache-Control": "no-store",
  });
  response.end(JSON.stringify(body));
}

// Drain the request so the socket can be reused for the next call.
function discardRequestBody(request) {
  return new Promise((resolve) => {
    request.on("data", () => {});
    request.on("end", resolve);
    request.on("error", resolve);
  });
}

export function createMockOperatorApi({
  jobCount = Number(process.env.MOCK_JOB_COUNT || 5),
  projectsPerJob = Number(process.env.MOCK_PROJECTS_PER_JOB || 12),
} = {}) {
  // Built once so the 30s dashboard poll keeps returning the same jobs and the
  // page does not reshuffle underneath you mid-click.
  const renovateJobs = buildMultiJobDashboard({ jobCount, projectsPerJob });

  /**
   * Handles a request if it targets the mocked API.
   * @returns true when the request was answered, false to let the static file
   *          handler take over.
   */
  return async function handleMockApiRequest(request, response, relativePath) {
    if (!relativePath.startsWith("/api/v1/")) {
      return false;
    }

    if (request.method === "GET") {
      switch (relativePath) {
        case "/api/v1/auth/status":
          sendJson(response, { enabled: false });
          return true;
        case "/api/v1/version":
          sendJson(response, { version: MOCK_VERSION });
          return true;
        case "/api/v1/renovatejobs":
          sendJson(response, renovateJobs);
          return true;
      }
    }

    // /renovate, /renovate/all, /renovate/cancel, /discovery/start and
    // /executionOptions. Nothing is mutated: the mock covers layout and
    // client-side state, not the operator's job lifecycle.
    if (request.method === "POST") {
      await discardRequestBody(request);
      sendJson(response, {});
      return true;
    }

    sendJson(
      response,
      { error: `mock API has no handler for ${request.method} ${relativePath}` },
      404,
    );
    return true;
  };
}

export function describeMockOperatorApi({
  jobCount = Number(process.env.MOCK_JOB_COUNT || 5),
  projectsPerJob = Number(process.env.MOCK_PROJECTS_PER_JOB || 12),
} = {}) {
  return `mock API on: ${jobCount} jobs x ${projectsPerJob} projects`;
}
