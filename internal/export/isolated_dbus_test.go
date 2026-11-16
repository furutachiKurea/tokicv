//go:build linux

package export

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportDBusStaysInRuntimeDirAndDoesNotLeakTmpSockets(t *testing.T) {
	if _, err := exec.LookPath("dbus-daemon"); err != nil {
		t.Skip("dbus-daemon is required")
	}
	before := exportTmpDBusNames(t)
	runtimeDir, err := os.MkdirTemp("", "tokicv-dbus-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(runtimeDir) })
	address, cmd, err := startIsolatedExportDBus(runtimeDir)
	if err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(runtimeDir, "dbus.sock")
	if !strings.Contains(address, "unix:path="+socket) {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatalf("D-Bus address = %q, want socket %q", address, socket)
	}
	if info, err := os.Stat(socket); err != nil || info.Mode()&os.ModeSocket == 0 {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatalf("isolated D-Bus socket = %v, mode %v", err, info)
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatalf("stop isolated D-Bus: %v", err)
	}
	_ = cmd.Wait()
	assertNoNewExportTmpDBusNames(t, before)
}

func TestExportDBusFailureDoesNotLeakTmpSockets(t *testing.T) {
	if _, err := exec.LookPath("dbus-daemon"); err != nil {
		t.Skip("dbus-daemon is required")
	}
	before := exportTmpDBusNames(t)
	blocker := filepath.Join(t.TempDir(), "runtime")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	address, cmd, err := startIsolatedExportDBus(blocker)
	if err == nil {
		if cmd != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
		t.Fatalf("startIsolatedExportDBus accepted a file runtime dir and returned %q", address)
	}
	assertNoNewExportTmpDBusNames(t, before)
}

func exportTmpDBusNames(t *testing.T) map[string]bool {
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

func assertNoNewExportTmpDBusNames(t *testing.T, before map[string]bool) {
	t.Helper()
	after := exportTmpDBusNames(t)
	for name := range after {
		if !before[name] {
			t.Errorf("isolated D-Bus leaked /tmp/%s", name)
		}
	}
}
