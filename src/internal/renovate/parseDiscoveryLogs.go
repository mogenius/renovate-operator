package renovate

import (
	"bufio"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// parseAndSortDiscoveredProjects extracts the project list from discovery job logs and sorts it.
func parseAndSortDiscoveredProjects(logs string) ([]string, error) {
	discovered, err := parseDiscoveredProjects(logs)
	if err != nil {
		return nil, err
	}
	sort.Strings(discovered)
	return discovered, nil
}

// parseDiscoveredProjects extracts the JSON string array from discovery pod logs.
// It first tries to parse the entire log as JSON. If that fails (e.g. due to
// stderr output mixed into the logs), it scans line by line for a valid JSON array.
func parseDiscoveredProjects(logs string) ([]string, error) {
	discovered, err := parseLineForDiscoveredProjects(logs)
	if err == nil {
		return discovered, nil
	}

	scanner := bufio.NewScanner(strings.NewReader(logs))
	for scanner.Scan() {
		discovered, err = parseLineForDiscoveredProjects(scanner.Text())
		if err == nil {
			return discovered, nil
		}
	}

	return nil, fmt.Errorf("no valid JSON array found in discovery logs (%d bytes)", len(logs))
}

func parseLineForDiscoveredProjects(line string) ([]string, error) {
	line = strings.TrimSpace(line)

	if len(line) == 0 || line[0] != '[' {
		return nil, fmt.Errorf("line does not start with '[': %s", line)
	}

	var discovered []string
	if err := json.Unmarshal([]byte(line), &discovered); err == nil {
		return discovered, nil
	}

	var raw []json.RawMessage
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		return nil, err
	}

	discovered = make([]string, 0, len(raw))
	for _, elem := range raw {
		var s string
		if err := json.Unmarshal(elem, &s); err == nil {
			discovered = append(discovered, s)
			continue
		}
		var obj struct {
			Repository string `json:"repository"`
		}
		if err := json.Unmarshal(elem, &obj); err == nil && obj.Repository != "" {
			discovered = append(discovered, obj.Repository)
			continue
		}
		return nil, fmt.Errorf("unsupported element in discovered projects array: %s", elem)
	}

	return discovered, nil
}
