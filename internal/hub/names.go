// package: hub / names
// type:    headless helper
// job:     auto-name new agents after Norse dwarves — friends of Sindri the
//          smith. "sindri" itself is never handed out.
// limits:  only allocates names; registering and launching the agent are the
//          hub's (-> hub.go).
package hub

import "fmt"

// dwarfNames are Norse dwarves (Dvergatal + the smith-brothers), lowercased for
// use as agent/workspace names. Deliberately excludes "sindri".
var dwarfNames = []string{
	"brokkr", "eitri", "dvalin", "durin", "dain", "nain", "fjalar", "galar",
	"alviss", "andvari", "regin", "bifur", "bofur", "bombur", "dori", "nori",
	"ori", "fili", "kili", "gloin", "oin", "balin", "dwalin", "thorin",
	"thrain", "fundin", "nyi", "nidi", "nordri", "sudri", "austri", "vestri",
	"frar", "loni", "jari", "hepti", "nar", "lit",
}

// autoName returns the first unused dwarf name; once the pool is exhausted it
// appends a numeric suffix (brokkr2, eitri2, …) so creation never fails.
func (h *Hub) autoName() (string, error) {
	roster, err := h.store.Roster()
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
