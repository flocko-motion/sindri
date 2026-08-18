// package: ui/cli / agent standing
// type:    command (host CLI)
// job:     the `sindri agent` verbs that change an agent's STANDING without touching its
// pod — retire it from new work, release the escalation it stopped on, arm or
// cancel a context clear. Each holds or releases a running agent.
// limits:  no logic; every verb marshals to the hub via the backend port. The pod's own
// lifecycle (start/stop/delete/rebuild) and the read views live in agent.go.
package cli

import (
	"fmt"
	"os"

	"github.com/flo-at/sindri/internal/api"
	"github.com/spf13/cobra"
)

// agentRetireCmd winds an agent down without interrupting it: the point is to stop it AFTER the work
// in hand, so the pod keeps running and only the next assignment is withheld.
func agentRetireCmd() *cobra.Command {
	var back bool
	c := &cobra.Command{
		Use: "retire <name>", Short: "Assign this agent no further work (it finishes what it holds)", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withAgent(args[0], func(b backend, a *api.AgentView) error {
				if err := b.SetRetired(a.Name, !back); err != nil {
					return err
				}
				if back {
					fmt.Fprintf(os.Stderr, "%s takes work again\n", a.Name)
					return nil
				}
				held := "it holds nothing, so it is done now"
				if a.Task != "" || a.Feature != "" || a.PR != "" {
					held = "it will finish what it holds first"
				}
				fmt.Fprintf(os.Stderr, "%s retired: no new work — %s. Stop it with 'sindri agent stop %s', "+
					"or bring it back with 'sindri agent retire %s --back'\n", a.Name, held, a.Name, a.Name)
				return nil
			})
		},
	}
	c.Flags().BoolVar(&back, "back", false, "put the agent back in service")
	return c
}

// agentResumeCmd clears an escalation from the host. Ordinarily the agent clears its own once it has
// the answer — only it knows it understood — so this is the release for one that cannot: an agent
// restarted, deleted, or simply wrong that it was blocked.
func agentResumeCmd() *cobra.Command {
	return &cobra.Command{
		Use: "resume <name>", Short: "Clear an agent's escalation yourself (it normally clears its own)", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withAgent(args[0], func(b backend, a *api.AgentView) error {
				if a.Escalation == "" {
					fmt.Fprintf(os.Stderr, "%s isn't escalated — nothing to clear\n", a.Name)
					return nil
				}
				if err := b.ResumeAgent(a.Name); err != nil {
					return err
				}
				fmt.Fprintf(os.Stderr, "cleared %s's escalation — tell it your answer with "+
					"'sindri agent tell %s \"<answer>\"' if you haven't\n", a.Name, a.Name)
				return nil
			})
		},
	}
}

// agentClearContextCmd arms a context clear — the remedy for a full agent. The user typing this
// command IS the confirmation (the convention `agent delete` uses for its own irreversible action,
// no extra prompt on top of it). WHEN it lands is the agent's answer, not a flag: it fires at the
// next leaf boundary, which for an agent holding nothing is now.
func agentClearContextCmd() *cobra.Command {
	var cancel bool
	c := &cobra.Command{
		Use: "clear-context <name>", Short: "Clear the agent's session at its next leaf boundary (--cancel takes it back)", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withAgent(args[0], func(b backend, a *api.AgentView) error {
				if err := b.SetClearArmed(a.Name, !cancel); err != nil {
					return err
				}
				if cancel {
					fmt.Fprintf(os.Stderr, "%s: context clear cancelled\n", a.Name)
					return nil
				}
				fmt.Fprintf(os.Stderr, "%s: %s. Take it back with 'sindri agent clear-context %s --cancel'\n",
					a.Name, clearLandsWhen(*a), a.Name)
				return nil
			})
		},
	}
	c.Flags().BoolVar(&cancel, "cancel", false, "disarm a clear that has not fired yet")
	return c
}

// clearLandsWhen words api.ClearWaitsFor for a listing: what it waits on comes from the rule, and
// only the sentence is this front-end's.
func clearLandsWhen(a api.AgentView) string {
	held := api.ClearWaitsFor(a)
	switch {
	case held == "":
		return "context cleared now — it picks up its directive fresh"
	case a.Role == "reviewer":
		return "context clear armed — it fires when it delivers its verdict on " + held
	default:
		return "context clear armed — it fires when it finishes " + held
	}
}
