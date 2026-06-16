package commands

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/fulsiram/platform-lab/tools/salami-cli/internal/appconfig"
	"github.com/fulsiram/platform-lab/tools/salami-cli/internal/auth"
	"github.com/fulsiram/platform-lab/tools/salami-cli/internal/devbox"
	"github.com/fulsiram/platform-lab/tools/salami-cli/internal/kube"
	"github.com/fulsiram/platform-lab/tools/salami-cli/internal/sshkeys"
	"github.com/spf13/cobra"
	"golang.org/x/term"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

type devboxListOptions struct {
	AllNamespaces bool
	AllOwners     bool
}

type devboxCreateOptions struct {
	Ports   []int
	NoWait  bool
	Timeout time.Duration
}

type devboxDeleteOptions struct {
	NoWait  bool
	Timeout time.Duration
}

type devboxPowerOptions struct {
	NoWait  bool
	Timeout time.Duration
}

func NewDevboxCmd(global *globalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "devbox",
		Aliases: []string{"devboxes"},
		Short:   "Manage Devbox resources",
	}

	listOpts := &devboxListOptions{}
	listCmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List Devboxes",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := global.config(cmd)
			if err != nil {
				return err
			}
			manager := auth.NewManager(cfg, cmd.ErrOrStderr())
			token, claims, err := manager.Token(cmd.Context())
			if err != nil {
				return err
			}
			client, err := kube.DynamicClient(cfg, token.IDToken)
			if err != nil {
				return err
			}
			devboxes, err := devbox.List(cmd.Context(), client, cfg.Namespace, listOpts.AllNamespaces)
			if err != nil {
				return err
			}
			if !listOpts.AllOwners {
				devboxes = devbox.FilterByOwner(devboxes, devbox.OwnerFromEmail(claims.Email))
			}
			return devbox.WriteTable(cmd.OutOrStdout(), devboxes, listOpts.AllNamespaces, listOpts.AllOwners)
		},
	}
	listCmd.Flags().BoolVarP(&listOpts.AllNamespaces, "all-namespaces", "A", false, "List Devboxes across all namespaces")
	listCmd.Flags().BoolVar(&listOpts.AllOwners, "all", false, "List Devboxes for all owners and include the owner column")

	getCmd := &cobra.Command{
		Use:     "get NAME",
		Aliases: []string{"show"},
		Short:   "Show a Devbox",
		Args:    devboxNameArg,
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
			client, err := kube.DynamicClient(cfg, token.IDToken)
			if err != nil {
				return err
			}
			got, err := devbox.Get(cmd.Context(), client, cfg.Namespace, args[0])
			if err != nil {
				return err
			}
			return devbox.WriteSummary(cmd.OutOrStdout(), got, nil)
		},
	}

	createOpts := &devboxCreateOptions{
		Ports:   []int{22},
		Timeout: 10 * time.Minute,
	}
	createCmd := &cobra.Command{
		Use:   "create NAME",
		Short: "Create a Devbox",
		Args:  devboxNameArg,
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			cfg, err := global.config(cmd)
			if err != nil {
				return err
			}
			ports, err := devbox.NormalizePorts(createOpts.Ports)
			if err != nil {
				return err
			}
			manager := auth.NewManager(cfg, cmd.ErrOrStderr())
			token, claims, err := manager.Token(cmd.Context())
			if err != nil {
				return err
			}
			keysName, err := sshkeys.DefaultConfigMapName(claims.Email)
			if err != nil {
				return err
			}
			kubeClient, err := kube.KubernetesClient(cfg, token.IDToken)
			if err != nil {
				return err
			}
			if err := ensureDevboxSSHKeys(cmd.Context(), kubeClient, cfg.Namespace, keysName); err != nil {
				return err
			}
			dynamicClient, err := kube.DynamicClient(cfg, token.IDToken)
			if err != nil {
				return err
			}
			startedAt := time.Now()
			created, err := devbox.Create(cmd.Context(), dynamicClient, devbox.CreateOptions{
				Namespace:               cfg.Namespace,
				Name:                    name,
				AuthorizedKeysConfigMap: keysName,
				ExposedPorts:            ports,
			})
			if err != nil {
				return err
			}
			if createOpts.NoWait {
				fmt.Fprintf(cmd.OutOrStdout(), "Created devbox %s/%s\n", created.Namespace, created.Name)
				return devbox.WriteSummary(cmd.OutOrStdout(), created, ports)
			}

			printer := newDevboxStatusPrinter(cmd.ErrOrStderr())
			waited, err := devbox.WaitForRunning(cmd.Context(), dynamicClient, cfg.Namespace, name, devbox.WaitOptions{
				Timeout:      createOpts.Timeout,
				PollInterval: time.Second,
				OnUpdate:     printer.Update,
			})
			printer.Done()
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), formatDevboxStarted(waited, time.Since(startedAt)))
			return devbox.WriteSummary(cmd.OutOrStdout(), waited, ports)
		},
	}
	createCmd.Flags().IntSliceVar(&createOpts.Ports, "port", createOpts.Ports, "Expose a Devbox port; may be repeated")
	createCmd.Flags().BoolVar(&createOpts.NoWait, "no-wait", false, "Return immediately after creating the Devbox")
	createCmd.Flags().DurationVar(&createOpts.Timeout, "timeout", createOpts.Timeout, "Maximum time to wait for the Devbox VM to become Running")

	deleteOpts := &devboxDeleteOptions{
		Timeout: 10 * time.Minute,
	}
	deleteCmd := &cobra.Command{
		Use:     "delete NAME",
		Aliases: []string{"rm"},
		Short:   "Delete a Devbox",
		Args:    devboxNameArg,
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			cfg, err := global.config(cmd)
			if err != nil {
				return err
			}
			manager := auth.NewManager(cfg, cmd.ErrOrStderr())
			token, _, err := manager.Token(cmd.Context())
			if err != nil {
				return err
			}
			client, err := kube.DynamicClient(cfg, token.IDToken)
			if err != nil {
				return err
			}
			startedAt := time.Now()
			if err := devbox.Delete(cmd.Context(), client, cfg.Namespace, name); err != nil {
				return err
			}
			if deleteOpts.NoWait {
				fmt.Fprintf(cmd.OutOrStdout(), "Delete requested: %s/%s\n", cfg.Namespace, name)
				return nil
			}

			printer := newDevboxDeleteStatusPrinter(cmd.ErrOrStderr())
			err = devbox.WaitForDeleted(cmd.Context(), client, cfg.Namespace, name, devbox.WaitOptions{
				Timeout:      deleteOpts.Timeout,
				PollInterval: time.Second,
				OnUpdate:     printer.Update,
			})
			printer.Done()
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), formatDevboxDeleted(cfg.Namespace, name, time.Since(startedAt)))
			return nil
		},
	}
	deleteCmd.Flags().BoolVar(&deleteOpts.NoWait, "no-wait", false, "Return immediately after requesting Devbox deletion")
	deleteCmd.Flags().DurationVar(&deleteOpts.Timeout, "timeout", deleteOpts.Timeout, "Maximum time to wait for the Devbox to be deleted")

	startOpts := &devboxPowerOptions{
		Timeout: 10 * time.Minute,
	}
	startCmd := &cobra.Command{
		Use:   "start NAME",
		Short: "Start a Devbox",
		Args:  devboxNameArg,
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			cfg, client, err := devboxDynamicClient(cmd, global)
			if err != nil {
				return err
			}
			startedAt := time.Now()
			updated, err := devbox.SetPowerState(cmd.Context(), client, cfg.Namespace, name, "Running")
			if err != nil {
				return err
			}
			if startOpts.NoWait {
				fmt.Fprintf(cmd.OutOrStdout(), "Start requested: %s/%s\n", updated.Namespace, updated.Name)
				return nil
			}

			printer := newDevboxStatusPrinter(cmd.ErrOrStderr())
			waited, err := devbox.WaitForVMStatus(cmd.Context(), client, cfg.Namespace, name, "Running", devbox.WaitOptions{
				Timeout:      startOpts.Timeout,
				PollInterval: time.Second,
				OnUpdate:     printer.Update,
			})
			printer.Done()
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), formatDevboxStarted(waited, time.Since(startedAt)))
			return nil
		},
	}
	startCmd.Flags().BoolVar(&startOpts.NoWait, "no-wait", false, "Return immediately after requesting Devbox start")
	startCmd.Flags().DurationVar(&startOpts.Timeout, "timeout", startOpts.Timeout, "Maximum time to wait for the Devbox VM to become Running")

	stopOpts := &devboxPowerOptions{
		Timeout: 10 * time.Minute,
	}
	stopCmd := &cobra.Command{
		Use:   "stop NAME",
		Short: "Stop a Devbox",
		Args:  devboxNameArg,
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			cfg, client, err := devboxDynamicClient(cmd, global)
			if err != nil {
				return err
			}
			startedAt := time.Now()
			updated, err := devbox.SetPowerState(cmd.Context(), client, cfg.Namespace, name, "Stopped")
			if err != nil {
				return err
			}
			if stopOpts.NoWait {
				fmt.Fprintf(cmd.OutOrStdout(), "Stop requested: %s/%s\n", updated.Namespace, updated.Name)
				return nil
			}

			printer := newDevboxStopStatusPrinter(cmd.ErrOrStderr())
			waited, err := devbox.WaitForVMStatus(cmd.Context(), client, cfg.Namespace, name, "Stopped", devbox.WaitOptions{
				Timeout:      stopOpts.Timeout,
				PollInterval: time.Second,
				OnUpdate:     printer.Update,
			})
			printer.Done()
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), formatDevboxStopped(waited, time.Since(startedAt)))
			return nil
		},
	}
	stopCmd.Flags().BoolVar(&stopOpts.NoWait, "no-wait", false, "Return immediately after requesting Devbox stop")
	stopCmd.Flags().DurationVar(&stopOpts.Timeout, "timeout", stopOpts.Timeout, "Maximum time to wait for the Devbox VM to become Stopped")

	restartOpts := &devboxPowerOptions{
		Timeout: 10 * time.Minute,
	}
	restartCmd := &cobra.Command{
		Use:   "restart NAME",
		Short: "Restart a Devbox",
		Args:  devboxNameArg,
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			cfg, client, err := devboxDynamicClient(cmd, global)
			if err != nil {
				return err
			}
			startedAt := time.Now()
			if _, err := devbox.SetPowerState(cmd.Context(), client, cfg.Namespace, name, "Stopped"); err != nil {
				return err
			}

			stopPrinter := newDevboxStopStatusPrinter(cmd.ErrOrStderr())
			_, err = devbox.WaitForVMStatus(cmd.Context(), client, cfg.Namespace, name, "Stopped", devbox.WaitOptions{
				Timeout:      restartOpts.Timeout,
				PollInterval: time.Second,
				OnUpdate:     stopPrinter.Update,
			})
			stopPrinter.Done()
			if err != nil {
				return err
			}

			if _, err := devbox.SetPowerState(cmd.Context(), client, cfg.Namespace, name, "Running"); err != nil {
				return err
			}
			startPrinter := newDevboxStatusPrinter(cmd.ErrOrStderr())
			waited, err := devbox.WaitForVMStatus(cmd.Context(), client, cfg.Namespace, name, "Running", devbox.WaitOptions{
				Timeout:      restartOpts.Timeout,
				PollInterval: time.Second,
				OnUpdate:     startPrinter.Update,
			})
			startPrinter.Done()
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), formatDevboxRestarted(waited, time.Since(startedAt)))
			return nil
		},
	}
	restartCmd.Flags().DurationVar(&restartOpts.Timeout, "timeout", restartOpts.Timeout, "Maximum time to wait for each restart phase")

	cmd.AddCommand(listCmd, getCmd, createCmd, deleteCmd, startCmd, stopCmd, restartCmd)
	return cmd
}

const devboxSSHKeySetupHint = "run salami keys add --file ~/.ssh/id_ed25519.pub"

func devboxNameArg(cmd *cobra.Command, args []string) error {
	if err := cobra.ExactArgs(1)(cmd, args); err != nil {
		return err
	}
	if errs := validation.IsDNS1123Subdomain(args[0]); len(errs) > 0 {
		return fmt.Errorf("invalid devbox name %q: %s", args[0], errs[0])
	}
	return nil
}

func ensureDevboxSSHKeys(ctx context.Context, client kubernetes.Interface, namespace string, name string) error {
	contents, found, err := sshkeys.GetContents(ctx, client, namespace, name)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("SSH keys ConfigMap %s/%s does not exist; %s", namespace, name, devboxSSHKeySetupHint)
	}
	keys, err := sshkeys.ParseAuthorizedKeys(contents)
	if err != nil {
		return fmt.Errorf("SSH keys ConfigMap %s/%s contains invalid authorized_keys data: %w; %s", namespace, name, err, devboxSSHKeySetupHint)
	}
	if len(keys) == 0 {
		return fmt.Errorf("SSH keys ConfigMap %s/%s does not contain any SSH keys; %s", namespace, name, devboxSSHKeySetupHint)
	}
	return nil
}

func devboxDynamicClient(cmd *cobra.Command, global *globalOptions) (appconfig.Config, dynamic.Interface, error) {
	cfg, err := global.config(cmd)
	if err != nil {
		return appconfig.Config{}, nil, err
	}
	manager := auth.NewManager(cfg, cmd.ErrOrStderr())
	token, _, err := manager.Token(cmd.Context())
	if err != nil {
		return appconfig.Config{}, nil, err
	}
	client, err := kube.DynamicClient(cfg, token.IDToken)
	if err != nil {
		return appconfig.Config{}, nil, err
	}
	return cfg, client, nil
}

type devboxStatusPrinter struct {
	w              io.Writer
	terminal       *os.File
	interactive    bool
	action         string
	status         func(devbox.Devbox) string
	suppressStatus string
	lastLine       string
	lastState      string
	tick           int
	wrote          bool
}

func newDevboxStatusPrinter(w io.Writer) *devboxStatusPrinter {
	return newDevboxProgressPrinter(w, "Starting", func(current devbox.Devbox) string {
		return statusOrPending(current.VMPrintableStatus)
	}, "Running")
}

func newDevboxStopStatusPrinter(w io.Writer) *devboxStatusPrinter {
	return newDevboxProgressPrinter(w, "Stopping", func(current devbox.Devbox) string {
		return statusOrPending(current.VMPrintableStatus)
	}, "Stopped")
}

func newDevboxDeleteStatusPrinter(w io.Writer) *devboxStatusPrinter {
	return newDevboxProgressPrinter(w, "Deleting", func(devbox.Devbox) string {
		return "Terminating"
	}, "")
}

func newDevboxProgressPrinter(w io.Writer, action string, status func(devbox.Devbox) string, suppressStatus string) *devboxStatusPrinter {
	terminal := terminalFile(w)
	return &devboxStatusPrinter{
		w:              w,
		terminal:       terminal,
		interactive:    terminal != nil,
		action:         action,
		status:         status,
		suppressStatus: suppressStatus,
	}
}

func (p *devboxStatusPrinter) Update(current devbox.Devbox, elapsed time.Duration) {
	if p.w == nil {
		return
	}
	p.tick++
	prefix := fmt.Sprintf(
		"=> %s devbox %s/%s: %s%s",
		p.action,
		dash(current.Namespace),
		dash(current.Name),
		p.status(current),
		progressDots(p.tick, p.interactive),
	)
	line := formatProgressLine(prefix, roundedElapsed(elapsed), p.terminal)
	if !p.interactive {
		state := p.status(current)
		if state == p.lastState || state == p.suppressStatus {
			p.lastState = state
			return
		}
		p.lastState = state
		fmt.Fprintln(p.w, line)
		return
	}
	padding := ""
	if len(p.lastLine) > len(line) {
		padding = strings.Repeat(" ", len(p.lastLine)-len(line))
	}
	fmt.Fprintf(p.w, "\r%s%s", line, padding)
	p.lastLine = line
	p.wrote = true
}

func (p *devboxStatusPrinter) Done() {
	if p.interactive && p.wrote {
		fmt.Fprintf(p.w, "\r%s\r", strings.Repeat(" ", len(p.lastLine)))
		p.lastLine = ""
		p.wrote = false
	}
}

func terminalFile(w io.Writer) *os.File {
	file, ok := w.(*os.File)
	if !ok {
		return nil
	}
	info, err := file.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice == 0 {
		return nil
	}
	return file
}

func formatProgressLine(prefix string, elapsed string, terminal *os.File) string {
	if terminal == nil {
		return prefix + "  " + elapsed
	}
	width, _, err := term.GetSize(int(terminal.Fd()))
	if err != nil || width <= 0 {
		return prefix + "  " + elapsed
	}
	// Keep one column free to avoid terminals wrapping when the line exactly
	// fills the available width.
	if width > 1 {
		width--
	}
	if len(prefix)+1+len(elapsed) >= width {
		return prefix + "  " + elapsed
	}
	return prefix + strings.Repeat(" ", width-len(prefix)-len(elapsed)) + elapsed
}

func progressDots(tick int, animated bool) string {
	if !animated {
		return "..."
	}
	count := ((tick - 1) % 3) + 1
	return strings.Repeat(".", count)
}

func statusOrPending(value string) string {
	if value == "" {
		return "Pending"
	}
	return value
}

func formatDevboxStarted(db devbox.Devbox, elapsed time.Duration) string {
	return fmt.Sprintf("=> Devbox %s/%s started in %s", dash(db.Namespace), dash(db.Name), roundedElapsed(elapsed))
}

func formatDevboxStopped(db devbox.Devbox, elapsed time.Duration) string {
	return fmt.Sprintf("=> Devbox %s/%s stopped in %s", dash(db.Namespace), dash(db.Name), roundedElapsed(elapsed))
}

func formatDevboxRestarted(db devbox.Devbox, elapsed time.Duration) string {
	return fmt.Sprintf("=> Devbox %s/%s restarted in %s", dash(db.Namespace), dash(db.Name), roundedElapsed(elapsed))
}

func formatDevboxDeleted(namespace string, name string, elapsed time.Duration) string {
	return fmt.Sprintf("=> Devbox %s/%s deleted in %s", dash(namespace), dash(name), roundedElapsed(elapsed))
}

func roundedElapsed(elapsed time.Duration) string {
	if elapsed < time.Second {
		return elapsed.Truncate(time.Millisecond).String()
	}
	return elapsed.Round(time.Second).String()
}

func dash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}
