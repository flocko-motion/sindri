// package: hub/flow/fleet / show
// type:    logic (one verb over three subjects)
// job:     send "show" to whichever subject owns the id it was given — a run, a mail, a pull
// request — refusing an id shape none of them owns before any of them is asked.
// limits:  the dispatch. Each subject renders its own.
package fleet

import (
	"fmt"
	"io"
	"strings"

	"github.com/flo-at/sindri/internal/api"
	"github.com/flo-at/sindri/internal/hub/api/agents/registry"
)

// showUsage is what every unrecognised shape gets, so a typo reads as "here is the grammar" rather
// than a lookup that was doomed before it ran.
const showUsage = "usage: show <pr-id> | <run-id> | <mail-id>"

// CmdShow dispatches "show" by id shape — run-, ml- or pr-. Anything else is refused HERE, by shape:
// a fallthrough to CmdShowPR once turned "show ml-465" into an opaque internal error (-> AgentExec).
func (e *Engine) CmdShow(c registry.Caller, args []string, out io.Writer) (int, error) {
	if len(args) == 0 {
		fmt.Fprintln(out, showUsage)
		return 2, nil
	}
	switch {
	case strings.HasPrefix(args[0], "run-"):
		return e.runAct().CmdShowRun(c, args, out)
	case strings.HasPrefix(args[0], api.MailIDPrefix):
		return e.Mail.CmdShowMail(c, args, out)
	case strings.HasPrefix(args[0], "pr-"):
		return e.prAct().CmdShowPR(c, args, out)
	}
	fmt.Fprintf(out, "%q is none of those.\n%s\n", args[0], showUsage)
	return 2, nil
}
