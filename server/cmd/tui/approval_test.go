package main

import (
	"encoding/json"
	tea "github.com/charmbracelet/bubbletea"
	"strings"
	"testing"
)

func TestApprovalUsesPlainEnglishAndOptionalDetails(t *testing.T) {
	m := newModel(nil, "w", uiOptions{NoColor: true})
	m.Update(wireMessage{ID: json.RawMessage(`1`), Method: "item/commandExecution/requestApproval", Params: json.RawMessage(`{"command":"Get-Content -LiteralPath C:/project/SKILL.md","commandActions":[{"type":"read","path":"C:/project/SKILL.md","name":"SKILL.md"}],"reason":"Read the project instructions"}`)})
	view := m.requestView()
	for _, want := range []string{"Read a file", "SKILL.md", "Allow once", "Don't allow"} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q in %s", want, view)
		}
	}
	if strings.Contains(view, "requestApproval") || strings.Contains(view, "commandActions") {
		t.Fatal("raw protocol visible by default")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	if !strings.Contains(m.requestView(), "Technical details") {
		t.Fatal("cannot inspect exact request")
	}
}

func TestApprovalHonorsAvailableDecisions(t *testing.T) {
	out := &bufferCloser{}
	m := newModel(&appServer{stdin: out}, "w", uiOptions{})
	m.Update(wireMessage{ID: json.RawMessage(`"original"`), Method: "item/commandExecution/requestApproval", Params: json.RawMessage(`{"command":"Get-Content file.txt","availableDecisions":["accept",{"acceptWithExecpolicyAmendment":{"execpolicy_amendment":["Get-Content"]}},"cancel"]}`)})
	r := m.requests[0]
	if len(r.choices) != 3 {
		t.Fatalf("invented unsupported choices: %+v", r.choices)
	}
	if !strings.Contains(r.choices[0].label, "Stop") {
		t.Fatal("default should not grant permission")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	cmd()
	if !strings.Contains(out.String(), `"decision":"accept"`) || !strings.Contains(out.String(), `"id":"original"`) {
		t.Fatal(out.String())
	}
}

func TestUnknownCommandSummaryDoesNotClaimReadOnly(t *testing.T) {
	m := newModel(nil, "w", uiOptions{NoColor: true})
	m.Update(wireMessage{ID: json.RawMessage(`2`), Method: "item/commandExecution/requestApproval", Params: json.RawMessage(`{"command":"custom-tool --do-work"}`)})
	view := m.requestView()
	if !strings.Contains(view, "Run a command") || !strings.Contains(view, "custom-tool") {
		t.Fatal(view)
	}
	if strings.Contains(view, "Read a file") {
		t.Fatal("guessed action")
	}
}
