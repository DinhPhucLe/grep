package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

func (m *model) approvalSummary(msg wireMessage) string {
	if msg.Method == "item/fileChange/requestApproval" {
		var p map[string]json.RawMessage
		if json.Unmarshal(msg.Params, &p) == nil && len(p["changes"]) == 0 {
			var thread, turn, item string
			_ = json.Unmarshal(p["threadId"], &thread)
			_ = json.Unmarshal(p["turnId"], &turn)
			_ = json.Unmarshal(p["itemId"], &item)
			if i := m.byID[thread+"/"+turn+"/"+item]; i != nil && json.Valid([]byte(i.raw)) {
				p["changes"] = json.RawMessage(i.raw)
				msg.Params, _ = json.Marshal(p)
			}
		}
	}
	return approvalSummary(msg)
}

// Summaries describe server-provided actions, never infer safety from shell text.
func approvalSummary(msg wireMessage) string {
	var p struct {
		Command                                           json.RawMessage
		Cwd, Reason, GrantRoot, Kind, Message, ServerName string
		CommandActions                                    []struct{ Type, Path, Name, Query string }
		Changes                                           json.RawMessage
		Permissions                                       json.RawMessage
	}
	_ = json.Unmarshal(msg.Params, &p)
	lines := []string{}
	switch msg.Method {
	case "item/commandExecution/requestApproval", "execCommandApproval":
		lines = append(lines, "Codex needs your permission to run a command.")
		if p.Kind == "writeStdin" {
			lines[0] = "Codex wants to send input to a running program."
		}
		if len(p.CommandActions) > 0 {
			for _, a := range p.CommandActions {
				switch a.Type {
				case "read":
					lines = append(lines, "Read a file: "+a.Path)
				case "listFiles":
					lines = append(lines, "List files in: "+a.Path)
				case "search":
					lines = append(lines, "Search for: "+a.Query)
					if a.Path != "" {
						lines = append(lines, "In: "+a.Path)
					}
				default:
					lines = append(lines, "Run a command: "+commandText(p.Command))
				}
			}
		} else {
			lines = append(lines, "Run a command: "+commandText(p.Command))
		}
	case "item/fileChange/requestApproval", "applyPatchApproval":
		lines = append(lines, "Codex wants permission to change files.")
		var changes []struct {
			Path string
			Kind struct{ Type string }
		}
		if json.Unmarshal(p.Changes, &changes) == nil {
			for _, c := range changes {
				verb := "Change"
				switch c.Kind.Type {
				case "add":
					verb = "Create"
				case "delete":
					verb = "Delete"
				}
				lines = append(lines, verb+": "+c.Path)
			}
		}
		if p.GrantRoot != "" {
			lines = append(lines, "Allow changes under: "+p.GrantRoot)
		}
	case "item/permissions/requestApproval":
		lines = append(lines, "Codex wants additional access.")
		var permissions struct {
			Network    json.RawMessage
			FileSystem json.RawMessage
		}
		_ = json.Unmarshal(p.Permissions, &permissions)
		if len(permissions.Network) > 0 && string(permissions.Network) != "null" {
			var net struct{ Enabled *bool }
			_ = json.Unmarshal(permissions.Network, &net)
			if net.Enabled != nil && *net.Enabled {
				lines = append(lines, "Connect to the network.")
			} else {
				lines = append(lines, "Change network permissions.")
			}
		}
		if len(permissions.FileSystem) > 0 && string(permissions.FileSystem) != "null" {
			var fs struct {
				Read, Write []string
				Entries     []json.RawMessage
			}
			_ = json.Unmarshal(permissions.FileSystem, &fs)
			for _, path := range fs.Read {
				lines = append(lines, "Read files under: "+path)
			}
			for _, path := range fs.Write {
				lines = append(lines, "Change files under: "+path)
			}
			if len(fs.Entries) > 0 {
				lines = append(lines, "Change access to specific files or folders (see technical details).")
			}
		}
		lines = append(lines, "Open technical details to review the exact access requested.")
	case "mcpServer/elicitation/request":
		lines = append(lines, "A connected service needs your input.")
		if p.ServerName != "" {
			lines = append(lines, "Service: "+p.ServerName)
		}
		if p.Message != "" {
			lines = append(lines, p.Message)
		}
	}
	if p.Reason != "" {
		lines = append(lines, "Reason given: "+p.Reason)
	}
	if p.Cwd != "" {
		lines = append(lines, "Working folder: "+p.Cwd)
	}
	return strings.Join(lines, "\n")
}
func commandText(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var args []string
	if json.Unmarshal(raw, &args) == nil {
		return strings.Join(args, " ")
	}
	return string(raw)
}
func decisionLabel(value json.RawMessage) string {
	var s string
	if json.Unmarshal(value, &s) == nil {
		switch s {
		case "accept":
			return "Allow once"
		case "decline":
			return "Don't allow"
		case "cancel":
			return "Stop this task"
		case "acceptForSession":
			return "Allow for this conversation"
		}
		return "Server choice: " + s
	}
	var v map[string]json.RawMessage
	_ = json.Unmarshal(value, &v)
	if a, ok := v["acceptWithExecpolicyAmendment"]; ok {
		var p struct {
			Prefix []string `json:"execpolicy_amendment"`
		}
		_ = json.Unmarshal(a, &p)
		return "Allow and save a rule for: " + strings.Join(p.Prefix, " ")
	}
	if a, ok := v["applyNetworkPolicyAmendment"]; ok {
		var p struct {
			Rule struct{ Host, Action string } `json:"network_policy_amendment"`
		}
		_ = json.Unmarshal(a, &p)
		return fmt.Sprintf("Save network rule: %s %s", p.Rule.Action, p.Rule.Host)
	}
	return "Server permission option (see technical details)"
}
