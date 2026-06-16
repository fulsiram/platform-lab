package commands

import (
	"fmt"

	"github.com/fulsiram/platform-lab/tools/salami-cli/internal/auth"
	"github.com/fulsiram/platform-lab/tools/salami-cli/internal/kube"
	"github.com/fulsiram/platform-lab/tools/salami-cli/internal/sshkeys"
	"github.com/spf13/cobra"
	"k8s.io/client-go/kubernetes"
)

type sshKeyFileOptions struct {
	File string
}

func NewKeysCmd(global *globalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "keys",
		Aliases: []string{"ssh-key", "ssh-keys"},
		Short:   "Manage SSH authorized keys",
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List SSH authorized keys",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			target, err := sshKeyTarget(cmd, global)
			if err != nil {
				return err
			}
			contents, _, err := sshkeys.GetContents(cmd.Context(), target.Client, target.Namespace, target.Name)
			if err != nil {
				return err
			}
			return sshkeys.WriteAuthorizedKeys(cmd.OutOrStdout(), contents)
		},
	}

	addOpts := &sshKeyFileOptions{}
	addCmd := &cobra.Command{
		Use:   "add [PUBLIC_KEY]",
		Short: "Add an SSH public key",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target, err := sshKeyTarget(cmd, global)
			if err != nil {
				return err
			}
			keyArg := ""
			if len(args) > 0 {
				keyArg = args[0]
			}
			keyInput, err := sshkeys.ReadKeyInput(keyArg, addOpts.File)
			if err != nil {
				return err
			}
			contents, _, err := sshkeys.GetContents(cmd.Context(), target.Client, target.Namespace, target.Name)
			if err != nil {
				return err
			}
			next, key, added, err := sshkeys.AddKey(contents, keyInput)
			if err != nil {
				return err
			}
			if !added {
				fmt.Fprintf(cmd.OutOrStdout(), "Key already present: %s\n", key.Line)
				return nil
			}
			if err := sshkeys.UpsertContents(cmd.Context(), target.Client, target.Namespace, target.Name, next); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Added key: %s\n", key.Line)
			fmt.Fprintf(cmd.OutOrStdout(), "ConfigMap: %s/%s\n", target.Namespace, target.Name)
			return nil
		},
	}
	addCmd.Flags().StringVar(&addOpts.File, "file", "", "Read an SSH public key from a file")

	removeOpts := &sshKeyFileOptions{}
	removeCmd := &cobra.Command{
		Use:   "remove [PUBLIC_KEY]",
		Short: "Remove an SSH public key",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target, err := sshKeyTarget(cmd, global)
			if err != nil {
				return err
			}
			keyArg := ""
			if len(args) > 0 {
				keyArg = args[0]
			}
			keyInput, err := sshkeys.ReadKeyInput(keyArg, removeOpts.File)
			if err != nil {
				return err
			}
			contents, found, err := sshkeys.GetContents(cmd.Context(), target.Client, target.Namespace, target.Name)
			if err != nil {
				return err
			}
			if !found {
				return fmt.Errorf("ConfigMap %s/%s does not exist", target.Namespace, target.Name)
			}
			next, key, removed, err := sshkeys.RemoveKey(contents, keyInput)
			if err != nil {
				return err
			}
			if !removed {
				return fmt.Errorf("SSH key is not present")
			}
			if err := sshkeys.UpsertContents(cmd.Context(), target.Client, target.Namespace, target.Name, next); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Removed key: %s\n", key.Line)
			return nil
		},
	}
	removeCmd.Flags().StringVar(&removeOpts.File, "file", "", "Read an SSH public key from a file")

	setOpts := &sshKeyFileOptions{}
	setCmd := &cobra.Command{
		Use:   "set --file AUTHORIZED_KEYS",
		Short: "Replace SSH authorized keys from a file",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if setOpts.File == "" {
				return fmt.Errorf("--file is required")
			}
			target, err := sshKeyTarget(cmd, global)
			if err != nil {
				return err
			}
			contents, err := sshkeys.ReadKeyInput("", setOpts.File)
			if err != nil {
				return err
			}
			normalized, keys, err := sshkeys.ValidateAuthorizedKeys(contents)
			if err != nil {
				return err
			}
			if err := sshkeys.UpsertContents(cmd.Context(), target.Client, target.Namespace, target.Name, normalized); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Wrote %d SSH key(s)\n", len(keys))
			fmt.Fprintf(cmd.OutOrStdout(), "ConfigMap: %s/%s\n", target.Namespace, target.Name)
			return nil
		},
	}
	setCmd.Flags().StringVar(&setOpts.File, "file", "", "Read authorized_keys contents from a file")

	cmd.AddCommand(listCmd, addCmd, removeCmd, setCmd)
	return cmd
}

type sshKeyTargetInfo struct {
	Namespace string
	Name      string
	Client    kubernetes.Interface
}

func sshKeyTarget(cmd *cobra.Command, global *globalOptions) (sshKeyTargetInfo, error) {
	cfg, err := global.config(cmd)
	if err != nil {
		return sshKeyTargetInfo{}, err
	}
	manager := auth.NewManager(cfg, cmd.ErrOrStderr())
	token, claims, err := manager.Token(cmd.Context())
	if err != nil {
		return sshKeyTargetInfo{}, err
	}
	name, err := sshkeys.DefaultConfigMapName(claims.Email)
	if err != nil {
		return sshKeyTargetInfo{}, err
	}
	client, err := kube.KubernetesClient(cfg, token.IDToken)
	if err != nil {
		return sshKeyTargetInfo{}, err
	}
	return sshKeyTargetInfo{
		Namespace: cfg.Namespace,
		Name:      name,
		Client:    client,
	}, nil
}
