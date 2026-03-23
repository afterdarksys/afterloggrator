package extractors

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"afterloggrator/parsers"
	"golang.org/x/crypto/ssh"
)

func StreamRemoteFile(ctx context.Context, host, user, keyPath, filename string, isFollow bool, color string, logChan chan<- parsers.LogEntry) {
	key, err := os.ReadFile(keyPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "SSH Key read error (%s): %v\n", host, err)
		return
	}

	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		fmt.Fprintf(os.Stderr, "SSH Key parse error (%s): %v\n", host, err)
		return
	}

	if !strings.Contains(host, ":") {
		host = host + ":22"
	}

	config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	client, err := ssh.Dial("tcp", host, config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "SSH Dial error (%s): %v\n", host, err)
		return
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		fmt.Fprintf(os.Stderr, "SSH Session error (%s): %v\n", host, err)
		return
	}
	defer session.Close()

	go func() {
		<-ctx.Done()
		session.Close()
		client.Close()
	}()

	stdout, err := session.StdoutPipe()
	if err != nil {
		return
	}

	cmd := fmt.Sprintf("cat %s", filename)
	if isFollow {
		cmd = fmt.Sprintf("tail -f %s", filename)
	}

	if err := session.Start(cmd); err != nil {
		fmt.Fprintf(os.Stderr, "SSH Start error (%s): %v\n", host, err)
		return
	}

	scanner := bufio.NewScanner(stdout)
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, 10*1024*1024)

	hostPrefix := strings.Split(host, ":")[0]

	for scanner.Scan() {
		logChan <- parsers.LogEntry{
			Filename:  fmt.Sprintf("%s:%s", hostPrefix, filename),
			Line:      scanner.Text(),
			Timestamp: time.Now(),
			Color:     color,
		}
	}
}
