// package: hub / toolskew
// type:    logic
// job:     compares the host's tool versions against the pod's (-> defaultPodManifest), mailing
// the user once per distinct mismatch — durably, so a restart doesn't repeat what it reported.
// limits:  reports, never blocks a build (-> check-go.sh, the cautionary tale). Only
// container.ImageName is compared, not a custom recipe's tag, and only at startup — a
// manifest that first appears mid-session (-> EnsureImage/RebuildImage) waits for the
// next restart to be picked up.
package hub

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/workflow"
)

// hostLookupTimeout bounds the host-side probes: check() runs inside New, before Serve answers the
// socket, so a wedged CLI must not wedge hub startup.
const hostLookupTimeout = 3 * time.Second

// toolskewMetaKey is where the last mismatch reported is persisted (Store.meta), so a restart reads
// its own past report instead of mailing the same finding again.
const toolskewMetaKey = "toolskew.said"

// toolskew reports a host/pod tool-version mismatch once, until it changes. Its two lookups are
// fields set at construction, not package vars monkey-patched by a test: newHub(t)'s default (a
// no-op) then applies to every hub test for free, and only toolskew's own tests need the real shape.
type toolskew struct {
	h            *Hub
	said         string
	hostVersions func(context.Context) map[string]string
	podManifest  func() (map[string]string, error)
}

// newToolskew loads whatever was last reported — surviving a restart — then runs the comparison
// once: the pod image only changes on a rebuild, not tick by tick.
func newToolskew(h *Hub, hostVersions func(context.Context) map[string]string, podManifest func() (map[string]string, error)) *toolskew {
	t := &toolskew{h: h, hostVersions: hostVersions, podManifest: podManifest}
	if saved, ok, err := h.store.GetMeta(toolskewMetaKey); err == nil && ok {
		t.said = saved
	}
	t.check()
	return t
}

// check compares the host's tool versions against the pod image's manifest and mails the user of
// whatever disagrees. Silent when the image has never been built: there is nothing yet to compare,
// which is not the same as a mismatch.
func (t *toolskew) check() {
	manifest, err := t.podManifest()
	if err != nil || len(manifest) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(t.h.lifetime, hostLookupTimeout)
	defer cancel()
	host := t.hostVersions(ctx)

	tools := make([]string, 0, len(manifest))
	for tool := range manifest {
		tools = append(tools, tool)
	}
	sort.Strings(tools)
	var lines []string
	for _, tool := range tools {
		podV := manifest[tool]
		hostV, ok := host[tool]
		if !ok || hostV == podV {
			continue
		}
		lines = append(lines, fmt.Sprintf("  %s: pods run %s, this host %s", tool, podV, hostV))
	}
	if len(lines) == 0 {
		t.say("")
		return
	}
	t.say("[hub] the gate and the agents run different tool versions — the gate uses this host's:\n" +
		strings.Join(lines, "\n"))
}

// say mails the user once per distinct message, recording it as reported only once the mail is
// actually written: SAY IT ONCE is about not repeating a mismatch the user HAS read, not about
// giving up on one that never reached them. A failed send leaves t.said unset, so the very next
// check retries rather than staying silent for ever.
func (t *toolskew) say(msg string) {
	if msg == t.said {
		return
	}
	if msg != "" {
		if err := t.h.Deliver(workflow.GlobalProject, api.SenderUser, msg, workflow.MailOnly.From("hub")); err != nil {
			fmt.Fprintf(os.Stderr, "hub: mailing tool-version mismatch: %v\n", err)
			return
		}
	}
	t.said = msg
	if err := t.h.store.SetMeta(toolskewMetaKey, msg); err != nil {
		fmt.Fprintf(os.Stderr, "hub: persisting tool-skew state: %v\n", err)
	}
}
