// package: api / tier
// type:    logic (a wire value + the pure rule behind it)
// job:     the difficulty tier a task carries — junior/mid/senior, THE ONE TABLE both front-ends
// and the hub parse against, and its default (TierOrDefault). Which model a tier maps onto
// is the dispatcher's, once it exists.
// limits:  the vocabulary and its default only; no persistence, no rendering.
package api

// TierWords are the assignable tiers, cheapest first.
var TierWords = []string{"junior", "mid", "senior"}

// ParseTier reads a tier off a flag or an agent's command; ok is false for anything else.
func ParseTier(word string) (tier string, ok bool) {
	for _, w := range TierWords {
		if w == word {
			return word, true
		}
	}
	return "", false
}

// TierOrDefault resolves a task's stored tier, defaulting the unset ("") case to mid — not senior,
// which would put every unrated task in the existing backlog on the most expensive worker, and not
// junior either, since a task nobody rated is not evidence that it is easy.
func TierOrDefault(tier string) string {
	if tier == "" {
		return "mid"
	}
	return tier
}
