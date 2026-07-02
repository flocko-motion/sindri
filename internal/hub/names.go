// package: hub / names
// type:    headless helper
// job:     auto-name new agents after Norse dwarves — friends of Sindri the
//          smith. The binary names "sindri" and "brokkr" are never handed out.
// limits:  only allocates names; registering and launching the agent are the
//          hub's (-> hub.go).
package hub

import (
	"fmt"

	"github.com/flo-at/sindri/internal/hub/store"
)

// dwarfNames are Norse dwarves (Dvergatal + the smith Eitri), lowercased for use
// as agent/workspace names. Deliberately excludes "sindri" and "brokkr" — those
// are the two binaries (the orchestrator and its toolbelt).
var dwarfNames = []string{
	"eitri", "dvalin", "durin", "dain", "nain", "fjalar", "galar",
	"alviss", "andvari", "regin", "bifur", "bofur", "bombur", "dori", "nori",
	"ori", "fili", "kili", "gloin", "oin", "balin", "dwalin", "thorin",
	"thrain", "fundin", "nyi", "nidi", "nordri", "sudri", "austri", "vestri",
	"frar", "loni", "jari", "hepti", "nar", "lit",
}

// autoName returns the first dwarf name unused in the given project; once the pool
// is exhausted it appends a numeric suffix (brokkr2, eitri2, …) so creation never
// fails. Names are unique per project, so two repos may each have an "eitri".
func (h *Hub) autoName(ps *store.ProjectStore) (string, error) {
	roster, err := ps.Roster()
	if err != nil {
		return "", err
	}
	taken := make(map[string]bool, len(roster))
	for _, a := range roster {
		taken[a.Name] = true
	}
	for _, n := range dwarfNames {
		if !taken[n] {
			return n, nil
		}
	}
	for i := 2; ; i++ {
		for _, n := range dwarfNames {
			if cand := fmt.Sprintf("%s%d", n, i); !taken[cand] {
				return cand, nil
			}
		}
	}
}
