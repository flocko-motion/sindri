// package: api / misc
// type:    data (wire types + a pure function)
// job:     small standalone wire types that don't warrant their own file, plus
// RepoTag: the protocol's project key, derived from a path so either side
// can compute it without asking the other.
// limits:  data and a pure function only.
package api

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
)

// RepoTag is a short, stable per-repo id derived from its absolute path — the key
// (AgentView.Project, BoardState.Tasks's scope) every request and view carries to
// name a repo without relying on its basename, which two repos can share.
func RepoTag(root string) string {
	abs, err := filepath.Abs(root)
	if err != nil {
		abs = root
	}
	sum := sha256.Sum256([]byte(abs))
	return hex.EncodeToString(sum[:4]) // 8 hex chars — plenty to separate repos
}

// Comment is one task comment, tagged with the source it came from and the
// external reference that identifies it there (so a re-sync can match it).
type Comment struct {
	Source    string `json:"source"`     // "github", or "td" on a thread synced before the import
	SourceRef string `json:"source_ref"` // external id / url, unique within the source
	Author    string `json:"author"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"` // RFC3339
}

// Event is one row of the append-only activity log.
type Event struct {
	ID      int64  `json:"id"`
	Project string `json:"project"`
	Agent   string `json:"agent"`
	TS      string `json:"ts"`
	Type    string `json:"type"`
	Payload string `json:"payload"`
}

// CmdInfo is a command as advertised to a browser (name + help).
type CmdInfo struct {
	Name string `json:"name"`
	Help string `json:"help"`
}

// ClientView is one dial-in on an agent's tmux session; orphaned attaches show up here too.
type ClientView struct {
	TTY      string `json:"tty"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	ReadOnly bool   `json:"read_only"`
}

// ExecReq is the body for POST /exec on an agent channel.
type ExecReq struct {
	Args []string `json:"args"`
}
