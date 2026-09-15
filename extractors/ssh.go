package extractors

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"afterloggrator/parsers"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }

// ReadRemoteFile verifies the remote host against a required known_hosts file.
func ReadRemoteFile(ctx context.Context, host, user, keyPath, knownHosts, filename string, follow bool, emit func(parsers.LogEntry) error) error {
	key, err := os.ReadFile(keyPath)
	if err != nil {
		return err
	}
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return err
	}
	verify, err := knownhosts.New(knownHosts)
	if err != nil {
		return err
	}
	if _, _, err = net.SplitHostPort(host); err != nil {
		host = net.JoinHostPort(strings.Trim(host, "[]"), "22")
	}
	conn, err := (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, "tcp", host)
	if err != nil {
		return err
	}
	defer conn.Close()
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			conn.Close()
		case <-done:
		}
	}()
	conn.SetDeadline(time.Now().Add(15 * time.Second))
	cc, chans, reqs, err := ssh.NewClientConn(conn, host, &ssh.ClientConfig{User: user, Auth: []ssh.AuthMethod{ssh.PublicKeys(signer)}, HostKeyCallback: verify})
	if err != nil {
		return err
	}
	conn.SetDeadline(time.Time{})
	client := ssh.NewClient(cc, chans, reqs)
	defer client.Close()
	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()
	out, err := session.StdoutPipe()
	if err != nil {
		return err
	}
	// Bound stderr independently, so a noisy remote command cannot exhaust memory.
	session.Stderr = os.Stderr
	command := "cat -- " + shellQuote(filename)
	if follow {
		command = "tail -n 0 -F -- " + shellQuote(filename)
	}
	if err = session.Start(command); err != nil {
		return err
	}
	sc := bufio.NewScanner(out)
	sc.Buffer(make([]byte, 64*1024), 10*1024*1024)
	for sc.Scan() {
		if err = emit(parsers.LogEntry{Filename: filename, Line: sc.Text()}); err != nil {
			return err
		}
	}
	if err = sc.Err(); err != nil {
		return err
	}
	return session.Wait()
}

// StreamRemoteFile is retained for callers of the old interface.
func StreamRemoteFile(ctx context.Context, host, user, keyPath, filename string, follow bool, color string, ch chan<- parsers.LogEntry) {
	home, _ := os.UserHomeDir()
	err := ReadRemoteFile(ctx, host, user, keyPath, home+"/.ssh/known_hosts", filename, follow, func(e parsers.LogEntry) error {
		e.Host = host
		e.Color = color
		select {
		case ch <- e:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "SSH:", err)
	}
}
