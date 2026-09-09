// Builders for the log stream the logs page consumes.
//
// /api/v1/logs is a Server-Sent Events endpoint: getRenovateJobLogs in
// src/ui/renovateController.go copies every JSON line of a Renovate container log
// into one `data:` frame and closes the stream with an `event: done` frame. The
// page itself only reads `level`, `time` and `msg`; every other field appears in
// the JSON a row expands to, which is why these entries carry the extra fields
// Renovate really logs — the search box looks at all of them.

/** pino levels, the same scale api.LogIssue uses. */
export const LOG_LEVELS = { debug: 20, info: 30, warn: 40, error: 50, fatal: 60 };

/** The buckets levelInfo() in pages/logs.html sorts a level into. */
const LEVEL_BOUNDS = {
  FATAL: [60, Infinity],
  ERROR: [50, 59],
  WARN: [40, 49],
  INFO: [30, 39],
  DEBUG: [20, 29],
};

export const ALL_LEVEL_LABELS = Object.keys(LEVEL_BOUNDS);

/**
 * A term that appears in the `msg` of exactly one entry and in no other field, so
 * a search for it narrows the list down to that row without opening it.
 */
export const MESSAGE_SEARCH_TERM = "modified";
export const MESSAGE_SEARCH_TERM_ENTRY = "Branch has been modified externally";

/**
 * A term that appears only in a field below `msg`. Searching for it keeps the one
 * entry that carries it and — since the match is not in the message — expands that
 * row on its own, so the user can see what matched.
 */
export const DETAIL_ONLY_SEARCH_TERM = "registry.internal.example";

// Fixed so the rendered timestamps do not move between runs. Renovate logs `time`
// as epoch milliseconds.
const FIRST_ENTRY_TIME = Date.parse("2026-07-27T08:00:00Z");

function millisecondsIntoTheRun(offset) {
  return FIRST_ENTRY_TIME + offset;
}

export function buildLogEntry({ level = LOG_LEVELS.info, msg, time = FIRST_ENTRY_TIME, ...rest }) {
  return { level, time, msg, ...rest };
}

/** How many of `entries` the page renders under the given level label. */
export function countEntriesWithLevel(entries, levelLabel) {
  const [minimum, maximum] = LEVEL_BOUNDS[levelLabel];
  return entries.filter((entry) => entry.level >= minimum && entry.level <= maximum).length;
}

/**
 * One Renovate run over a single repository, with at least one entry per level
 * label so a spec can switch any checkbox in the level filter and see the list
 * change.
 *
 * @param fillerEntryCount extra DEBUG entries appended before the closing summary,
 *        to make the page taller than the viewport. They carry the same shape as
 *        the rest, so the level counts stay derivable via countEntriesWithLevel.
 */
export function buildRenovateRunLog({
  repository = "acme/service-1-1",
  fillerEntryCount = 0,
} = {}) {
  const entries = [
    buildLogEntry({
      level: LOG_LEVELS.info,
      time: millisecondsIntoTheRun(0),
      msg: "Repository started",
      repository,
      renovateVersion: "41.0.0",
    }),
    buildLogEntry({
      level: LOG_LEVELS.debug,
      time: millisecondsIntoTheRun(120),
      msg: "Resolving package registries",
      repository,
      // The only place DETAIL_ONLY_SEARCH_TERM occurs — a search for it has to
      // open this row to show what it matched.
      config: { npmrc: `registry=https://${DETAIL_ONLY_SEARCH_TERM}/npm/` },
    }),
    buildLogEntry({
      level: LOG_LEVELS.debug,
      time: millisecondsIntoTheRun(340),
      msg: "Detected package files",
      repository,
      packageFiles: ["package.json", "go.mod", "Dockerfile"],
    }),
    buildLogEntry({
      level: LOG_LEVELS.info,
      time: millisecondsIntoTheRun(900),
      msg: "Dependency extraction complete",
      repository,
      stats: { managers: { npm: { fileCount: 2, depCount: 74 } } },
    }),
    buildLogEntry({
      level: LOG_LEVELS.warn,
      time: millisecondsIntoTheRun(1500),
      msg: "Package lookup failures",
      repository,
      packageFiles: { npm: ["package.json"] },
    }),
    buildLogEntry({
      level: LOG_LEVELS.error,
      time: millisecondsIntoTheRun(1700),
      msg: "Failed to look up npm package @acme/ui",
      repository,
      depName: "@acme/ui",
      statusCode: 404,
    }),
    buildLogEntry({
      level: LOG_LEVELS.error,
      time: millisecondsIntoTheRun(2100),
      msg: "Error updating branch: update failure",
      repository,
      branchName: "renovate/react-19.x",
    }),
    buildLogEntry({
      level: LOG_LEVELS.warn,
      time: millisecondsIntoTheRun(2400),
      msg: "Branch has been modified externally",
      repository,
      branchName: "renovate/postgres-17.x",
    }),
    buildLogEntry({
      level: LOG_LEVELS.fatal,
      time: millisecondsIntoTheRun(2600),
      msg: "Repository has invalid config",
      repository,
      validationError: "Invalid configuration option: schedul",
    }),
    ...Array.from({ length: fillerEntryCount }, (_, index) =>
      buildLogEntry({
        level: LOG_LEVELS.debug,
        time: millisecondsIntoTheRun(3000 + index * 20),
        msg: `Considering dependency vendor/module-${index + 1}`,
        repository,
        depName: `vendor/module-${index + 1}`,
        currentValue: `1.${index}.0`,
      }),
    ),
    buildLogEntry({
      level: LOG_LEVELS.info,
      time: millisecondsIntoTheRun(9000),
      msg: "Repository finished",
      repository,
      durationMs: 42_000,
      cloned: true,
    }),
  ];

  return entries;
}

/**
 * The stream body getRenovateJobLogs writes: one `data:` frame per entry, then the
 * `done` event that tells the page to close its EventSource.
 */
export function encodeLogStream(entries) {
  return (
    entries.map((entry) => `data: ${JSON.stringify(entry)}\n\n`).join("") +
    "event: done\ndata: {}\n\n"
  );
}
