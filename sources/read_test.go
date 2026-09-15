package sources

import (
	"afterloggrator/parsers"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLocalReadAndCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log")
	os.WriteFile(path, []byte("first\nlast"), 0600)
	var got []string
	err := local(context.Background(), path, false, func(e parsers.LogEntry) error { got = append(got, e.Line); return nil })
	if err != nil || len(got) != 2 || got[1] != "last" {
		t.Fatalf("%v %v", got, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err = local(ctx, path, false, func(parsers.LogEntry) error { t.Error("emitted after cancel"); return nil }); err == nil {
		t.Fatal("cancellation ignored")
	}
}
func TestFollowPartialLineAndRotation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log")
	os.WriteFile(path, nil, 0600)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan string, 8)
	done := make(chan error, 1)
	go func() {
		done <- local(ctx, path, true, func(e parsers.LogEntry) error { events <- e.Line; return nil })
	}()
	time.Sleep(150 * time.Millisecond)
	appendText := func(s string) {
		f, e := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
		if e != nil {
			t.Fatal(e)
		}
		f.WriteString(s)
		f.Close()
	}
	appendText("par")
	time.Sleep(150 * time.Millisecond)
	appendText("tial\n")
	select {
	case line := <-events:
		if line != "partial" {
			t.Fatal(line)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("lost partial line")
	}
	os.Rename(path, path+".1")
	os.WriteFile(path, []byte("rotated\n"), 0600)
	select {
	case line := <-events:
		if line != "rotated" {
			t.Fatal(line)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("rotation lost")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("follow failed to cancel")
	}
}
