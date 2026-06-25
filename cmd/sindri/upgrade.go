// package: main (sindri) / upgrade
// type:    command
// job:     wires `sindri upgrade` — an explicit, on-demand check for a newer
//          release (no daily throttle). If one exists it points you at the
//          generated `sindri-do-upgrade` helper, which does the actual install.
// limits:  only checks + recommends; it can't replace the running binary itself
//          (-> internal/update writes sindri-do-upgrade for that).
package main

import (
	"github.com/flo-at/sindri/internal/update"
	"github.com/spf13/cobra"
)

func newUpgradeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "upgrade",
		Short: "Check for a newer release; if found, recommend running sindri-do-upgrade",
		Args:  cobra.NoArgs,
		// A network/check failure is a runtime error, not a usage mistake — don't
		// dump the usage block after it.
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return update.Upgrade(version, cmd.OutOrStdout())
		},
	}
}
