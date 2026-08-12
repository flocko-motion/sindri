// package: lint / gotoolchain
// type:    logic (diagnosis)
// job:     recognise the go command refusing to run because the toolchain is older than go.mod
// requires, and answer it with the upgrade — the version needed and the command that installs it.
// limits:  text in, advice out. It upgrades nothing (`go-upgrade`, in the agent image, does).
package lint

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"golang.org/x/tools/go/packages"
)

// UpgradeCommand installs the toolchain go.mod asks for. It lives in the agent image, so the
// advice names it only where it exists.
const UpgradeCommand = "go-upgrade"

// tooOldRE matches the go command's refusal, e.g. "go: go.mod requires go >= 1.26.5 (running
// go 1.26.4; GOTOOLCHAIN=local)". A required module can raise the floor too, hence not just go.mod.
var tooOldRE = regexp.MustCompile(`requires go >= ([0-9][^ )]*) \(running go ([0-9][^;) ]*)`)

// ToolchainAdvice turns a go-command failure caused by an outdated toolchain into the fix, or
// returns "" for any other failure, leaving the caller's own errors alone. Exported because the
// gopls MCP shim answers the same refusal — one definition, so the two cannot drift into giving
// different advice for the same failure.
//
// Worth special-casing because the raw failure reads like a broken build: nothing in "load: err:
// exit status 1: stderr: go: go.mod requires go >= …" says the code is fine and the environment is
// behind, so the natural next move is to go looking for a defect that isn't there.
func ToolchainAdvice(text string) string {
	m := tooOldRE.FindStringSubmatch(text)
	if m == nil {
		return ""
	}
	want, have := m[1], m[2]
	fix := fmt.Sprintf("upgrade the Go toolchain to %s or newer", want)
	if _, err := exec.LookPath(UpgradeCommand); err == nil {
		fix = fmt.Sprintf("run `%s` to install it", UpgradeCommand)
	}
	return fmt.Sprintf("Go toolchain too old: this module needs go %s but go %s is running — %s, then re-run", want, have, fix)
}

// loadErrorText joins the errors the loaded packages carry, the other shape a toolchain refusal
// arrives in: go/packages can report it per package instead of failing the load outright.
func loadErrorText(initial []*packages.Package) string {
	var sb strings.Builder
	packages.Visit(initial, nil, func(p *packages.Package) {
		for _, e := range p.Errors {
			sb.WriteString(e.Error())
			sb.WriteByte('\n')
		}
	})
	return sb.String()
}
