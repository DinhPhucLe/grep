package main

import (
	"bytes"
	"io"
	"reflect"
	"strings"
	"testing"
)

func TestArgumentsArePassedWithoutReparsing(t *testing.T) {
	args := []string{"--help", "a prompt with spaces", "$(literal)", "--", "\"quoted\""}
	parsed, err := parse(append([]string{"--codex", "--metrics", "status.json", "--"}, args...), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(parsed.args, args) {
		t.Fatalf("forwarded args: %#v", parsed.args)
	}
}

func TestInvalidModes(t *testing.T) {
	for _, args := range [][]string{
		{"resume"}, {"--demo", "--codex"}, {"--demo", "--", "hi"},
		{"--demo", "--metrics", "file"}, {"--render", "-", "--", "--help"},
		{"--metrics", "-"}, {"--unknown"},
	} {
		if _, err := parse(args, io.Discard); err == nil {
			t.Errorf("accepted %#v", args)
		}
	}
}

func TestRenderConsumesProducerValues(t *testing.T) {
	var output bytes.Buffer
	err := renderFile("-", strings.NewReader(`{"score":42,"riskLabel":"CUSTOM","trend":"falling","delta":-3}`), &output)
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"42", "CUSTOM", "v", "-3"} {
		if !strings.Contains(output.String(), text) {
			t.Errorf("missing %q in %q", text, output.String())
		}
	}
	if strings.Contains(output.String(), "\x1b") {
		t.Fatal("plain render contains ANSI escapes")
	}
}
