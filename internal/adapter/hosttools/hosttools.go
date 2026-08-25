// package: hosttools
// type:    adapter (external tool)
// job:     reports the host's installed go/node/openspec/brokkr versions — the one place the core
// (-> hub/toolskew.go) goes for them (openspec's own exec lives in adapter/tasks/spec,
// which this calls, so the core itself still never shells out directly).
// limits:  best-effort per tool; a missing or failing tool is simply absent from the result.
package hosttools

import (
	"context"
	"os/exec"
	"strings"

	"github.com/flo-at/sindri/internal/adapter/tasks/spec"
)

// Versions reports whatever of go/node/openspec/brokkr is found on the host's PATH — the same
// binaries a gate invocation (hub/repo.runVerify) would reach for, not the toolchain that built the
// caller.
func Versions(ctx context.Context) map[string]string {
	versions := map[string]string{}
	if v, ok := goVersion(ctx); ok {
		versions["go"] = v
	}
	if v, ok := run(ctx, "node", "--version"); ok {
		versions["node"] = v
	}
	if v, ok := spec.Version(ctx); ok {
		versions["openspec"] = v
	}
	if v, ok := brokkrVersion(ctx); ok {
		versions["brokkr"] = v
	}
	return versions
}

// goVersion runs `go version` and extracts its version field.
func goVersion(ctx context.Context) (string, bool) {
	out, ok := run(ctx, "go", "version")
	if !ok {
		return "", false
	}
	return parseGoVersion(out)
}

// parseGoVersion trims "go version go1.27.0 linux/amd64" down to "go1.27.0" — split out from
// goVersion so the parse itself is table-testable without a real `go` on PATH.
func parseGoVersion(raw string) (string, bool) {
	fields := strings.Fields(raw)
	if len(fields) < 3 {
		return "", false
	}
	return fields[2], true
}

// brokkrVersion runs `brokkr version`, whose first line is "brokkr <version>".
func brokkrVersion(ctx context.Context) (string, bool) {
	out, ok := run(ctx, "brokkr", "version")
	if !ok {
		return "", false
	}
	return parseBrokkrVersion(out)
}

// parseBrokkrVersion pulls the version off "brokkr <version>" — split out for the same reason as
// parseGoVersion: table-testable without a real `brokkr` on PATH.
func parseBrokkrVersion(raw string) (string, bool) {
	fields := strings.Fields(raw)
	if len(fields) < 2 {
		return "", false
	}
	return fields[1], true
}

func run(ctx context.Context, name string, args ...string) (string, bool) {
	out, err := exec.CommandContext(ctx, name, args...).Output()
	if err != nil {
		return "", false
	}
	return firstLine(string(out)), true
}

// firstLine keeps just a command's first line, trimmed — symmetric with the pod side, which only
// ever captures the one build-log line its marker echo produced (-> container.ParseVersionManifest),
// so a CLI whose --version prints more than one line can't compare as a permanent, unactionable skew.
func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}
