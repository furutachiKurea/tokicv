//go:build linux

package desktop

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func startIsolatedDBus(t *testing.T, root string) string {
	t.Helper()
	address, stop, err := launchIsolatedDBus(root, isolatedDBusConfig(root))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(stop)
	return address
}

func launchIsolatedDBus(root string, configData []byte) (string, func(), error) {
	daemon, err := exec.LookPath("dbus-daemon")
	if err != nil {
		return "", nil, fmt.Errorf("dbus-daemon is required for an isolated WebKitGTK settings session: %w", err)
	}
	config := filepath.Join(root, "session.conf")
	if err := os.WriteFile(config, configData, 0o600); err != nil {
		return "", nil, fmt.Errorf("write isolated D-Bus config: %w", err)
	}
	logFile, err := os.Create(filepath.Join(root, "dbus.log"))
	if err != nil {
		return "", nil, fmt.Errorf("create isolated D-Bus log: %w", err)
	}
	cmd := exec.Command(daemon, "--config-file="+config, "--nofork", "--print-address=1")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Stderr = logFile
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = logFile.Close()
		return "", nil, fmt.Errorf("open isolated D-Bus output: %w", err)
	}
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return "", nil, fmt.Errorf("start isolated D-Bus: %w", err)
	}
	var once sync.Once
	stop := func() {
		once.Do(func() {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			_ = cmd.Wait()
			_ = logFile.Close()
		})
	}
	type result struct {
		address string
		err     error
	}
	address := make(chan result, 1)
	go func() {
		line, err := bufio.NewReader(stdout).ReadString('\n')
		address <- result{address: strings.TrimSpace(line), err: err}
	}()
	select {
	case result := <-address:
		if result.err != nil || result.address == "" {
			stop()
			return "", nil, fmt.Errorf("read isolated D-Bus address: %v; %s",
				result.err, readFileOrEmpty(filepath.Join(root, "dbus.log")))
		}
		return result.address, stop, nil
	case <-time.After(5 * time.Second):
		stop()
		return "", nil, fmt.Errorf("isolated D-Bus did not publish an address: %s",
			readFileOrEmpty(filepath.Join(root, "dbus.log")))
	}
}

func readFileOrEmpty(path string) string {
	data, _ := os.ReadFile(path)
	return string(data)
}

func isolatedDBusConfig(root string) []byte {
	return []byte(fmt.Sprintf(`<!DOCTYPE busconfig PUBLIC "-//freedesktop//DTD D-Bus Bus Configuration 1.0//EN"
 "http://www.freedesktop.org/standards/dbus/1.0/busconfig.dtd">
<busconfig>
  <type>session</type>
  <listen>unix:path=%s</listen>
  <auth>EXTERNAL</auth>
  <policy context="default">
    <allow send_destination="*" eavesdrop="true"/>
    <allow eavesdrop="true"/>
    <allow own="*"/>
  </policy>
  <limit name="max_connections_per_user">64</limit>
</busconfig>
`, filepath.Join(root, "dbus.sock")))
}

func TestIsolatedDBusSocketAndCleanupStayInOwnedDirectory(t *testing.T) {
	before := tmpDBusNames(t)
	var root string
	t.Run("lifecycle", func(t *testing.T) {
		var err error
		root, err = os.MkdirTemp("", "tokicv-dbus-")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(root) })
		address, stop, err := launchIsolatedDBus(root, isolatedDBusConfig(root))
		if err != nil {
			if strings.Contains(err.Error(), "dbus-daemon is required") {
				t.Skip(err)
			}
			t.Fatal(err)
		}
		if stop == nil {
			t.Fatal("isolated D-Bus returned no cleanup")
		}
		socket := filepath.Join(root, "dbus.sock")
		if !strings.Contains(address, "unix:path="+socket) {
			stop()
			t.Fatalf("D-Bus address = %q, want socket %q", address, socket)
		}
		if info, err := os.Stat(socket); err != nil || info.Mode()&os.ModeSocket == 0 {
			stop()
			t.Fatalf("isolated D-Bus socket = %v, mode %v", err, info)
		}
		stop()
	})
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Errorf("owned D-Bus directory was not removed: %v", err)
	}
	assertNoNewTmpDBusNames(t, before)
}

func TestIsolatedDBusFailureDoesNotCreateTmpSockets(t *testing.T) {
	before := tmpDBusNames(t)
	root, err := os.MkdirTemp("", "tokicv-dbus-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	address, stop, err := launchIsolatedDBus(root, []byte("<busconfig>"))
	if err == nil {
		if stop != nil {
			stop()
		}
		t.Fatalf("invalid isolated D-Bus config returned address %q", address)
	}
	if stop != nil {
		stop()
	}
	assertNoNewTmpDBusNames(t, before)
}

func tmpDBusNames(t *testing.T) map[string]bool {
	t.Helper()
	paths, err := filepath.Glob("/tmp/dbus-*")
	if err != nil {
		t.Fatal(err)
	}
	names := make(map[string]bool, len(paths))
	for _, path := range paths {
		names[filepath.Base(path)] = true
	}
	return names
}

func assertNoNewTmpDBusNames(t *testing.T, before map[string]bool) {
	t.Helper()
	after := tmpDBusNames(t)
	for name := range after {
		if !before[name] {
			t.Errorf("isolated D-Bus leaked /tmp/%s", name)
		}
	}
}
