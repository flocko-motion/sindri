// package: ui/cli / upgrade
// type:    command
// job:     wires `sindri upgrade` — an explicit, on-demand check for a newer
//          release (no daily throttle). If one exists it points you at the
//          generated `sindri-do-upgrade` helper, which does the actual install.
// limits:  only checks + recommends; it can't replace the running binary itself
//          (-> internal/update writes sindri-do-upgrade for that).
package cli

import (
	"github.com/flo-at/sindri/internal/update"
	"github.com/spf13/cobra"
)

// NewUpgradeCmd builds the `upgrade` command (self-update to the latest release).
func NewUpgradeCmd() *cobra.Command {
	var list bool
	c := &cobra.Command{
		Use:   "upgrade [version]",
		Short: "Upgrade sindri: latest by default, or a specific released version",
		Long: "With no argument, check the latest release and (if newer) recommend\n" +
			"`sindri-do-upgrade`. Pass a version to install exactly that release —\n" +
			"including a reinstall or a downgrade. `--list` shows available versions.\n\n" +
			"  sindri upgrade            # latest\n" +
			"  sindri upgrade v0.12.0    # a specific release (a 'v' prefix is optional)\n" +
			"  sindri upgrade --list     # list published releases",
		Args: cobra.MaximumNArgs(1),
		// A network/check failure is a runtime error, not a usage mistake — don't
		// dump the usage block after it.
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if list {
				return update.ListReleases(cmd.OutOrStdout())
			}
			target := ""
			if len(args) == 1 {
				target = args[0]
			}
			return update.Upgrade(version, target, cmd.OutOrStdout())
		},
	}
	c.Flags().BoolVar(&list, "list", false, "list published releases you can upgrade (or downgrade) to")
	return c
}
