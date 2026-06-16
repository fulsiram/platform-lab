package commands

import (
	"strings"

	"github.com/fulsiram/platform-lab/tools/salami-cli/internal/appconfig"
	"github.com/spf13/cobra"
)

type globalOptions struct {
	ConfigPath               string
	APIServer                string
	IssuerURL                string
	OIDCClientID             string
	CertificateAuthority     string
	CertificateAuthorityData string
	Namespace                string
	OIDCScopes               []string
}

func NewRootCmd() (*cobra.Command, error) {
	opts := &globalOptions{}
	cmd := &cobra.Command{
		Use:           "salami",
		Short:         "Salami platform CLI",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	flags := cmd.PersistentFlags()
	flags.StringVar(&opts.ConfigPath, "config", "", "Path to salami config file")
	flags.StringVar(&opts.APIServer, "api-server", "", "Kubernetes API server URL")
	flags.StringVar(&opts.IssuerURL, "issuer-url", "", "OIDC issuer URL")
	flags.StringVar(&opts.OIDCClientID, "oidc-client-id", "", "OIDC client ID")
	flags.StringVar(&opts.CertificateAuthority, "certificate-authority", "", "Path to Kubernetes API server CA PEM")
	flags.StringVar(&opts.CertificateAuthorityData, "certificate-authority-data", "", "Kubernetes API server CA PEM or base64-encoded PEM")
	flags.StringVarP(&opts.Namespace, "namespace", "n", "", "Kubernetes namespace")
	flags.StringSliceVar(&opts.OIDCScopes, "oidc-scope", nil, "OIDC scope to request; may be repeated")

	cmd.AddCommand(NewAuthCmd(opts))
	cmd.AddCommand(NewNamespaceCmd(opts))
	cmd.AddCommand(NewDevboxCmd(opts))
	cmd.AddCommand(NewKeysCmd(opts))

	return cmd, nil
}

func (o *globalOptions) config(cmd *cobra.Command) (appconfig.Config, error) {
	cfg, err := appconfig.Load(o.ConfigPath)
	if err != nil {
		return appconfig.Config{}, err
	}
	overrides := appconfig.Config{}
	if flagChanged(cmd, "api-server") {
		overrides.APIServer = o.APIServer
	}
	if flagChanged(cmd, "issuer-url") {
		overrides.IssuerURL = o.IssuerURL
	}
	if flagChanged(cmd, "oidc-client-id") {
		overrides.OIDCClientID = o.OIDCClientID
	}
	if flagChanged(cmd, "certificate-authority") {
		overrides.CertificateAuthority = o.CertificateAuthority
	}
	if flagChanged(cmd, "certificate-authority-data") {
		overrides.CertificateAuthorityData = o.CertificateAuthorityData
	}
	if flagChanged(cmd, "namespace") {
		overrides.Namespace = o.Namespace
	}
	if flagChanged(cmd, "oidc-scope") {
		overrides.Scopes = compactStrings(o.OIDCScopes)
	}
	return appconfig.Normalize(appconfig.Merge(cfg, overrides))
}

func flagChanged(cmd *cobra.Command, name string) bool {
	flag := cmd.Flag(name)
	return flag != nil && flag.Changed
}

func compactStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			out = append(out, value)
		}
	}
	return out
}
