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
	"github.com/flo-at/sindri/internal/container"
	"github.com/flo-at/sindri/internal/hub/agent"
	"github.com/flo-at/sindri/internal/hub/messaging/mail"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/flo-at/sindri/internal/api"
)

// hostLookupTimeout bounds the host-side probes: check() runs inside New, before Serve answers the
// socket, so a wedged CLI must not wedge hub startup.
const hostLookupTimeout = 3 * time.Second

// toolskewMetaKey is where the last mismatch reported is persisted (Store.meta), so a restart reads
// its own past report instead of mailing the same finding again.
const toolskewMetaKey = "toolskew.said"

// toolskew reports a host/pod tool-version mismatch once, until it changes. Its two lookups are
// fields set at construction rather than package vars a test monkey-patches, so newHub(t)'s no-op
// default applies to every hub test for free.
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

// check mails the user whatever the host and the pod image disagree about. Silent when the image
// has never been built: nothing to compare with is not a mismatch.
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

// say mails the user once per distinct message, marking it reported only once the mail is written:
// a failed send leaves t.said unset, so the next check retries rather than going quiet for ever.
func (t *toolskew) say(msg string) {
	if msg == t.said {
		return
	}
	if msg != "" {
		if err := t.h.mail.Deliver(api.GlobalProject, api.SenderUser, msg, mail.MailOnly.From("hub")); err != nil {
			fmt.Fprintf(os.Stderr, "hub: mailing tool-version mismatch: %v\n", err)
			return
		}
	}
	t.said = msg
	if err := t.h.store.SetMeta(toolskewMetaKey, msg); err != nil {
		fmt.Fprintf(os.Stderr, "hub: persisting tool-skew state: %v\n", err)
	}
}

// defaultPodManifest is the pod half of the comparison: the image's baked-in manifest, a cache read
// with no build and no container call. brokkr is bind-mounted rather than baked, so its version
// comes from the mounted binary's own build info instead.
func defaultPodManifest() (map[string]string, error) {
	m, err := container.ImageManifest(container.ImageName)
	if err != nil {
		return nil, err
	}
	if v, ok := agent.PodBrokkrVersion(); ok {
		if m == nil {
			m = map[string]string{}
		}
		m["brokkr"] = v
	}
	return m, nil
}
