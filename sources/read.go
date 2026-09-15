package sources

import (
	"afterloggrator/extractors"
	"afterloggrator/parsers"
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

type Options struct {
	Follow       bool
	Since, Until time.Time
}
type Emit func(parsers.LogEntry) error

func Read(ctx context.Context, s Source, opt Options, emit Emit) error {
	wrapped := func(e parsers.LogEntry) error {
		e.Source = s.Name
		e.Host = s.Host
		e.Service = s.Service
		e.Cluster = s.Cluster
		if e.Container == "" {
			e.Container = s.Container
		}
		if e.Filename == "" {
			e.Filename = s.Name
		}
		if e.ParsedFields == nil {
			e.ParsedFields = map[string]string{}
		}
		for k, v := range s.Labels {
			e.ParsedFields[k] = v
		}
		return emit(e)
	}
	switch s.Type {
	case "file":
		return local(ctx, s.Path, opt.Follow, wrapped)
	case "ssh":
		return extractors.ReadRemoteFile(ctx, s.Host, s.User, s.KeyFile, s.KnownHosts, s.Path, opt.Follow, wrapped)
	case "docker", "kubernetes":
		return command(ctx, s, opt, wrapped)
	default:
		if opt.Follow {
			return fmt.Errorf("%s sources support finite searches only; omit -f", s.Type)
		}
		switch s.Type {
		case "aws":
			return awsLogs(ctx, s, opt, wrapped)
		case "oci":
			return ociLogs(ctx, s, opt, wrapped)
		default:
			return httpLogs(ctx, s, opt, wrapped)
		}
	}
}
func scan(ctx context.Context, r io.Reader, name string, emit Emit) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 10*1024*1024)
	for sc.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := emit(parsers.LogEntry{Filename: name, Line: sc.Text()}); err != nil {
			return err
		}
	}
	return sc.Err()
}
func local(ctx context.Context, path string, follow bool, emit Emit) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("file source requires a regular file: %s", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { f.Close() }()
	if !follow {
		r, err := extractors.GetReader(f, path)
		if err != nil {
			return err
		}
		if r != f {
			defer r.Close()
		}
		return scan(ctx, r, path, emit)
	}
	lower := strings.ToLower(path)
	for _, ext := range []string{".gz", ".bz2", ".zip", ".tar", ".z", ".tgz"} {
		if strings.HasSuffix(lower, ext) {
			return fmt.Errorf("cannot follow archives: %s", path)
		}
	}
	offset, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return err
	}
	reader := bufio.NewReader(f)
	var pending []byte
	timer := time.NewTicker(100 * time.Millisecond)
	defer timer.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		part, err := reader.ReadSlice('\n')
		offset += int64(len(part))
		if len(pending)+len(part) > 10*1024*1024 {
			return fmt.Errorf("line exceeds 10 MiB")
		}
		pending = append(pending, part...)
		if err == bufio.ErrBufferFull {
			continue
		}
		if err == nil {
			if e := emit(parsers.LogEntry{Filename: path, Line: strings.TrimSuffix(strings.TrimSuffix(string(pending), "\n"), "\r")}); e != nil {
				return e
			}
			pending = nil
			continue
		}
		if err != io.EOF {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
		old, e := f.Stat()
		if e != nil {
			return e
		}
		current, e := os.Stat(path)
		if os.IsNotExist(e) {
			continue
		}
		if e != nil {
			return e
		}
		if !os.SameFile(old, current) {
			next, e := os.Open(path)
			if e != nil {
				return e
			}
			f.Close()
			f = next
			reader.Reset(f)
			offset = 0
			pending = nil
		} else if current.Size() < offset {
			if _, e = f.Seek(0, io.SeekStart); e != nil {
				return e
			}
			reader.Reset(f)
			offset = 0
			pending = nil
		}
	}
}
func command(ctx context.Context, s Source, opt Options, emit Emit) error {
	var name string
	var args []string
	if s.Type == "docker" {
		name = "docker"
		if s.Context != "" {
			args = append(args, "--context", s.Context)
		}
		args = append(args, "logs", "--timestamps")
		if opt.Follow {
			args = append(args, "--follow", "--tail", "0")
		}
		if !opt.Since.IsZero() {
			args = append(args, "--since", opt.Since.Format(time.RFC3339Nano))
		}
		if !opt.Until.IsZero() {
			args = append(args, "--until", opt.Until.Format(time.RFC3339Nano))
		}
		args = append(args, "--", s.Container)
	} else {
		name = "kubectl"
		args = []string{"--context", s.Context, "--namespace", s.Namespace, "logs", s.Pod, "--container", s.Container, "--timestamps=true"}
		if opt.Follow {
			args = append(args, "--follow", "--tail=0")
		}
		if !opt.Since.IsZero() {
			args = append(args, "--since-time", opt.Since.Format(time.RFC3339Nano))
		}
	}
	cmd := exec.CommandContext(ctx, name, args...)
	// Docker emits container stderr on command stderr, so merge both through one pipe.
	r, w, err := os.Pipe()
	if err != nil {
		return err
	}
	defer r.Close()
	defer w.Close()
	cmd.Stdout = w
	cmd.Stderr = w
	if err = cmd.Start(); err != nil {
		return err
	}
	w.Close()
	err = scan(ctx, r, s.Name, emit)
	if err != nil {
		cmd.Process.Kill()
	}
	waitErr := cmd.Wait()
	if err != nil {
		return err
	}
	return waitErr
}
