package kube

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/httpstream"
	"k8s.io/client-go/tools/portforward"
)

func TestReadSSHIdentificationAcceptsImmediateBanner(t *testing.T) {
	got, err := readSSHIdentification(context.Background(), strings.NewReader("SSH-2.0-OpenSSH_9.9\r\n"), time.Second, defaultSSHBannerMaxBytes)
	if err != nil {
		t.Fatalf("readSSHIdentification: %v", err)
	}
	if string(got) != "SSH-2.0-OpenSSH_9.9\r\n" {
		t.Fatalf("banner = %q", string(got))
	}
}

func TestReadSSHIdentificationAcceptsLegalPreBannerLines(t *testing.T) {
	input := "NOTICE one\r\nNOTICE two\nSSH-1.99-test-server\r\n"
	got, err := readSSHIdentification(context.Background(), strings.NewReader(input), time.Second, defaultSSHBannerMaxBytes)
	if err != nil {
		t.Fatalf("readSSHIdentification: %v", err)
	}
	if string(got) != input {
		t.Fatalf("buffered = %q, want %q", string(got), input)
	}
}

func TestReadSSHIdentificationRejectsUnsupportedProtocol(t *testing.T) {
	_, err := readSSHIdentification(context.Background(), strings.NewReader("SSH-1.5-old-server\r\n"), time.Second, defaultSSHBannerMaxBytes)
	if err == nil {
		t.Fatal("expected error")
	}
	if !isTerminalAttemptError(err) {
		t.Fatalf("error = %T %v, want terminal", err, err)
	}
	if !strings.Contains(err.Error(), "unsupported SSH protocol") {
		t.Fatalf("error = %v", err)
	}
}

func TestReadSSHIdentificationReportsEOFBeforeBanner(t *testing.T) {
	_, err := readSSHIdentification(context.Background(), strings.NewReader("NOTICE only\r\n"), time.Second, defaultSSHBannerMaxBytes)
	if !errors.Is(err, errSSHBannerEOF) {
		t.Fatalf("error = %v, want EOF before banner", err)
	}
}

func TestReadSSHIdentificationReportsBufferCapExceeded(t *testing.T) {
	_, err := readSSHIdentification(context.Background(), strings.NewReader("12345"), time.Second, 4)
	if err == nil {
		t.Fatal("expected error")
	}
	if !isTerminalAttemptError(err) {
		t.Fatalf("error = %T %v, want terminal", err, err)
	}
	if !errors.Is(err, errSSHBannerTooLong) {
		t.Fatalf("error = %v, want buffer cap", err)
	}
}

func TestReadSSHIdentificationReportsBannerSilenceTimeout(t *testing.T) {
	stream := newFakeSSHStream(nil)
	_, err := readSSHIdentification(context.Background(), stream, 5*time.Millisecond, defaultSSHBannerMaxBytes)
	_ = stream.Reset()
	if !errors.Is(err, errSSHBannerSilence) {
		t.Fatalf("error = %v, want silence timeout", err)
	}
}

func TestProxySSHPortForwardRetriesRemoteErrorAndWritesBannerOnce(t *testing.T) {
	firstError := newFakeSSHStream(strings.NewReader("dial tcp 10.0.0.1:22: connect: no route to host"))
	firstData := newFakeSSHStream(nil)
	secondError := newFakeSSHStream(strings.NewReader(""))
	secondData := newFakeSSHStream(strings.NewReader("SSH-2.0-ready\r\n"))
	conn := newFakeSSHConnection(
		&fakeSSHAttempt{errorStream: firstError, dataStream: firstData},
		&fakeSSHAttempt{errorStream: secondError, dataStream: secondData},
	)
	dialer := &fakeSSHDialer{connection: conn}
	var stdout bytes.Buffer
	var sleeps []time.Duration

	err := proxySSHPortForward(context.Background(), sshProxyConfig{
		Dialer:               dialer,
		RemotePort:           22,
		Stdin:                strings.NewReader(""),
		Stdout:               &stdout,
		ReadyTimeout:         time.Second,
		RetryDelay:           defaultSSHRetryDelay,
		BannerSilenceTimeout: time.Second,
		BannerMaxBytes:       defaultSSHBannerMaxBytes,
		Sleep: func(_ context.Context, duration time.Duration) error {
			sleeps = append(sleeps, duration)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("proxySSHPortForward: %v", err)
	}
	if got := stdout.String(); got != "SSH-2.0-ready\r\n" {
		t.Fatalf("stdout = %q", got)
	}
	if !reflect.DeepEqual(sleeps, []time.Duration{defaultSSHRetryDelay}) {
		t.Fatalf("sleeps = %#v", sleeps)
	}
	if !firstData.wasReset() {
		t.Fatal("first data stream was not reset after failed attempt")
	}
	if got := secondData.Headers().Get(corev1.PortForwardRequestIDHeader); got != "1" {
		t.Fatalf("second requestID = %q, want 1", got)
	}
	if !reflect.DeepEqual(dialer.protocols, []string{portforward.PortForwardProtocolV1Name}) {
		t.Fatalf("dial protocols = %#v", dialer.protocols)
	}
}

func TestProxySSHPortForwardCopiesOnEstablishedStream(t *testing.T) {
	data := newFakeSSHStream(nil)
	data.reader = &serverAfterWriteReader{
		stream: data,
		prefix: strings.NewReader("SSH-2.0-ready\r\n"),
		after:  strings.NewReader("server-after\n"),
	}
	conn := newFakeSSHConnection(&fakeSSHAttempt{
		errorStream: newFakeSSHStream(strings.NewReader("")),
		dataStream:  data,
	})
	var stdout bytes.Buffer

	err := proxySSHPortForward(context.Background(), sshProxyConfig{
		Dialer:               &fakeSSHDialer{connection: conn},
		RemotePort:           22,
		Stdin:                strings.NewReader("client-input\n"),
		Stdout:               &stdout,
		ReadyTimeout:         time.Second,
		BannerSilenceTimeout: time.Second,
		BannerMaxBytes:       defaultSSHBannerMaxBytes,
		Sleep:                func(context.Context, time.Duration) error { return nil },
	})
	if err != nil {
		t.Fatalf("proxySSHPortForward: %v", err)
	}
	if got := stdout.String(); got != "SSH-2.0-ready\r\nserver-after\n" {
		t.Fatalf("stdout = %q", got)
	}
	if got := data.Written(); got != "client-input\n" {
		t.Fatalf("stream writes = %q", got)
	}
}

func TestProxySSHPortForwardReadinessTimeoutDoesNotLimitEstablishedSession(t *testing.T) {
	data := newFakeSSHStream(nil)
	data.reader = &serverAfterWriteReader{
		stream: data,
		prefix: strings.NewReader("SSH-2.0-ready\r\n"),
		after:  strings.NewReader("after-readiness-deadline\n"),
		delay:  25 * time.Millisecond,
	}
	conn := newFakeSSHConnection(&fakeSSHAttempt{
		errorStream: newFakeSSHStream(strings.NewReader("")),
		dataStream:  data,
	})
	var stdout bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err := proxySSHPortForward(ctx, sshProxyConfig{
		Dialer:               &fakeSSHDialer{connection: conn},
		RemotePort:           22,
		Stdin:                strings.NewReader("client-input\n"),
		Stdout:               &stdout,
		ReadyTimeout:         5 * time.Millisecond,
		BannerSilenceTimeout: time.Second,
		BannerMaxBytes:       defaultSSHBannerMaxBytes,
		Sleep:                func(context.Context, time.Duration) error { return nil },
	})
	if err != nil {
		t.Fatalf("proxySSHPortForward: %v", err)
	}
	if got := stdout.String(); got != "SSH-2.0-ready\r\nafter-readiness-deadline\n" {
		t.Fatalf("stdout = %q", got)
	}
}

func TestProxySSHPortForwardDoesNotConsumeStdinDuringFailedAttempts(t *testing.T) {
	firstError := newFakeSSHStream(strings.NewReader("connect: no route to host"))
	firstData := newFakeSSHStream(nil)
	secondData := newFakeSSHStream(nil)
	secondData.reader = &serverAfterWriteReader{
		stream: secondData,
		prefix: strings.NewReader("SSH-2.0-ready\r\n"),
		after:  strings.NewReader(""),
	}
	conn := newFakeSSHConnection(
		&fakeSSHAttempt{errorStream: firstError, dataStream: firstData},
		&fakeSSHAttempt{errorStream: newFakeSSHStream(strings.NewReader("")), dataStream: secondData},
	)
	var stdout bytes.Buffer

	err := proxySSHPortForward(context.Background(), sshProxyConfig{
		Dialer:               &fakeSSHDialer{connection: conn},
		RemotePort:           22,
		Stdin:                strings.NewReader("client-input\n"),
		Stdout:               &stdout,
		ReadyTimeout:         time.Second,
		BannerSilenceTimeout: time.Second,
		BannerMaxBytes:       defaultSSHBannerMaxBytes,
		Sleep:                func(context.Context, time.Duration) error { return nil },
	})
	if err != nil {
		t.Fatalf("proxySSHPortForward: %v", err)
	}
	if got := firstData.Written(); got != "" {
		t.Fatalf("failed attempt wrote stdin = %q", got)
	}
	if got := secondData.Written(); got != "client-input\n" {
		t.Fatalf("successful attempt writes = %q", got)
	}
}

func TestProxySSHPortForwardTimeoutReportsLastRemoteError(t *testing.T) {
	conn := newFakeSSHConnection(&fakeSSHAttempt{
		errorStream: newFakeSSHStream(strings.NewReader("connect: no route to host")),
		dataStream:  newFakeSSHStream(nil),
	})
	err := proxySSHPortForward(context.Background(), sshProxyConfig{
		Dialer:               &fakeSSHDialer{connection: conn},
		RemotePort:           22,
		Stdin:                strings.NewReader(""),
		Stdout:               io.Discard,
		ReadyTimeout:         5 * time.Millisecond,
		RetryDelay:           time.Second,
		BannerSilenceTimeout: time.Second,
		BannerMaxBytes:       defaultSSHBannerMaxBytes,
		Sleep: func(ctx context.Context, _ time.Duration) error {
			<-ctx.Done()
			return ctx.Err()
		},
	})
	if err == nil {
		t.Fatal("expected timeout")
	}
	if !strings.Contains(err.Error(), "last remote error") || !strings.Contains(err.Error(), "no route to host") {
		t.Fatalf("error = %v", err)
	}
}

type fakeSSHDialer struct {
	connection httpstream.Connection
	protocols  []string
	protocol   string
	err        error
}

func (d *fakeSSHDialer) Dial(protocols ...string) (httpstream.Connection, string, error) {
	d.protocols = append([]string(nil), protocols...)
	protocol := d.protocol
	if protocol == "" {
		protocol = portforward.PortForwardProtocolV1Name
	}
	return d.connection, protocol, d.err
}

type fakeSSHConnection struct {
	mu       sync.Mutex
	attempts []*fakeSSHAttempt
	next     int
	closeCh  chan bool
	close    sync.Once
}

type fakeSSHAttempt struct {
	errorStream *fakeSSHStream
	dataStream  *fakeSSHStream
}

func newFakeSSHConnection(attempts ...*fakeSSHAttempt) *fakeSSHConnection {
	return &fakeSSHConnection{
		attempts: attempts,
		closeCh:  make(chan bool),
	}
}

func (c *fakeSSHConnection) CreateStream(headers http.Header) (httpstream.Stream, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.next >= len(c.attempts) {
		return nil, fmt.Errorf("no fake attempt %d", c.next)
	}
	attempt := c.attempts[c.next]
	switch headers.Get(corev1.StreamType) {
	case corev1.StreamTypeError:
		attempt.errorStream.headers = cloneHeader(headers)
		return attempt.errorStream, nil
	case corev1.StreamTypeData:
		attempt.dataStream.headers = cloneHeader(headers)
		c.next++
		return attempt.dataStream, nil
	default:
		return nil, fmt.Errorf("unsupported stream type %q", headers.Get(corev1.StreamType))
	}
}

func (c *fakeSSHConnection) Close() error {
	c.close.Do(func() {
		close(c.closeCh)
	})
	return nil
}

func (c *fakeSSHConnection) CloseChan() <-chan bool {
	return c.closeCh
}

func (c *fakeSSHConnection) SetIdleTimeout(time.Duration) {}

func (c *fakeSSHConnection) RemoveStreams(...httpstream.Stream) {}

type fakeSSHStream struct {
	headers http.Header
	reader  io.Reader

	writeMu sync.Mutex
	writes  bytes.Buffer
	writeCh chan struct{}
	wrote   sync.Once

	resetCh chan struct{}
	reset   sync.Once
}

func newFakeSSHStream(reader io.Reader) *fakeSSHStream {
	return &fakeSSHStream{
		reader:  reader,
		writeCh: make(chan struct{}),
		resetCh: make(chan struct{}),
	}
}

func (s *fakeSSHStream) Read(p []byte) (int, error) {
	if s.reader == nil {
		<-s.resetCh
		return 0, io.EOF
	}
	return s.reader.Read(p)
}

func (s *fakeSSHStream) Write(p []byte) (int, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	n, err := s.writes.Write(p)
	s.wrote.Do(func() {
		close(s.writeCh)
	})
	return n, err
}

func (s *fakeSSHStream) Close() error {
	return nil
}

func (s *fakeSSHStream) Reset() error {
	s.reset.Do(func() {
		close(s.resetCh)
	})
	return nil
}

func (s *fakeSSHStream) Headers() http.Header {
	return s.headers
}

func (s *fakeSSHStream) Identifier() uint32 {
	return 0
}

func (s *fakeSSHStream) Written() string {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.writes.String()
}

func (s *fakeSSHStream) wasReset() bool {
	select {
	case <-s.resetCh:
		return true
	default:
		return false
	}
}

type serverAfterWriteReader struct {
	stream *fakeSSHStream
	prefix *strings.Reader
	after  *strings.Reader
	delay  time.Duration
	waited bool
	slept  bool
}

func (r *serverAfterWriteReader) Read(p []byte) (int, error) {
	if r.prefix != nil {
		n, err := r.prefix.Read(p)
		if n > 0 || err != io.EOF {
			return n, err
		}
		r.prefix = nil
	}
	if !r.waited {
		select {
		case <-r.stream.writeCh:
			r.waited = true
		case <-r.stream.resetCh:
			return 0, io.EOF
		}
	}
	if r.delay > 0 && !r.slept {
		time.Sleep(r.delay)
		r.slept = true
	}
	return r.after.Read(p)
}

func cloneHeader(source http.Header) http.Header {
	cloned := http.Header{}
	for key, values := range source {
		cloned[key] = append([]string(nil), values...)
	}
	return cloned
}
