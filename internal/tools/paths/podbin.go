// package: paths / podbin
// type:    logic (filesystem locations)
// job:     locate the pod-bin directory mounted into every agent pod, and name the
// host-built tools it carries.
// limits:  paths and names only; filling it is the hub's (-> hub/agent/podbin.go).
package paths

import "path/filepath"

// PodBinDir is the one directory bind-mounted into every agent pod. It is the hub's own,
// not the install prefix: mounting ~/.local/bin would expose every binary kept there.
func PodBinDir() string { return filepath.Join(StateDir(), "pod-bin") }

// PodBinMount is where PodBinDir appears in the pod; the image's /usr/local/bin symlinks
// point here, so a refreshed tool resolves on the next exec.
const PodBinMount = "/opt/sindri/host-bin"

// AgentHomeDir is one agent's coding-agent home, mounted into its pod. Named here because the
// launcher writes it and the credential upkeep reads it, and a second spelling would silently
// refresh a directory nothing mounts.
func AgentHomeDir(project, agent string) string {
	return filepath.Join(StateDir(), project, "agents", agent)
}

// PodTool names one tool: the host filename to copy from, and the name the agent invokes.
type PodTool struct {
	HostName string
	PodName  string
}

// PodTools is what pod-bin carries. brokkr drops its -linux suffix because the pod is
// linux whatever the host is.
var PodTools = []PodTool{
	{HostName: "brokkr-linux", PodName: "brokkr"},
	{HostName: "sindri-worker", PodName: "sindri-worker"},
}
