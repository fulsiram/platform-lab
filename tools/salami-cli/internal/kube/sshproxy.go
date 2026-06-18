package kube

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/httpstream"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

const (
	defaultSSHReadyTimeout         = 90 * time.Second
	defaultSSHRetryDelay           = 100 * time.Millisecond
	defaultSSHBannerSilenceTimeout = 20 * time.Second
	defaultSSHBannerMaxBytes       = 8 * 1024

	sshProxyDebugEnv = "SALAMI_SSH_PROXY_DEBUG"
)

var (
	errSSHBannerEOF     = errors.New("EOF before SSH identification")
	errSSHBannerSilence = errors.New("timed out waiting for SSH identification")
	errSSHBannerTooLong = errors.New("SSH identification buffer exceeded maximum size")
)

type sshProxyConfig struct {
	Dialer               httpstream.Dialer
	RemotePort           int
	Stdin                io.Reader
	Stdout               io.Writer
	ReadyTimeout         time.Duration
	RetryDelay           time.Duration
	BannerSilenceTimeout time.Duration
	BannerMaxBytes       int
	Sleep                func(context.Context, time.Duration) error
	Debug                io.Writer
}

type sshPortForwardAttempt struct {
	connection  httpstream.Connection
	dataStream  httpstream.Stream
	errorStream httpstream.Stream
	errorCh     <-chan error
	remotePort  int
	requestID   int
}

type terminalAttemptError struct {
	err error
}

type sshProxyDebug struct {
	out io.Writer
	mu  sync.Mutex
}

func (e terminalAttemptError) Error() string {
	return e.err.Error()
}

func (e terminalAttemptError) Unwrap() error {
	return e.err
}

type remoteStreamError struct {
	err error
}

func (e remoteStreamError) Error() string {
	return e.err.Error()
}

func (e remoteStreamError) Unwrap() error {
	return e.err
}

// ProxySSHPortForward proxies stdin/stdout to a pod port-forward stream after
// the remote port emits a valid SSH identification line.
func ProxySSHPortForward(ctx context.Context, restConfig *rest.Config, namespace string, podName string, remotePort int, stdin io.Reader, stdout io.Writer, errOut io.Writer) error {
	if remotePort < 1 || remotePort > 65535 {
		return fmt.Errorf("remote port %d is outside the valid range 1-65535", remotePort)
	}
	if stdin == nil {
		stdin = strings.NewReader("")
	}
	if stdout == nil {
		stdout = io.Discard
	}
	if errOut == nil {
		errOut = io.Discard
	}

	debugOut := debugWriterFromEnv(errOut)
	debug := newSSHProxyDebug(debugOut)
	debug.printf("target namespace=%s pod=%s remote_port=%d api_server=%s", namespace, podName, remotePort, restConfig.Host)

	dialer, err := portForwardDialer(restConfig, namespace, podName, debug)
	if err != nil {
		return err
	}
	return proxySSHPortForward(ctx, sshProxyConfig{
		Dialer:     dialer,
		RemotePort: remotePort,
		Stdin:      stdin,
		Stdout:     stdout,
		Debug:      debugOut,
	})
}

func portForwardDialer(restConfig *rest.Config, namespace string, podName string, debug *sshProxyDebug) (httpstream.Dialer, error) {
	debug.printf("creating SPDY port-forward transport")
	transport, upgrader, err := spdy.RoundTripperFor(restConfig)
	if err != nil {
		return nil, fmt.Errorf("create port-forward transport: %w", err)
	}
	endpoint, err := portForwardURL(restConfig.Host, namespace, podName)
	if err != nil {
		return nil, err
	}
	endpoint = cloneURL(endpoint)
	debug.printf("port-forward endpoint=%s", endpoint.String())
	return spdy.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost, endpoint), nil
}

func cloneURL(source *url.URL) *url.URL {
	cloned := *source
	return &cloned
}

func proxySSHPortForward(ctx context.Context, cfg sshProxyConfig) error {
	cfg.setDefaults()
	debug := newSSHProxyDebug(cfg.Debug)
	if cfg.Dialer == nil {
		return errors.New("port-forward dialer is required")
	}
	if cfg.RemotePort < 1 || cfg.RemotePort > 65535 {
		return fmt.Errorf("remote port %d is outside the valid range 1-65535", cfg.RemotePort)
	}

	readyCtx, cancel := context.WithTimeout(ctx, cfg.ReadyTimeout)
	defer cancel()

	debug.printf("dialing port-forward protocol=%s ready_timeout=%s retry_delay=%s banner_silence_timeout=%s banner_max_bytes=%d", portforward.PortForwardProtocolV1Name, cfg.ReadyTimeout, cfg.RetryDelay, cfg.BannerSilenceTimeout, cfg.BannerMaxBytes)
	connection, protocol, err := cfg.Dialer.Dial(portforward.PortForwardProtocolV1Name)
	if err != nil {
		debug.printf("port-forward dial failed: %v", err)
		return fmt.Errorf("connect pod port-forward: %w", err)
	}
	defer connection.Close()
	debug.printf("port-forward connected negotiated_protocol=%s", protocol)
	if protocol != portforward.PortForwardProtocolV1Name {
		return fmt.Errorf("unable to negotiate port-forward protocol: client supports %q, server returned %q", portforward.PortForwardProtocolV1Name, protocol)
	}

	var requestID int
	var lastRemoteErr error
	for {
		if err := readyCtx.Err(); err != nil {
			return sshReadyTimeoutError(ctx, cfg.RemotePort, cfg.ReadyTimeout, lastRemoteErr)
		}

		debug.printf("attempt=%d opening remote port-forward streams", requestID)
		attempt, err := openSSHPortForwardAttempt(connection, cfg.RemotePort, requestID, debug)
		requestID++
		if err == nil {
			debug.printf("attempt=%d waiting for SSH identification", attempt.requestID)
			buffered, err := attempt.waitForSSHIdentification(readyCtx, cfg.BannerSilenceTimeout, cfg.BannerMaxBytes, debug)
			if err == nil {
				debug.printf("attempt=%d SSH ready; forwarding buffered_bytes=%d readiness_deadline_released=true", attempt.requestID, len(buffered))
				return attempt.proxy(ctx, cfg.Stdin, cfg.Stdout, buffered, debug)
			}
			debug.printf("attempt=%d failed before SSH ready: %v", attempt.requestID, err)
			attempt.reset()
			if isTerminalAttemptError(err) {
				debug.printf("attempt=%d terminal banner error; not retrying", attempt.requestID)
				return err
			}
			if remoteErr := asRemoteStreamError(err); remoteErr != nil {
				lastRemoteErr = remoteErr
			}
		}

		if err := readyCtx.Err(); err != nil {
			return sshReadyTimeoutError(ctx, cfg.RemotePort, cfg.ReadyTimeout, lastRemoteErr)
		}
		debug.printf("retrying SSH port-forward in %s", cfg.RetryDelay)
		if err := cfg.Sleep(readyCtx, cfg.RetryDelay); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return sshReadyTimeoutError(ctx, cfg.RemotePort, cfg.ReadyTimeout, lastRemoteErr)
		}
	}
}

func newSSHProxyDebug(out io.Writer) *sshProxyDebug {
	return &sshProxyDebug{out: out}
}

func (d *sshProxyDebug) printf(format string, args ...any) {
	if d == nil || d.out == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	fmt.Fprintf(d.out, "salami ssh-proxy: "+format+"\n", args...)
}

func debugWriterFromEnv(errOut io.Writer) io.Writer {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(sshProxyDebugEnv)))
	switch value {
	case "1", "true", "yes", "on":
		return errOut
	default:
		return nil
	}
}

func (cfg *sshProxyConfig) setDefaults() {
	if cfg.Stdin == nil {
		cfg.Stdin = strings.NewReader("")
	}
	if cfg.Stdout == nil {
		cfg.Stdout = io.Discard
	}
	if cfg.ReadyTimeout == 0 {
		cfg.ReadyTimeout = defaultSSHReadyTimeout
	}
	if cfg.RetryDelay == 0 {
		cfg.RetryDelay = defaultSSHRetryDelay
	}
	if cfg.BannerSilenceTimeout == 0 {
		cfg.BannerSilenceTimeout = defaultSSHBannerSilenceTimeout
	}
	if cfg.BannerMaxBytes == 0 {
		cfg.BannerMaxBytes = defaultSSHBannerMaxBytes
	}
	if cfg.Sleep == nil {
		cfg.Sleep = sleepContext
	}
}

func openSSHPortForwardAttempt(connection httpstream.Connection, remotePort int, requestID int, debug *sshProxyDebug) (*sshPortForwardAttempt, error) {
	headers := http.Header{}
	headers.Set(corev1.StreamType, corev1.StreamTypeError)
	headers.Set(corev1.PortHeader, strconv.Itoa(remotePort))
	headers.Set(corev1.PortForwardRequestIDHeader, strconv.Itoa(requestID))
	errorStream, err := connection.CreateStream(headers)
	if err != nil {
		debug.printf("attempt=%d create error stream failed: %v", requestID, err)
		return nil, fmt.Errorf("create port-forward error stream for remote port %d: %w", remotePort, err)
	}
	_ = errorStream.Close()
	debug.printf("attempt=%d error stream created", requestID)

	headers.Set(corev1.StreamType, corev1.StreamTypeData)
	dataStream, err := connection.CreateStream(headers)
	if err != nil {
		connection.RemoveStreams(errorStream)
		_ = errorStream.Reset()
		debug.printf("attempt=%d create data stream failed: %v", requestID, err)
		return nil, fmt.Errorf("create port-forward data stream for remote port %d: %w", remotePort, err)
	}
	debug.printf("attempt=%d data stream created", requestID)

	return &sshPortForwardAttempt{
		connection:  connection,
		dataStream:  dataStream,
		errorStream: errorStream,
		errorCh:     readPortForwardError(errorStream, remotePort, requestID, debug),
		remotePort:  remotePort,
		requestID:   requestID,
	}, nil
}

func (a *sshPortForwardAttempt) waitForSSHIdentification(ctx context.Context, silenceTimeout time.Duration, maxBytes int, debug *sshProxyDebug) ([]byte, error) {
	resultCh := make(chan sshBannerResult, 1)
	go func() {
		buffered, err := readSSHIdentificationWithDebug(ctx, a.dataStream, silenceTimeout, maxBytes, a.requestID, debug)
		resultCh <- sshBannerResult{buffered: buffered, err: err}
	}()

	errorCh := a.errorCh
	for {
		select {
		case result := <-resultCh:
			return result.buffered, result.err
		case err, ok := <-errorCh:
			if !ok {
				errorCh = nil
				continue
			}
			if err != nil {
				_ = a.dataStream.Reset()
				debug.printf("attempt=%d remote error stream reported before banner: %v", a.requestID, err)
				return nil, err
			}
			errorCh = nil
		case <-ctx.Done():
			_ = a.dataStream.Reset()
			return nil, ctx.Err()
		}
	}
}

func (a *sshPortForwardAttempt) proxy(ctx context.Context, stdin io.Reader, stdout io.Writer, buffered []byte, debug *sshProxyDebug) error {
	defer a.reset()

	if len(buffered) > 0 {
		if _, err := stdout.Write(buffered); err != nil {
			return fmt.Errorf("write SSH identification to stdout: %w", err)
		}
	}

	stdinDone := make(chan error, 1)
	stdoutDone := make(chan error, 1)
	go func() {
		_, err := io.Copy(a.dataStream, stdin)
		_ = a.dataStream.Close()
		debug.printf("attempt=%d stdin copy finished err=%v", a.requestID, err)
		stdinDone <- err
	}()
	go func() {
		_, err := io.Copy(stdout, a.dataStream)
		debug.printf("attempt=%d stdout copy finished err=%v", a.requestID, err)
		stdoutDone <- err
	}()

	errorCh := a.errorCh
	for stdinDone != nil || stdoutDone != nil {
		select {
		case err := <-stdinDone:
			stdinDone = nil
			if err != nil {
				_ = a.dataStream.Reset()
				return fmt.Errorf("copy stdin to SSH stream: %w", err)
			}
		case err := <-stdoutDone:
			stdoutDone = nil
			_ = a.dataStream.Reset()
			if remoteErr := waitForRemoteStreamError(ctx, errorCh); remoteErr != nil {
				return remoteErr
			}
			if err != nil {
				return fmt.Errorf("copy SSH stream to stdout: %w", err)
			}
			return nil
		case err, ok := <-errorCh:
			if !ok {
				errorCh = nil
				continue
			}
			if err != nil {
				_ = a.dataStream.Reset()
				debug.printf("attempt=%d remote error stream reported during proxy: %v", a.requestID, err)
				return err
			}
			errorCh = nil
		case <-ctx.Done():
			_ = a.dataStream.Reset()
			return ctx.Err()
		}
	}
	return nil
}

func (a *sshPortForwardAttempt) reset() {
	if a == nil {
		return
	}
	if a.dataStream != nil {
		_ = a.dataStream.Reset()
	}
	if a.errorStream != nil {
		_ = a.errorStream.Reset()
	}
	if a.connection != nil {
		a.connection.RemoveStreams(a.errorStream, a.dataStream)
	}
}

func readPortForwardError(stream io.Reader, remotePort int, requestID int, debug *sshProxyDebug) <-chan error {
	ch := make(chan error, 1)
	go func() {
		defer close(ch)
		message, err := io.ReadAll(stream)
		if err != nil {
			debug.printf("attempt=%d error stream read failed: %v", requestID, err)
			ch <- fmt.Errorf("read port-forward error stream for remote port %d: %w", remotePort, err)
			return
		}
		if text := strings.TrimSpace(string(message)); text != "" {
			debug.printf("attempt=%d error stream message=%q", requestID, truncateDebugString(text, 300))
			ch <- remoteStreamError{err: fmt.Errorf("remote port %d error: %s", remotePort, text)}
			return
		}
		debug.printf("attempt=%d error stream closed cleanly", requestID)
		ch <- nil
	}()
	return ch
}

type sshBannerResult struct {
	buffered []byte
	err      error
}

func readSSHIdentification(ctx context.Context, reader io.Reader, silenceTimeout time.Duration, maxBytes int) ([]byte, error) {
	return readSSHIdentificationWithDebug(ctx, reader, silenceTimeout, maxBytes, -1, nil)
}

func readSSHIdentificationWithDebug(ctx context.Context, reader io.Reader, silenceTimeout time.Duration, maxBytes int, requestID int, debug *sshProxyDebug) ([]byte, error) {
	var buffered []byte
	var line []byte
	for {
		b, err := readByteWithTimeout(ctx, reader, silenceTimeout)
		if err != nil {
			if errors.Is(err, io.EOF) {
				debug.printf("attempt=%d SSH identification read hit EOF before banner buffered_bytes=%d", requestID, len(buffered))
				return nil, errSSHBannerEOF
			}
			debug.printf("attempt=%d SSH identification read failed before banner buffered_bytes=%d err=%v", requestID, len(buffered), err)
			return nil, err
		}

		buffered = append(buffered, b)
		line = append(line, b)
		if len(buffered) > maxBytes {
			debug.printf("attempt=%d SSH identification exceeded buffer cap buffered_bytes=%d max_bytes=%d", requestID, len(buffered), maxBytes)
			return nil, terminalAttemptError{err: fmt.Errorf("%w: %d bytes", errSSHBannerTooLong, maxBytes)}
		}
		if b != '\n' {
			continue
		}

		trimmed := strings.TrimRight(string(line), "\r\n")
		switch {
		case strings.HasPrefix(trimmed, "SSH-2.0-"), strings.HasPrefix(trimmed, "SSH-1.99-"):
			debug.printf("attempt=%d accepted SSH identification line=%q buffered_bytes=%d", requestID, truncateDebugString(trimmed, 200), len(buffered))
			return buffered, nil
		case strings.HasPrefix(trimmed, "SSH-"):
			debug.printf("attempt=%d unsupported SSH identification line=%q", requestID, truncateDebugString(trimmed, 200))
			return nil, terminalAttemptError{err: fmt.Errorf("unsupported SSH protocol identification %q", trimmed)}
		default:
			debug.printf("attempt=%d pre-banner line bytes=%d text=%q", requestID, len(line), truncateDebugString(trimmed, 200))
			line = line[:0]
		}
	}
}

func truncateDebugString(value string, max int) string {
	if max <= 0 || len(value) <= max {
		return value
	}
	return value[:max] + "...(truncated)"
}

func readByteWithTimeout(ctx context.Context, reader io.Reader, silenceTimeout time.Duration) (byte, error) {
	resultCh := make(chan struct {
		value byte
		err   error
	}, 1)
	go func() {
		var one [1]byte
		for {
			n, err := reader.Read(one[:])
			if n > 0 {
				resultCh <- struct {
					value byte
					err   error
				}{value: one[0]}
				return
			}
			if err != nil {
				resultCh <- struct {
					value byte
					err   error
				}{err: err}
				return
			}
		}
	}()

	timer := time.NewTimer(silenceTimeout)
	defer timer.Stop()

	select {
	case result := <-resultCh:
		return result.value, result.err
	case <-timer.C:
		return 0, errSSHBannerSilence
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

func waitForRemoteStreamError(ctx context.Context, errorCh <-chan error) error {
	if errorCh == nil {
		return nil
	}
	select {
	case err, ok := <-errorCh:
		if !ok {
			return nil
		}
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func isTerminalAttemptError(err error) bool {
	var terminal terminalAttemptError
	return errors.As(err, &terminal)
}

func asRemoteStreamError(err error) error {
	var remote remoteStreamError
	if errors.As(err, &remote) {
		return remote.err
	}
	return nil
}

func sshReadyTimeoutError(ctx context.Context, remotePort int, timeout time.Duration, lastRemoteErr error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if lastRemoteErr != nil {
		return fmt.Errorf("timed out waiting for SSH on remote port %d after %s: last remote error: %w", remotePort, timeout, lastRemoteErr)
	}
	return fmt.Errorf("timed out waiting for SSH identification on remote port %d after %s", remotePort, timeout)
}

func sleepContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
