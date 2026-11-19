package main

import (
	"bytes"
	"strings"
	"testing"
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
