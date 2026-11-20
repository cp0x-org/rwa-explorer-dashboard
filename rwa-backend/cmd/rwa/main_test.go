package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/cp0x-org/pi-rwa-interface/rwa-backend/internal/testdb"
)

func exec(args ...string) (int, string, string) {
	var o, e bytes.Buffer
	code := run(args, &o, &e)
	return code, o.String(), e.String()
}

func TestStubs(t *testing.T) {
	for _, c := range stubs {
		code, out, errOut := exec(c.name)
		if code != 2 || out != "" || errOut != "rwa "+c.name+": not implemented\n" {
			t.Errorf("%s: code=%d out=%q err=%q", c.name, code, out, errOut)
		}
	}
}

func TestHelp(t *testing.T) {
	for _, a := range []string{"--help", "-h", "help"} {
		code, out, errOut := exec(a)
		if code != 0 || errOut != "" {
			t.Errorf("%s: code=%d err=%q", a, code, errOut)
		}
		for _, c := range stubs {
			if !strings.Contains(out, c.name) {
				t.Errorf("%s: missing %s", a, c.name)
			}
		}
	}
}

func TestVersion(t *testing.T) {
	for _, a := range []string{"version", "--version"} {
		code, out, errOut := exec(a)
		if code != 0 || errOut != "" || strings.TrimSpace(out) == "" {
			t.Errorf("%s: code=%d out=%q err=%q", a, code, out, errOut)
		}
	}
}

func TestNoArgsAndUnknown(t *testing.T) {
	code, out, errOut := exec()
	if code != 2 || out != "" || !strings.Contains(errOut, "Usage") {
		t.Errorf("no args: %d %q %q", code, out, errOut)
	}
	code, out, errOut = exec("nope")
	if code != 2 || out != "" || !strings.Contains(errOut, "unknown command") {
		t.Errorf("unknown: %d %q %q", code, out, errOut)
	}
}

func TestMigrateUsage(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	for _, args := range [][]string{
		{"migrate"},
		{"migrate", "sideways"},
		{"migrate", "up", "extra"},
		{"migrate", "down-to"},
		{"migrate", "down-to", "abc"},
		{"migrate", "down-to", "-1"},
		{"migrate", "down-to", "1", "2"},
	} {
		code, out, errOut := exec(args...)
		if code != 2 || out != "" || errOut == "" {
			t.Errorf("%v: code=%d out=%q err=%q", args, code, out, errOut)
		}
	}
	code, _, errOut := exec("migrate", "up")
	if code != 2 || !strings.Contains(errOut, "DATABASE_URL is required") {
		t.Errorf("migrate up without DATABASE_URL: code=%d err=%q", code, errOut)
	}
}

func TestMigrateDownRequiresYes(t *testing.T) {
	u := testdb.Empty(t)
	t.Setenv("DATABASE_URL", u)
	if code, _, e := exec("migrate", "up"); code != 0 {
		t.Fatalf("up: %d %s", code, e)
	}
	if code, _, e := exec("migrate", "down-to", "0"); code != 2 || !strings.Contains(e, "--yes") {
		t.Fatalf("down-to without --yes: %d %q", code, e)
	}
	if code, out, e := exec("migrate", "status"); code != 0 || !strings.Contains(out, "applied") {
		t.Fatalf("status: %d %q %q", code, out, e)
	}
	if code, _, e := exec("migrate", "down-to", "0", "--yes"); code != 0 {
		t.Fatalf("down-to --yes: %d %s", code, e)
	}
	if code, out, _ := exec("migrate", "down-to", "0"); code != 0 || !strings.Contains(out, "nothing to roll back") {
		t.Fatalf("down-to at target: %d %q", code, out)
	}
}
