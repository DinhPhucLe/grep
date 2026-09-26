package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// viewSessionLog pretty-prints a --log JSONL file. Empty path → newest in tui/log/.
func viewSessionLog(path string) error {
	if path == "" || path == "latest" {
		var err error
		path, err = latestSessionLog()
		if err != nil {
			return err
		}
	}

	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	fmt.Fprintf(os.Stdout, "session log: %s\n\n", path)

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	n := 0
	for sc.Scan() {
		raw := bytes.TrimSpace(sc.Bytes())
		if len(raw) == 0 {
			continue
		}
		n++
		var entry logLine
		if err := json.Unmarshal(raw, &entry); err != nil {
			fmt.Fprintf(os.Stdout, "——— #%d (unparseable) ———\n%s\n\n", n, string(raw))
			continue
		}
		printLogEntry(n, entry)
	}
	if err := sc.Err(); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "(%d events)\n", n)
	return nil
}

func latestSessionLog() (string, error) {
	dir, err := resolveLogDir()
	if err != nil {
		return "", err
	}
	matches, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("no *.jsonl files in %s — run with --log first", dir)
	}
	var newest string
	var newestMod time.Time
	for _, p := range matches {
		st, err := os.Stat(p)
		if err != nil {
			continue
		}
		if newest == "" || st.ModTime().After(newestMod) {
			newest = p
			newestMod = st.ModTime()
		}
	}
	if newest == "" {
		return "", fmt.Errorf("no readable log files in %s", dir)
	}
	return newest, nil
}

func printLogEntry(n int, entry logLine) {
	arrow := "→"
	label := "OUT"
	if entry.Direction == "in" {
		arrow = "←"
		label = "IN "
	}

	ts := entry.TS
	if t, err := time.Parse(time.RFC3339Nano, entry.TS); err == nil {
		if loc, err := time.LoadLocation("America/New_York"); err == nil {
			ts = t.In(loc).Format("15:04:05.000 MST")
		} else {
			ts = t.Local().Format("15:04:05.000")
		}
	}

	summary := summarizeRPC(entry.Message)
	fmt.Fprintf(os.Stdout, "——— #%d %s %s %s ———\n", n, ts, label, arrow)
	fmt.Fprintf(os.Stdout, "%s\n", summary)

	var pretty bytes.Buffer
	if err := json.Indent(&pretty, entry.Message, "  ", "  "); err != nil {
		fmt.Fprintf(os.Stdout, "  %s\n\n", string(entry.Message))
		return
	}
	fmt.Fprintf(os.Stdout, "%s\n\n", pretty.String())
}

func summarizeRPC(raw json.RawMessage) string {
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil {
		return "(invalid message)"
	}

	parts := []string{}
	if id, ok := m["id"]; ok {
		parts = append(parts, "id="+trimRaw(id))
	}
	if method, ok := m["method"]; ok {
		parts = append(parts, "method="+trimRaw(method))
	}
	if _, ok := m["result"]; ok {
		parts = append(parts, "result")
	}
	if errObj, ok := m["error"]; ok {
		parts = append(parts, "error="+trimRaw(errObj))
	}
	if len(parts) == 0 {
		return "(message)"
	}
	return strings.Join(parts, "  ")
}

func trimRaw(r json.RawMessage) string {
	s := strings.TrimSpace(string(r))
	if len(s) > 0 && s[0] == '"' {
		var str string
		if json.Unmarshal(r, &str) == nil {
			return str
		}
	}
	if len(s) > 120 {
		return s[:117] + "..."
	}
	return s
}
