package export

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"
)

type printLocaleEnvironment struct {
	mu     sync.Mutex
	values map[string]string
	events []string
}

func newPrintLocaleEnvironment(values map[string]string) *printLocaleEnvironment {
	copied := make(map[string]string, len(values))
	for name, value := range values {
		copied[name] = value
	}
	return &printLocaleEnvironment{values: copied}
}

func (e *printLocaleEnvironment) lookup(name string) (string, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	value, ok := e.values[name]
	return value, ok
}

func (e *printLocaleEnvironment) setenv(name, value string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.values[name] = value
	e.events = append(e.events, "set "+name+"="+value)
	return nil
}

func (e *printLocaleEnvironment) unsetenv(name string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.values, name)
	e.events = append(e.events, "unset "+name)
	return nil
}

func (e *printLocaleEnvironment) record(event string) {
	e.mu.Lock()
	e.events = append(e.events, event)
	e.mu.Unlock()
}

func (e *printLocaleEnvironment) recordedEvents() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string(nil), e.events...)
}

func TestPrintLocaleRestoresLanguageAfterSuccessAndFailure(t *testing.T) {
	printErr := errors.New("print failed")
	for _, tc := range []struct {
		name      string
		wantValue string
		wantSet   bool
		fail      bool
	}{
		{name: "unset success"},
		{name: "unset failure", fail: true},
		{name: "empty success", wantSet: true},
		{name: "empty failure", wantSet: true, fail: true},
		{name: "custom success", wantValue: "zh_CN:en", wantSet: true},
		{name: "custom failure", wantValue: "zh_CN:en", wantSet: true, fail: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := newPrintLocaleEnvironment(nil)
			if tc.wantSet {
				env.values["LANGUAGE"] = tc.wantValue
			}

			var seen string
			pdf, err := withPrintLocale(
				context.Background(),
				"linux",
				env.lookup,
				env.setenv,
				env.unsetenv,
				func() ([]byte, error) {
					seen, _ = env.lookup("LANGUAGE")
					if tc.fail {
						return nil, printErr
					}
					return []byte("%PDF-"), nil
				},
			)

			if seen != "C" {
				t.Fatalf("LANGUAGE during print = %q, want C", seen)
			}
			if tc.fail {
				if !errors.Is(err, printErr) {
					t.Fatalf("print error = %v, want %v", err, printErr)
				}
				if pdf != nil {
					t.Fatalf("PDF = %q after failed print, want nil", pdf)
				}
			} else {
				if err != nil {
					t.Fatalf("print: %v", err)
				}
				if string(pdf) != "%PDF-" {
					t.Fatalf("PDF = %q, want the printed bytes", pdf)
				}
			}

			got, set := env.lookup("LANGUAGE")
			if set != tc.wantSet || got != tc.wantValue {
				t.Fatalf("restored LANGUAGE = %q (set=%t), want %q (set=%t)",
					got, set, tc.wantValue, tc.wantSet)
			}
		})
	}
}

func TestPrintLocaleSerializesConcurrentPrints(t *testing.T) {
	env := newPrintLocaleEnvironment(map[string]string{"LANGUAGE": "zh_CN"})
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	secondStarted := make(chan struct{})
	results := make(chan error, 2)

	print := func(name string, block bool) func() ([]byte, error) {
		return func() ([]byte, error) {
			language, _ := env.lookup("LANGUAGE")
			env.record("print " + name + " " + language)
			if block {
				close(firstStarted)
				<-releaseFirst
			} else {
				close(secondStarted)
			}
			return []byte("%PDF-"), nil
		}
	}

	go func() {
		_, err := withPrintLocale(
			context.Background(), "linux", env.lookup, env.setenv, env.unsetenv,
			print("first", true),
		)
		results <- err
	}()
	<-firstStarted

	secondReady := make(chan struct{})
	go func() {
		close(secondReady)
		_, err := withPrintLocale(
			context.Background(), "linux", env.lookup, env.setenv, env.unsetenv,
			print("second", false),
		)
		results <- err
	}()
	<-secondReady

	select {
	case <-secondStarted:
		t.Fatal("the second print started before the first print scope ended")
	case <-time.After(100 * time.Millisecond):
	}
	if language, _ := env.lookup("LANGUAGE"); language != "C" {
		t.Fatalf("LANGUAGE during the first blocked print = %q, want C", language)
	}

	close(releaseFirst)
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}

	want := []string{
		"set LANGUAGE=C",
		"print first C",
		"set LANGUAGE=zh_CN",
		"set LANGUAGE=C",
		"print second C",
		"set LANGUAGE=zh_CN",
	}
	if got := env.recordedEvents(); !slices.Equal(got, want) {
		t.Fatalf("events = %q, want %q", got, want)
	}
}

func TestPrintLocaleDoesNotPrintAfterTheWaitExpires(t *testing.T) {
	printLocaleMu.Lock()
	locked := true
	defer func() {
		if locked {
			printLocaleMu.Unlock()
		}
	}()

	env := newPrintLocaleEnvironment(map[string]string{"LANGUAGE": "zh_CN"})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	called := make(chan struct{}, 1)
	result := make(chan error, 1)
	go func() {
		_, err := withPrintLocale(
			ctx, "linux", env.lookup, env.setenv, env.unsetenv,
			func() ([]byte, error) {
				called <- struct{}{}
				return []byte("%PDF-"), nil
			},
		)
		result <- err
	}()

	<-ctx.Done()
	printLocaleMu.Unlock()
	locked = false

	select {
	case err := <-result:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("print error = %v, want context deadline exceeded", err)
		}
	case <-time.After(time.Second):
		t.Fatal("print did not return after its waiting context expired")
	}
	select {
	case <-called:
		t.Fatal("print started after its waiting context expired")
	default:
	}
	if got := env.recordedEvents(); len(got) != 0 {
		t.Fatalf("environment changed after the waiting context expired: %q", got)
	}
}

func TestPrintLocaleLeavesOtherOperatingSystemsAlone(t *testing.T) {
	env := newPrintLocaleEnvironment(map[string]string{"LANGUAGE": "zh_CN"})
	pdf, err := withPrintLocale(
		context.Background(),
		"darwin",
		env.lookup,
		env.setenv,
		env.unsetenv,
		func() ([]byte, error) { return []byte("%PDF-"), nil },
	)
	if err != nil || string(pdf) != "%PDF-" {
		t.Fatalf("print = %q, %v", pdf, err)
	}
	if got := env.recordedEvents(); len(got) != 0 {
		t.Fatalf("non-Linux print changed the environment: %q", got)
	}
}
