package workspace

import (
	"fmt"
	"math/rand"
	"strings"
)

// Norse dwarf and mythological names — fitting for Sindri's forge.
var norseNames = []string{
	"brokkr",
	"dvalin",
	"alviss",
	"andvari",
	"eitri",
	"fjalar",
	"galar",
	"hreidmar",
	"ivaldi",
	"lit",
	"nain",
	"nordri",
	"sudri",
	"austri",
	"vestri",
	"regin",
	"otr",
	"motsoenir",
	"durin",
	"nyi",
	"nithi",
	"vigg",
	"gandalf",
	"vindalf",
	"thorin",
	"fili",
	"kili",
	"bombur",
	"nori",
	"ori",
	"draupnir",
	"dolgthvari",
	"haur",
	"hugstari",
	"hledgjalf",
	"gloin",
	"dori",
	"bifur",
	"bofur",
	"brokk",
	"ai",
}

// nextNorseName picks a random unused Norse name, given a list of already-taken names.
// Falls back to "worker-N" if all names are exhausted.
func nextNorseName(taken []string) string {
	takenSet := make(map[string]bool, len(taken))
	for _, n := range taken {
		takenSet[strings.ToLower(n)] = true
	}

	var available []string
	for _, name := range norseNames {
		if !takenSet[name] {
			available = append(available, name)
		}
	}

	if len(available) == 0 {
		for i := 1; ; i++ {
			candidate := fmt.Sprintf("worker-%d", i)
			if !takenSet[candidate] {
				return candidate
			}
		}
	}

	return available[rand.Intn(len(available))]
}
