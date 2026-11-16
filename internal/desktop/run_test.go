package desktop

import (
	"os"
	"testing"
)

func TestConfigureLinuxPortalFileChooser(t *testing.T) {
	t.Run("sets GTK_USE_PORTAL on linux when unset", func(t *testing.T) {
		orig, had := os.LookupEnv("GTK_USE_PORTAL")
		t.Cleanup(func() {
			if had {
				_ = os.Setenv("GTK_USE_PORTAL", orig)
			} else {
				_ = os.Unsetenv("GTK_USE_PORTAL")
			}
		})
		if err := os.Unsetenv("GTK_USE_PORTAL"); err != nil {
			t.Fatal(err)
		}

		configureLinuxPortalFileChooser("linux", os.LookupEnv, os.Setenv)

		got, ok := os.LookupEnv("GTK_USE_PORTAL")
		if !ok || got != "1" {
			t.Fatalf("GTK_USE_PORTAL = (%q, %v), want (\"1\", true)", got, ok)
		}
	})

	t.Run("respects explicit GTK_USE_PORTAL=0", func(t *testing.T) {
		t.Setenv("GTK_USE_PORTAL", "0")

		configureLinuxPortalFileChooser("linux", os.LookupEnv, os.Setenv)

		got, ok := os.LookupEnv("GTK_USE_PORTAL")
		if !ok || got != "0" {
			t.Fatalf("GTK_USE_PORTAL = (%q, %v), want (\"0\", true)", got, ok)
		}
	})

	t.Run("respects explicit empty GTK_USE_PORTAL", func(t *testing.T) {
		t.Setenv("GTK_USE_PORTAL", "")

		configureLinuxPortalFileChooser("linux", os.LookupEnv, os.Setenv)

		got, ok := os.LookupEnv("GTK_USE_PORTAL")
		if !ok || got != "" {
			t.Fatalf("GTK_USE_PORTAL = (%q, %v), want (\"\", true)", got, ok)
		}
	})

	t.Run("no-op outside linux", func(t *testing.T) {
		called := false
		configureLinuxPortalFileChooser("darwin", func(string) (string, bool) {
			return "", false
		}, func(string, string) error {
			called = true
			return nil
		})
		if called {
			t.Fatal("setenv called on non-linux OS")
		}
	})
}
