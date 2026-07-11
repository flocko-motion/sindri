// package: ui/cli / cli
// type:    ui (host CLI shared bits)
// job:     package-shared state for the host CLI command tree — the build version
//          (set by the thin main via SetVersion, since -ldflags targets main) and
//          small shared render helpers. The commands live in the sibling files;
//          cmd/sindri/main.go assembles them via the exported New*Cmd constructors.
// limits:  no domain logic — each command delegates to the hub (client or in-process).
package cli

// version is the CLI build version, mirrored from main (where -ldflags "-X
// main.version" sets it) via SetVersion. The hub/version/upgrade commands compare
// against it.
var version = "dev"

// SetVersion mirrors the main-package build version into this package, so the
// command tree can report and compare it. Called once by cmd/sindri/main.go.
func SetVersion(v string) { version = v }

// dash renders "-" for an empty string in tabular output.
func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
