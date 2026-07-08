// package: hub / agent_rebuild
// type:    logic (agent image refresh)
// job:     RebuildAgent — force-rebuild the agent's image (re-pulling the base, so a
//          newer Go is picked up) and relaunch into it. The Claude session resumes
//          from the mounted ~/.claude (`claude --continue`), so nothing is lost.
// limits:  orchestration only; the build lives in internal/container, the relaunch
//          in Launch (hub.go).
package hub

import (
	"fmt"
	"io"

	"github.com/flo-at/sindri/internal/config"
	"github.com/flo-at/sindri/internal/container"
)

// RebuildAgent rebuilds the agent's image (re-pull base) then relaunches it,
// streaming build/restart progress to w. A bad project config fails loudly before
// any build; a running agent is stopped first so it comes up on the fresh image.
func (h *Hub) RebuildAgent(project, name string, w io.Writer) error {
	ps := h.store.For(project)
	root := h.projectRoot(project)
	if _, ok, err := ps.GetAgent(name); err != nil {
		return err
	} else if !ok {
		return fmt.Errorf("no such agent %q", name)
	}
	cfg, err := h.projectConfig(project)
	if err != nil {
		return err
	}
	if _, err := container.RebuildImage(root, config.Abs(root, cfg.Containerfile), w); err != nil {
		return err
	}
	if container.Running(h.container(project, name)) {
		fmt.Fprintf(w, "Image rebuilt — restarting %s to run it (the session resumes)…\n", name)
		if err := h.StopAgent(project, name); err != nil {
			return err
		}
	}
	return h.Launch(project, name, false, false, w)
}
