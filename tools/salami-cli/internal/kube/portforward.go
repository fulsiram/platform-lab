package kube

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"sync"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

type PortForwardSession struct {
	LocalAddress string
	LocalPort    int

	stopCh chan struct{}
	doneCh chan error
	once   sync.Once
}

func StartPortForward(ctx context.Context, restConfig *rest.Config, namespace string, podName string, localAddress string, localPort int, remotePort int, out io.Writer, errOut io.Writer) (*PortForwardSession, error) {
	if localAddress == "" {
		localAddress = "127.0.0.1"
	}
	if localPort < 0 || localPort > 65535 {
		return nil, fmt.Errorf("local port %d is outside the valid range 0-65535", localPort)
	}
	if remotePort < 1 || remotePort > 65535 {
		return nil, fmt.Errorf("remote port %d is outside the valid range 1-65535", remotePort)
	}
	if out == nil {
		out = io.Discard
	}
	if errOut == nil {
		errOut = io.Discard
	}

	transport, upgrader, err := spdy.RoundTripperFor(restConfig)
	if err != nil {
		return nil, fmt.Errorf("create port-forward transport: %w", err)
	}
	endpoint, err := portForwardURL(restConfig.Host, namespace, podName)
	if err != nil {
		return nil, err
	}
	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost, endpoint)

	stopCh := make(chan struct{})
	readyCh := make(chan struct{})
	spec := fmt.Sprintf("%d:%d", localPort, remotePort)
	forwarder, err := portforward.NewOnAddresses(dialer, []string{localAddress}, []string{spec}, stopCh, readyCh, out, errOut)
	if err != nil {
		return nil, fmt.Errorf("create port-forwarder: %w", err)
	}

	session := &PortForwardSession{
		LocalAddress: localAddress,
		stopCh:       stopCh,
		doneCh:       make(chan error, 1),
	}
	go func() {
		session.doneCh <- forwarder.ForwardPorts()
	}()
	go func() {
		<-ctx.Done()
		session.Close()
	}()

	select {
	case <-readyCh:
		ports, err := forwarder.GetPorts()
		if err != nil {
			session.Close()
			return nil, fmt.Errorf("read forwarded port: %w", err)
		}
		if len(ports) == 0 {
			session.Close()
			return nil, errors.New("port-forwarder did not report a local port")
		}
		session.LocalPort = int(ports[0].Local)
		return session, nil
	case err := <-session.doneCh:
		if err == nil {
			err = errors.New("port-forwarder exited before it was ready")
		}
		return nil, fmt.Errorf("start port-forward: %w", err)
	case <-ctx.Done():
		session.Close()
		return nil, ctx.Err()
	}
}

func (s *PortForwardSession) Close() {
	if s == nil {
		return
	}
	s.once.Do(func() {
		close(s.stopCh)
	})
}

func (s *PortForwardSession) Wait(ctx context.Context) error {
	if s == nil {
		return nil
	}
	select {
	case err := <-s.doneCh:
		return err
	case <-ctx.Done():
		s.Close()
		return ctx.Err()
	}
}

func ProxyPortForward(ctx context.Context, restConfig *rest.Config, namespace string, podName string, remotePort int, stdin io.Reader, stdout io.Writer, errOut io.Writer) error {
	session, err := StartPortForward(ctx, restConfig, namespace, podName, "127.0.0.1", 0, remotePort, io.Discard, errOut)
	if err != nil {
		return err
	}
	defer session.Close()

	address := net.JoinHostPort(session.LocalAddress, strconv.Itoa(session.LocalPort))
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", address)
	if err != nil {
		return fmt.Errorf("connect to local port-forward %s: %w", address, err)
	}
	defer conn.Close()

	stdinDone := make(chan error, 1)
	stdoutDone := make(chan error, 1)
	go func() {
		_, err := io.Copy(conn, stdin)
		if tcp, ok := conn.(*net.TCPConn); ok {
			_ = tcp.CloseWrite()
		}
		stdinDone <- err
	}()
	go func() {
		_, err := io.Copy(stdout, conn)
		stdoutDone <- err
	}()

	for stdinDone != nil || stdoutDone != nil {
		select {
		case err := <-stdinDone:
			stdinDone = nil
			if err != nil {
				return fmt.Errorf("copy stdin to port-forward: %w", err)
			}
		case err := <-stdoutDone:
			if err != nil {
				return fmt.Errorf("copy port-forward to stdout: %w", err)
			}
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func portForwardURL(host string, namespace string, podName string) (*url.URL, error) {
	endpoint, err := url.Parse(host)
	if err != nil {
		return nil, fmt.Errorf("parse Kubernetes API server URL: %w", err)
	}
	endpoint.Path = "/api/v1/namespaces/" + url.PathEscape(namespace) + "/pods/" + url.PathEscape(podName) + "/portforward"
	endpoint.RawQuery = ""
	endpoint.Fragment = ""
	return endpoint, nil
}
