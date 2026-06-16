package commands

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"

	"github.com/fulsiram/platform-lab/tools/salami-cli/internal/appconfig"
	"github.com/fulsiram/platform-lab/tools/salami-cli/internal/auth"
	"github.com/fulsiram/platform-lab/tools/salami-cli/internal/devbox"
	"github.com/fulsiram/platform-lab/tools/salami-cli/internal/kube"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const defaultDevboxSSHUser = "user"
const defaultDevboxSSHPort = 22

type devboxSSHOptions struct {
	User     string
	Port     int
	Identity string
}

type devboxSSHConfigOptions struct {
	User     string
	Port     int
	Identity string
	Host     string
}

type devboxForwardOptions struct {
	ListenAddress string
	LocalPort     int
	Port          int
}

type devboxSSHClient struct {
	Config     appconfig.Config
	Claims     auth.Claims
	RESTConfig *rest.Config
	Dynamic    dynamic.Interface
	Kubernetes kubernetes.Interface
	Program    string
	GlobalArgs []string
}

var runOpenSSH = runOpenSSHCommand

func NewDevboxSSHCmd(global *globalOptions) *cobra.Command {
	opts := &devboxSSHOptions{
		User: defaultDevboxSSHUser,
		Port: defaultDevboxSSHPort,
	}
	cmd := &cobra.Command{
		Use:   "ssh NAME [-- SSH_ARGS...]",
		Short: "SSH into a Devbox",
		Args:  devboxSSHArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			extraArgs := args[1:]
			clients, err := devboxSSHClients(cmd, global)
			if err != nil {
				return err
			}
			if _, err := devbox.ResolveAccess(cmd.Context(), clients.Dynamic, clients.Kubernetes, clients.Config.Namespace, name, clients.Claims.Email, opts.Port); err != nil {
				return err
			}
			proxyCommand := buildDevboxProxyCommand(clients.Program, clients.GlobalArgs, name, opts.Port)
			sshArgs := buildOpenSSHArgs(devboxSSHConfigOptions{
				User:     opts.User,
				Port:     opts.Port,
				Identity: opts.Identity,
				Host:     defaultSSHHostAlias(clients.Config.Namespace, name),
			}, clients.Config.Namespace, name, proxyCommand, extraArgs)
			return runOpenSSH(cmd.Context(), sshArgs, cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	cmd.Flags().StringVar(&opts.User, "user", opts.User, "SSH username")
	cmd.Flags().IntVar(&opts.Port, "port", opts.Port, "Remote SSH port")
	cmd.Flags().StringVarP(&opts.Identity, "identity", "i", "", "Path to an SSH identity file")
	return cmd
}

func NewDevboxSSHConfigCmd(global *globalOptions) *cobra.Command {
	opts := &devboxSSHConfigOptions{
		User: defaultDevboxSSHUser,
		Port: defaultDevboxSSHPort,
	}
	cmd := &cobra.Command{
		Use:   "ssh-config NAME",
		Short: "Print SSH config for a Devbox",
		Args:  devboxNameArg,
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			cfg, err := global.config(cmd)
			if err != nil {
				return err
			}
			if opts.Host == "" {
				opts.Host = defaultSSHHostAlias(cfg.Namespace, name)
			}
			proxyCommand := buildDevboxProxyCommand("salami", devboxProxyGlobalArgs(cmd, global, cfg.Namespace), name, opts.Port)
			return writeDevboxSSHConfig(cmd.OutOrStdout(), *opts, cfg.Namespace, name, proxyCommand)
		},
	}
	cmd.Flags().StringVar(&opts.User, "user", opts.User, "SSH username")
	cmd.Flags().IntVar(&opts.Port, "port", opts.Port, "Remote SSH port")
	cmd.Flags().StringVarP(&opts.Identity, "identity", "i", "", "Path to an SSH identity file")
	cmd.Flags().StringVar(&opts.Host, "host", "", "SSH host alias to emit")
	return cmd
}

func NewDevboxSSHProxyCmd(global *globalOptions) *cobra.Command {
	opts := &devboxForwardOptions{
		Port: defaultDevboxSSHPort,
	}
	cmd := &cobra.Command{
		Use:    "ssh-proxy NAME",
		Short:  "Proxy SSH to a Devbox",
		Hidden: true,
		Args:   devboxNameArg,
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
			defer stop()

			clients, err := devboxSSHClients(cmd, global)
			if err != nil {
				return err
			}
			target, err := devbox.ResolveAccess(ctx, clients.Dynamic, clients.Kubernetes, clients.Config.Namespace, name, clients.Claims.Email, opts.Port)
			if err != nil {
				return err
			}
			err = kube.ProxyPortForward(ctx, clients.RESTConfig, target.Namespace, target.PodName, opts.Port, cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr())
			if errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		},
	}
	cmd.Flags().IntVar(&opts.Port, "port", opts.Port, "Remote SSH port")
	return cmd
}

func NewDevboxForwardCmd(global *globalOptions) *cobra.Command {
	opts := &devboxForwardOptions{
		ListenAddress: "127.0.0.1",
		Port:          defaultDevboxSSHPort,
	}
	cmd := &cobra.Command{
		Use:   "forward NAME",
		Short: "Forward a local port to a Devbox",
		Args:  devboxNameArg,
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
			defer stop()

			clients, err := devboxSSHClients(cmd, global)
			if err != nil {
				return err
			}
			target, err := devbox.ResolveAccess(ctx, clients.Dynamic, clients.Kubernetes, clients.Config.Namespace, name, clients.Claims.Email, opts.Port)
			if err != nil {
				return err
			}
			session, err := kube.StartPortForward(ctx, clients.RESTConfig, target.Namespace, target.PodName, opts.ListenAddress, opts.LocalPort, opts.Port, io.Discard, cmd.ErrOrStderr())
			if err != nil {
				return err
			}
			defer session.Close()

			local := net.JoinHostPort(session.LocalAddress, strconv.Itoa(session.LocalPort))
			fmt.Fprintf(cmd.OutOrStdout(), "=> Forwarding devbox %s/%s: %s -> %d\n", target.Namespace, target.Name, local, opts.Port)
			err = session.Wait(ctx)
			if errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		},
	}
	cmd.Flags().StringVar(&opts.ListenAddress, "listen-address", opts.ListenAddress, "Local address to listen on")
	cmd.Flags().IntVar(&opts.LocalPort, "local-port", 0, "Local port to listen on; 0 chooses an available port")
	cmd.Flags().IntVar(&opts.Port, "port", opts.Port, "Remote Devbox port")
	return cmd
}

func devboxSSHClients(cmd *cobra.Command, global *globalOptions) (devboxSSHClient, error) {
	cfg, err := global.config(cmd)
	if err != nil {
		return devboxSSHClient{}, err
	}
	manager := auth.NewManager(cfg, cmd.ErrOrStderr())
	token, claims, err := manager.Token(cmd.Context())
	if err != nil {
		return devboxSSHClient{}, err
	}
	restConfig, err := kube.RESTConfig(cfg, token.IDToken)
	if err != nil {
		return devboxSSHClient{}, err
	}
	dynamicClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return devboxSSHClient{}, fmt.Errorf("create Kubernetes dynamic client: %w", err)
	}
	kubeClient, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return devboxSSHClient{}, fmt.Errorf("create Kubernetes client: %w", err)
	}
	program, err := os.Executable()
	if err != nil || program == "" {
		program = os.Args[0]
	}
	return devboxSSHClient{
		Config:     cfg,
		Claims:     claims,
		RESTConfig: restConfig,
		Dynamic:    dynamicClient,
		Kubernetes: kubeClient,
		Program:    program,
		GlobalArgs: devboxProxyGlobalArgs(cmd, global, cfg.Namespace),
	}, nil
}

func devboxSSHArgs(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("requires at least 1 arg(s), only received 0")
	}
	if errs := validationErrorsForDevboxName(args[0]); len(errs) > 0 {
		return fmt.Errorf("invalid devbox name %q: %s", args[0], errs[0])
	}
	return nil
}

func validationErrorsForDevboxName(name string) []string {
	return validation.IsDNS1123Subdomain(name)
}

func buildOpenSSHArgs(opts devboxSSHConfigOptions, namespace string, name string, proxyCommand string, extraArgs []string) []string {
	args := []string{
		"-o", "ProxyCommand=" + proxyCommand,
		"-o", "HostKeyAlias=" + hostKeyAlias(namespace, name),
		"-o", "CheckHostIP=no",
	}
	if opts.Identity != "" {
		args = append(args, "-i", opts.Identity)
	}
	args = append(args, extraArgs...)
	args = append(args, opts.User+"@"+opts.Host)
	return args
}

func writeDevboxSSHConfig(w io.Writer, opts devboxSSHConfigOptions, namespace string, name string, proxyCommand string) error {
	if opts.Host == "" {
		opts.Host = defaultSSHHostAlias(namespace, name)
	}
	lines := []string{
		"Host " + opts.Host,
		"  HostName " + opts.Host,
		"  User " + opts.User,
		"  ProxyCommand " + proxyCommand,
		"  HostKeyAlias " + hostKeyAlias(namespace, name),
		"  CheckHostIP no",
	}
	if opts.Identity != "" {
		lines = append(lines, "  IdentityFile "+opts.Identity)
	}
	_, err := fmt.Fprintln(w, strings.Join(lines, "\n"))
	return err
}

func buildDevboxProxyCommand(program string, globalArgs []string, name string, port int) string {
	args := []string{program}
	args = append(args, globalArgs...)
	args = append(args, "devbox", "ssh-proxy", name, "--port", strconv.Itoa(port))
	return shellJoin(args)
}

func devboxProxyGlobalArgs(cmd *cobra.Command, global *globalOptions, namespace string) []string {
	args := make([]string, 0, 16)
	if global.ConfigPath != "" {
		args = append(args, "--config", global.ConfigPath)
	}
	if flagChanged(cmd, "api-server") {
		args = append(args, "--api-server", global.APIServer)
	}
	if flagChanged(cmd, "issuer-url") {
		args = append(args, "--issuer-url", global.IssuerURL)
	}
	if flagChanged(cmd, "oidc-client-id") {
		args = append(args, "--oidc-client-id", global.OIDCClientID)
	}
	if flagChanged(cmd, "certificate-authority") {
		args = append(args, "--certificate-authority", global.CertificateAuthority)
	}
	if flagChanged(cmd, "certificate-authority-data") {
		args = append(args, "--certificate-authority-data", global.CertificateAuthorityData)
	}
	for _, scope := range compactStrings(global.OIDCScopes) {
		if flagChanged(cmd, "oidc-scope") {
			args = append(args, "--oidc-scope", scope)
		}
	}
	args = append(args, "--namespace", namespace)
	return args
}

func defaultSSHHostAlias(namespace string, name string) string {
	return name + "." + namespace + ".devbox"
}

func hostKeyAlias(namespace string, name string) string {
	return "salami-devbox/" + namespace + "/" + name
}

func shellJoin(args []string) string {
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		quoted = append(quoted, shellQuote(arg))
	}
	return strings.Join(quoted, " ")
}

func shellQuote(arg string) string {
	if arg == "" {
		return "''"
	}
	if isShellSafe(arg) {
		return arg
	}
	return "'" + strings.ReplaceAll(arg, "'", `'\''`) + "'"
}

func isShellSafe(arg string) bool {
	for _, r := range arg {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case strings.ContainsRune("_@%+=:,./-", r):
		default:
			return false
		}
	}
	return true
}

func runOpenSSHCommand(ctx context.Context, args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
	sshPath, err := exec.LookPath("ssh")
	if err != nil {
		return errors.New("ssh executable not found in PATH")
	}
	command := exec.CommandContext(ctx, sshPath, args...)
	command.Stdin = stdin
	command.Stdout = stdout
	command.Stderr = stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("run ssh: %w", err)
	}
	return nil
}
