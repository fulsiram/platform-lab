package commands

import (
	"fmt"
	"strings"
	"time"

	"github.com/fulsiram/platform-lab/tools/salami-cli/internal/auth"
	"github.com/spf13/cobra"
)

type loginOptions struct {
	ListenAddress string
	NoBrowser     bool
	Timeout       time.Duration
}

func NewAuthCmd(global *globalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Authenticate to the Salami Kubernetes platform",
	}

	loginOpts := &loginOptions{}
	loginCmd := &cobra.Command{
		Use:   "login",
		Short: "Log in with Keycloak OIDC",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := global.config(cmd)
			if err != nil {
				return err
			}
			manager := auth.NewManager(cfg, cmd.ErrOrStderr())
			_, claims, err := manager.Login(cmd.Context(), auth.LoginOptions{
				ListenAddress: loginOpts.ListenAddress,
				NoBrowser:     loginOpts.NoBrowser,
				Timeout:       loginOpts.Timeout,
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "\nLogged in as oidc:%s\n", claims.Email)
			if len(claims.Groups) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "Groups: %s\n", strings.Join(prefixGroups(claims.Groups), ", "))
			}
			return nil
		},
	}
	loginCmd.Flags().StringVar(&loginOpts.ListenAddress, "listen-address", "127.0.0.1:0", "Local address for the OIDC callback listener")
	loginCmd.Flags().BoolVar(&loginOpts.NoBrowser, "no-browser", false, "Print the login URL without opening a browser")
	loginCmd.Flags().DurationVar(&loginOpts.Timeout, "timeout", 5*time.Minute, "Maximum time to wait for the browser login callback")

	tokenCmd := &cobra.Command{
		Use:   "token",
		Short: "Print a Kubernetes ExecCredential",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := global.config(cmd)
			if err != nil {
				return err
			}
			manager := auth.NewManager(cfg, cmd.ErrOrStderr())
			token, _, err := manager.Token(cmd.Context())
			if err != nil {
				return err
			}
			return auth.WriteExecCredential(cmd.OutOrStdout(), token)
		},
	}

	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show cached OIDC login status",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := global.config(cmd)
			if err != nil {
				return err
			}
			manager := auth.NewManager(cfg, cmd.ErrOrStderr())
			status, err := manager.Status()
			if err != nil {
				return err
			}
			if !status.Authenticated {
				fmt.Fprintf(cmd.OutOrStdout(), "Not logged in\nToken cache: %s\n", status.TokenPath)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Logged in as oidc:%s\n", status.Claims.Email)
			fmt.Fprintf(cmd.OutOrStdout(), "Issuer: %s\n", status.Claims.Issuer)
			fmt.Fprintf(cmd.OutOrStdout(), "Audience: %s\n", strings.Join(status.Claims.Aud, ", "))
			fmt.Fprintf(cmd.OutOrStdout(), "Groups: %s\n", strings.Join(prefixGroups(status.Claims.Groups), ", "))
			fmt.Fprintf(cmd.OutOrStdout(), "Expires: %s\n", status.Claims.ExpirationTime().Format(time.RFC3339))
			if status.NeedsRefresh {
				fmt.Fprintln(cmd.OutOrStdout(), "Refresh: needed")
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), "Refresh: not needed")
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Token cache: %s\n", status.TokenPath)
			return nil
		},
	}

	logoutCmd := &cobra.Command{
		Use:   "logout",
		Short: "Remove cached OIDC tokens",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := global.config(cmd)
			if err != nil {
				return err
			}
			manager := auth.NewManager(cfg, cmd.ErrOrStderr())
			if err := manager.Logout(); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Logged out")
			return nil
		},
	}

	cmd.AddCommand(loginCmd, tokenCmd, statusCmd, logoutCmd)
	return cmd
}

func prefixGroups(groups []string) []string {
	out := make([]string, 0, len(groups))
	for _, group := range groups {
		if strings.HasPrefix(group, "oidc:") {
			out = append(out, group)
			continue
		}
		out = append(out, "oidc:"+group)
	}
	return out
}
