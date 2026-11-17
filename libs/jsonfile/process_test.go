package jsonfile_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/candy-tools/todo/libs/jsonfile"
)

const helperEnv = "JSONFILE_HELPER"

// helperCmd re-executes the test binary as a helper process running mode.
func helperCmd(mode string) *exec.Cmd {
	cmd := exec.Command(os.Args[0], "-test.run=TestHelperProcess")
	cmd.Env = append(os.Environ(), helperEnv+"="+mode)
	cmd.Stderr = os.Stderr
	return cmd
}

// splitLastField splits s at its last ':' that is not a Windows drive colon
// (the `:` of a leading `C:`), so both halves may be absolute paths.
func splitLastField(s string) (string, string, bool) {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] != ':' {
			continue
		}
		driveColon := i >= 1 && isASCIILetter(s[i-1]) && (i == 1 || s[i-2] == ':')
		if driveColon {
			continue
		}
		return s[:i], s[i+1:], true
	}
	return "", "", false
}

func isASCIILetter(c byte) bool {
	return ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z')
}

// TestHelperProcess is not a real test: it is the body of the helper
// processes started by the multi-process tests, and returns at once otherwise.
func TestHelperProcess(t *testing.T) {
	spec := os.Getenv(helperEnv)
	if spec == "" {
		return
	}
	if err := runHelper(spec); err != nil {
		fmt.Fprintln(os.Stderr, "helper:", err)
		os.Exit(1)
	}
	os.Exit(0)
}

func runHelper(spec string) error {
	mode, rest, ok := strings.Cut(spec, ":")
	if !ok {
		return fmt.Errorf("bad spec %q", spec)
	}
	first, second, ok := splitLastField(rest)
	if !ok {
		return fmt.Errorf("bad spec %q", spec)
	}
	s := jsonfile.New(jsonfile.Config[doc]{Path: first, LockTimeout: 30 * time.Second})
	switch mode {
	case "incr":
		n, err := strconv.Atoi(second)
		if err != nil {
			return err
		}
		for range n {
			if _, err := s.Update(context.Background(), func(v *doc, _ jsonfile.Snapshot) (bool, error) {
				v.N++
				return true, nil
			}); err != nil {
				return err
			}
		}
		return nil
	case "hold":
		_, err := s.Update(context.Background(), func(*doc, jsonfile.Snapshot) (bool, error) {
			if err := os.WriteFile(second, nil, 0o644); err != nil {
				return false, err
			}
			time.Sleep(2 * time.Second)
			return false, nil
		})
		return err
	default:
		return fmt.Errorf("unknown mode %q", mode)
	}
}

func TestMultiProcessUpdates(t *testing.T) {
	path := tempPath(t)
	const procs, rounds = 4, 50
	cmds := make([]*exec.Cmd, procs)
	waited := make([]bool, procs)
	for i := range cmds {
		cmds[i] = helperCmd(fmt.Sprintf("incr:%s:%d", path, rounds))
		if err := cmds[i].Start(); err != nil {
			t.Fatalf("start helper: %v", err)
		}
		// Kill and reap the helper if the test bails out before waiting.
		t.Cleanup(func() {
			if !waited[i] {
				_ = cmds[i].Process.Kill()
				_ = cmds[i].Wait()
			}
		})
	}
	for i, cmd := range cmds {
		err := cmd.Wait()
		waited[i] = true
		if err != nil {
			t.Errorf("helper: %v", err)
		}
	}
	v, _, err := jsonfile.New(jsonfile.Config[doc]{Path: path}).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if v.N != procs*rounds {
		t.Errorf("N = %d, want %d", v.N, procs*rounds)
	}
}

func TestMultiProcessBusy(t *testing.T) {
	path := tempPath(t)
	ready := filepath.Join(filepath.Dir(path), "ready")
	cmd := helperCmd("hold:" + path + ":" + ready)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("helper never signalled it holds the lock")
		}
		time.Sleep(10 * time.Millisecond)
	}

	s := jsonfile.New(jsonfile.Config[doc]{Path: path, LockTimeout: 100 * time.Millisecond})
	if _, err := s.Update(context.Background(), setN(1)); !errors.Is(err, jsonfile.ErrBusy) {
		t.Fatalf("err = %v, want ErrBusy", err)
	}
}
