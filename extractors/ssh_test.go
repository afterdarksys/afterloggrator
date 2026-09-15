package extractors

import (
	"afterloggrator/parsers"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSSHHostVerificationAndQuotedPath(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "key")
	os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}), 0600)
	for _, trusted := range []bool{true, false} {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		done := make(chan struct{})
		commands := make(chan string, 1)
		go func() {
			defer close(done)
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			defer conn.Close()
			cfg := &ssh.ServerConfig{NoClientAuth: true}
			cfg.AddHostKey(signer)
			server, chans, reqs, err := ssh.NewServerConn(conn, cfg)
			if err != nil {
				return
			}
			defer server.Close()
			go ssh.DiscardRequests(reqs)
			for ch := range chans {
				channel, requests, err := ch.Accept()
				if err != nil {
					return
				}
				for req := range requests {
					if req.Type == "exec" {
						var command struct{ Command string }
						ssh.Unmarshal(req.Payload, &command)
						commands <- command.Command
						req.Reply(true, nil)
						io.WriteString(channel, "first\nlast\n")
						channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{0}))
						channel.Close()
						return
					}
					req.Reply(false, nil)
				}
			}
		}()
		known := filepath.Join(dir, "known_hosts")
		data := ""
		if trusted {
			data = knownhosts.Line([]string{listener.Addr().String()}, signer.PublicKey()) + "\n"
		}
		os.WriteFile(known, []byte(data), 0600)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		var lines []string
		path := "/logs/a'; echo injected"
		err = ReadRemoteFile(ctx, listener.Addr().String(), "ops", keyPath, known, path, false, func(e parsers.LogEntry) error { lines = append(lines, e.Line); return nil })
		cancel()
		listener.Close()
		<-done
		if trusted {
			if err != nil || len(lines) != 2 {
				t.Fatalf("%v %v", err, lines)
			}
			if command := <-commands; command != "cat -- "+shellQuote(path) {
				t.Fatal(command)
			}
		} else {
			if err == nil || len(lines) != 0 {
				t.Fatal("unknown host accepted")
			}
		}
	}
}
