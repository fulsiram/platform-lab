package commands

import (
	"fmt"

	"github.com/fulsiram/platform-lab/tools/salami-cli/internal/appconfig"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/util/validation"
)

func NewNamespaceCmd(global *globalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "namespace",
		Short: "Manage the default Kubernetes namespace",
	}

	getCmd := &cobra.Command{
		Use:   "get",
		Short: "Print the effective namespace",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := global.config(cmd)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), cfg.Namespace)
			return nil
		},
	}

	setCmd := &cobra.Command{
		Use:   "set NAMESPACE",
		Short: "Persist the default namespace",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if errs := validation.IsDNS1123Label(args[0]); len(errs) > 0 {
				return fmt.Errorf("invalid namespace %q: %s", args[0], errs[0])
			}
			userCfg, path, err := appconfig.LoadUserConfig(global.ConfigPath)
			if err != nil {
				return err
			}
			userCfg.Namespace = args[0]
			if _, err := appconfig.Normalize(userCfg); err != nil {
				return err
			}
			if err := appconfig.SaveUserConfig(path, userCfg); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Namespace set to %s\n", args[0])
			fmt.Fprintf(cmd.OutOrStdout(), "Config: %s\n", path)
			return nil
		},
	}

	unsetCmd := &cobra.Command{
		Use:   "unset",
		Short: "Remove the persisted namespace",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			userCfg, path, err := appconfig.LoadUserConfig(global.ConfigPath)
			if err != nil {
				return err
			}
			userCfg.Namespace = ""
			if err := appconfig.SaveUserConfig(path, userCfg); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Namespace unset; default is %s\n", appconfig.DefaultNamespace)
			fmt.Fprintf(cmd.OutOrStdout(), "Config: %s\n", path)
			return nil
		},
	}

	cmd.AddCommand(getCmd, setCmd, unsetCmd)
	return cmd
}
