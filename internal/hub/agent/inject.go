// package: hub/agent / inject
// type:    logic (message delivery into a running agent)
// job:     put text into an agent's live tmux session and look for it there — Inject,
// InjectWhenReady (for messages right after a launch) and Tell (source-stamped and
// logged). Repeated failures to confirm are what Unreachable reports.
// limits:  WHAT to send and WHEN is the caller's; an unconfirmed send is reported here,
// never retried. The tmux session is named after the agent.
package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	agentport "github.com/flo-at/sindri/internal/adapter/agent"
	"github.com/flo-at/sindri/internal/adapter/tmux"
	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/container"
)

// Inject types text into an agent's tmux session, failing on one that isn't running and on a signed-out
// one whose input box swallows anything typed — a user's own message answers for itself (-> Tell).
func (s *Service) Inject(ctx context.Context, project, name, text string) error {
	return s.inject(ctx, project, name, text, true)
}

// inject is the delivery; guard refuses a signed-out pane, which a human asked the question may
// overrule — the pane is a prediction and theirs can be the better information.
func (s *Service) inject(ctx context.Context, project, name, text string, guard bool) error {
	c := s.deps.ContainerName(project, name)
	if !container.RunningContext(ctx, c) {
		return fmt.Errorf("agent %q is not running — launch it first", name)
	}
	if guard && s.readsSignedOut(ctx, project, name) {
		// Says "READS", never "is": the banner outlives the turn that printed it, so this fires on
		// agents that are working. No restart is offered — the hub makes that move itself when the
		// host's token changes (-> credwatch.revive).
		return fmt.Errorf("agent %q reads signed out — its pane carries a /login banner, and nothing typed at "+
			"that prompt is sent. If the banner is stale it is working normally: `sindri agent tell %s \"…\" "+
			"--anyway` sends regardless. If it is genuinely at the prompt, only a fresh token on the HOST "+
			"fixes it — the hub stages that itself, and restarts the agent when it arrives", name, name)
	}
	if _, err := container.ExecContext(ctx, c, append([]string{"tmux"}, tmux.SendLiteral(name, text)...)...); err != nil {
		return err // the tmux session is the agent name
	}
	// Read back BETWEEN the text and the Enter: tmux exiting 0 says it accepted the keystrokes, not
	// that they reached the pane (-> sd-176312), and this is the one moment they are on screen
	// whatever the message then does — /clear and /compact wipe the pane a later look would use.
	verdict, pane := s.typedIntoPane(ctx, c, name, text)
	// The Enter goes whatever the caller does: a read-back abandoned halfway must not leave a line
	// sitting unsubmitted in the input box, to go later with whatever is typed in front of it.
	submit, cancel := context.WithTimeout(context.WithoutCancel(ctx), probeTimeout)
	defer cancel()
	if _, err := container.ExecContext(submit, c, append([]string{"tmux"}, tmux.Submit(name)...)...); err != nil {
		return err
	}
	switch {
	case verdict == paneShowedIt:
		s.clearStrikes(project, name)
		return nil
	case verdict == paneCannotSay, agentport.Runtime(pane) == string(agentport.Blocked):
		// Neither struck nor cleared, and no error: this send says nothing either way, so it must not
		// erase the evidence of the ones before it. Blocked is read off the capture just taken, never
		// Observe's — that is memoised for runtimeTTL, so a dialog opened inside the window is invisible
		// to it, which is every back-to-back injection. A form eats keystrokes without drawing them, and
		// answering one is what `agent tell` is FOR (-> README).
		return nil
	case verdict == paneUnreadable, ctx.Err() != nil:
		// Nothing was read, or nobody was left to read it for. Reported, since the caller asked and has
		// no answer, but struck against nothing: neither is a fact about the agent.
		return fmt.Errorf("agent %q: the pane could not be read back — %w", name, errUnconfirmed)
	}
	// Reported, never repaired: only the caller knows whether its message may be delivered twice.
	s.strikeUnconfirmed(project, name)
	return fmt.Errorf("agent %q: tmux accepted the keystrokes and the pane never showed them — %w",
		name, errUnconfirmed)
}

// errUnconfirmed marks a send that WAS typed and submitted and whose text never showed. A different
// fact from nothing being typed at all, and the log a user reconstructs from must not conflate them.
var errUnconfirmed = errors.New("nothing confirms the message arrived")

// needleCap is the most of a message worth looking for: past it a slice is no more distinctive, only
// likelier to straddle a row.
const needleCap = 40

// paneChrome is what a row spends on the input box's borders and prompt before the message starts.
// Measured against a bordered box from 9 to 120 columns: the longest matchable prefix ran
// paneWidth-6 at every one, so 8 leaves a row's slack for a tool that draws more.
const paneChrome = 8

// needleFloor is the least distinctive slice worth believing. A pane with less room than this shows
// nothing that could prove anything, so confirmation is SKIPPED there rather than failed — sindri
// creates panes as narrow as 9 columns (-> tui.previewSize), and a false alarm is the one outcome
// worse than the silence this replaces.
const needleFloor = 8

// paneRepaintCap bounds the read-back. Measured on a real pane: 25ms to show typed text on an idle
// host, 59ms with every core spinning — the margin is for the pod exec, not the repaint.
const paneRepaintCap = 3 * time.Second

// confirmPoll spaces the re-reads inside that window; each one costs a container exec.
const confirmPoll = 100 * time.Millisecond

// readback is what looking at the pane settled. FOUR outcomes, because "cannot tell" is not an
// answer and must not be filed as one: recorded as arrival it reports a vanished message as landed
// and wipes the strikes standing against the agent, which is this task's own bug wearing a new face.
type readback int

// paneCannotSay leads so it is the ZERO VALUE: a readback nobody set claims nothing and changes
// nothing, where the obvious ordering would make an unset one mean "the text is on screen".
const (
	paneCannotSay  readback = iota // readable, and too narrow for any slice of it to prove anything
	paneShowedIt                   // the text is on screen
	paneLacksIt                    // a pane wide enough to have shown it, and it did not
	paneUnreadable                 // no capture came back at all
)

// typedIntoPane looks for the text until paneRepaintCap, and hands back the last capture — the only
// reading of this agent from AFTER the keystrokes. A blank capture is no reading rather than a narrow
// pane, so it keeps waiting: a pane that momentarily came back empty must not end the search.
func (s *Service) typedIntoPane(ctx context.Context, c, name, text string) (readback, string) {
	ctx, cancel := context.WithTimeout(ctx, paneRepaintCap)
	defer cancel()
	var pane string
	measured := false
	for {
		out, err := container.ExecContext(ctx, c, append([]string{"tmux"}, tmux.CapturePane(name, 0, false)...)...)
		if err == nil {
			pane = string(out)
			if width := paneWidth(pane); width > 0 {
				measured = true
				needle := paneNeedle(text, width-paneChrome)
				if needle == "" {
					return paneCannotSay, pane // this pane is that narrow, and waiting will not widen it
				}
				if strings.Contains(pane, needle) {
					return paneShowedIt, pane
				}
			}
		}
		select {
		case <-ctx.Done():
			if !measured {
				return paneUnreadable, pane
			}
			return paneLacksIt, pane
		case <-time.After(confirmPoll):
		}
	}
}

// paneWidth is the pane's own width, read off its widest row — 0 for a capture with nothing in it.
// Derived rather than picked, because sindri creates sessions at the TUI's preview width: 29 columns
// on an 80-column terminal, 9 on a 30, where a fixed 40-rune needle would never match anything.
func paneWidth(pane string) int {
	widest := 0
	for line := range strings.SplitSeq(pane, "\n") {
		widest = max(widest, utf8.RuneCountInString(line))
	}
	return widest
}

// tagBytes bounds a leading provenance tag, so a bracket further in is read as message text. A loose
// bound: over-run, the tag stays in the needle and still matches, since it is typed literally too.
const tagBytes = 24

// paneNeedle is the slice of text to look for within one row of budget COLUMNS, "" where this pane
// can show nothing distinctive enough to prove anything. The provenance tag goes, but its width does
// not: it is typed onto that same row, so it spends the budget whether the needle keeps it or not.
// Then everything past the FIRST NEWLINE, which the pane draws as its own row behind the chrome.
func paneNeedle(text string, budget int) string {
	if strings.HasPrefix(text, "[") {
		if i := strings.Index(text, "] "); i > 0 && i < tagBytes {
			budget -= paneColumns(text[:i+2])
			text = text[i+2:]
		}
	}
	if budget < needleFloor {
		return ""
	}
	text, _, _ = strings.Cut(text, "\n")
	limit, width := min(budget, needleCap), 0
	var r []rune
	for _, c := range strings.TrimSpace(text) {
		w := runeColumns(c)
		if width+w > limit {
			break
		}
		r = append(r, c) // RUNES, never bytes: half a character matches no pane
		width += w
	}
	return string(r)
}

// runeColumns is how many terminal columns a rune takes, every non-ASCII one counted as two. Coarse
// on purpose, and only ever an OVER-estimate: it may shorten a needle needlessly, which risks
// confirming a message that vanished — where under-counting would wrap the needle and raise the false
// alarm this must not raise.
func runeColumns(r rune) int {
	if r > unicode.MaxASCII {
		return 2
	}
	return 1
}

// paneColumns is that width for a whole string.
func paneColumns(s string) int {
	n := 0
	for _, r := range s {
		n += runeColumns(r)
	}
	return n
}

// InjectWhenReady waits (briefly) for the tmux session, then injects. It ERRORS when nothing was
// injected — logging the skip used to read as delivered, and a mail row's "pushed" reads this outcome.
func (s *Service) InjectWhenReady(ctx context.Context, project, name, text string) error {
	return s.injectWhenReady(ctx, project, name, text, true)
}

// injectWhenReady leaves the guard to the caller: a restarted agent must not be turned away by a stale pane.
func (s *Service) injectWhenReady(ctx context.Context, project, name, text string, guard bool) error {
	c := s.deps.ContainerName(project, name)
	for i := 0; i < 25; i++ {
		if container.RunningContext(ctx, c) {
			if _, err := container.ExecContext(ctx, c, "tmux", "has-session", "-t", name); err == nil {
				err := s.inject(ctx, project, name, text, guard)
				if err != nil {
					// Recorded, not dropped: the log is where a user reconstructs what an agent was never
					// told — and "typed, never seen" is not "never typed", so the two get their own words.
					kind := "inject-skipped"
					if errors.Is(err, errUnconfirmed) {
						kind = "inject-unconfirmed"
					}
					_ = s.store.For(project).Log(name, kind, text)
				}
				return err
			}
		}
		// The wait is the caller's to abandon: a cancelled request has nobody left to deliver to.
		select {
		case <-ctx.Done():
			_ = s.store.For(project).Log(name, "inject-skipped", text)
			return fmt.Errorf("agent %q: %w — nothing was injected", name, ctx.Err())
		case <-time.After(200 * time.Millisecond):
		}
	}
	_ = s.store.For(project).Log(name, "inject-skipped", text)
	return fmt.Errorf("agent %q has no live session — nothing was injected", name)
}

// unreachableStrikes is the run of unconfirmed pushes the board waits for, against one late redraw.
const unreachableStrikes = 3

// reachMemo counts consecutive unconfirmed pushes. A Service field for the same reason runtimeMemo is.
type reachMemo struct {
	mu      sync.Mutex
	strikes map[lcKey]int
}

// clearStrikes forgets the count: a push just landed, or a fresh pod is starting (-> Launch).
func (s *Service) clearStrikes(project, name string) {
	s.reach.mu.Lock()
	defer s.reach.mu.Unlock()
	delete(s.reach.strikes, lcKey{project, name})
}

// strikeUnconfirmed counts a push that never showed, logging the reason as the run reaches the word.
func (s *Service) strikeUnconfirmed(project, name string) {
	s.reach.mu.Lock()
	if s.reach.strikes == nil {
		s.reach.strikes = map[lcKey]int{}
	}
	key := lcKey{project, name}
	s.reach.strikes[key]++
	n := s.reach.strikes[key]
	s.reach.mu.Unlock()
	if n == unreachableStrikes {
		_ = s.store.For(project).Log(name, "unreachable",
			fmt.Sprintf("%d pushes in a row never appeared in its pane", n))
		s.deps.Notify()
	}
}

// Unreachable reports an agent whose last unreachableStrikes pushes all went unconfirmed — the board's
// own word, beside signed-out and down: up, and nothing said to it can be shown to have arrived.
func (s *Service) Unreachable(project, name string) bool {
	s.reach.mu.Lock()
	defer s.reach.mu.Unlock()
	return s.reach.strikes[lcKey{project, name}] >= unreachableStrikes
}

// Interrupt sends Escape — "abort the current operation" — so a follow-up lands on an idle prompt
// rather than queueing behind in-flight work. Errors on an agent that isn't running.
func (s *Service) Interrupt(ctx context.Context, project, name string) error {
	c := s.deps.ContainerName(project, name)
	if !container.RunningContext(ctx, c) {
		return fmt.Errorf("agent %q is not running", name)
	}
	full := append([]string{"tmux"}, tmux.Interrupt(name)...)
	_, err := container.ExecContext(ctx, c, full...)
	return err
}

// Tell delivers a source-stamped message (provenance, D12) and logs it. signedOut carries the sender's
// answer to a signed-out pane; PUSH-ONLY by decision, so a failure reaches whoever typed it.
func (s *Service) Tell(ctx context.Context, project, name, msg, source, signedOut string) error {
	ps := s.store.For(project)
	if _, ok, err := ps.GetAgent(name); err != nil {
		return err
	} else if !ok {
		return fmt.Errorf("no such agent %q", name)
	}
	if source == "" {
		source = "user"
	}
	stamped := fmt.Sprintf("[%s] %s", source, msg)
	if err := s.deliver(ctx, project, name, stamped, signedOut); err != nil {
		return err
	}
	defer s.deps.Notify()
	return ps.Log(name, "recv", stamped)
}

// deliver honours the sender's answer about a signed-out pane, carrying out the restart the refusal
// names. It applies only where the pane really reads signed out, so a fine session is never bounced.
func (s *Service) deliver(ctx context.Context, project, name, stamped, signedOut string) error {
	switch signedOut {
	case api.SignedOutSend, api.SignedOutRestart:
		if !s.readsSignedOut(ctx, project, name) {
			break
		}
		if signedOut == api.SignedOutRestart {
			if err := s.RestartAgent(ctx, project, name, io.Discard); err != nil {
				return fmt.Errorf("restarting %s to deliver the message: %w", name, err)
			}
			return s.injectWhenReady(ctx, project, name, stamped, false)
		}
		return s.inject(ctx, project, name, stamped, false)
	}
	return s.Inject(ctx, project, name, stamped)
}

// readsSignedOut is the pane's verdict on whether typing is sent — an observation a user may overrule.
func (s *Service) readsSignedOut(ctx context.Context, project, name string) bool {
	return s.RuntimeState(ctx, project, name) == string(agentport.SignedOut)
}
