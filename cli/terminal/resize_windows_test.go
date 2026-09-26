//go:build windows

package terminal

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf16"
	"unsafe"

	"github.com/charmbracelet/x/term"
	"github.com/charmbracelet/x/xpty"
	"golang.org/x/sys/windows"
)

type screenCapture struct {
	Rows []string
	Want []string
}

// Read the actual console cells, rather than checking whether output once
// contained a label. This catches old glyphs surviving outside the new header.
func readConsoleRows() ([]string, error) {
	h := windows.Handle(os.Stdout.Fd())
	var info windows.ConsoleScreenBufferInfo
	if err := windows.GetConsoleScreenBufferInfo(h, &info); err != nil {
		return nil, err
	}
	read := windows.NewLazySystemDLL("kernel32.dll").NewProc("ReadConsoleOutputCharacterW")
	width := int(info.Window.Right - info.Window.Left + 1)
	var rows []string
	for y := info.Window.Top; y <= info.Window.Bottom; y++ {
		buf := make([]uint16, width)
		var n uint32
		coord := uint32(uint16(info.Window.Left)) | uint32(uint16(y))<<16
		ok, _, err := read.Call(uintptr(h), uintptr(unsafe.Pointer(&buf[0])), uintptr(width), uintptr(coord), uintptr(unsafe.Pointer(&n)))
		if ok == 0 {
			return nil, err
		}
		rows = append(rows, string(utf16.Decode(buf[:n])))
	}
	return rows, nil
}

func TestResizeConsoleHelper(t *testing.T) {
	dir := os.Getenv("CORTISOL_SCREEN_TEST")
	if dir == "" {
		return
	}
	restore, err := EnableOutput(os.Stdout)
	if err != nil {
		t.Fatal(err)
	}
	defer restore()
	raw, err := term.MakeRaw(os.Stdin.Fd())
	if err != nil {
		t.Fatal(err)
	}
	defer term.Restore(os.Stdin.Fd(), raw)
	fmt.Fprint(os.Stdout, CSI+"2J"+CSI+"H"+"original shell screen")
	r := NewRenderer(os.Stdout, 80, 24, true)
	r.color = ColorTrue
	if err := r.Start(); err != nil {
		t.Fatal(err)
	}
	defer r.Stop()
	r.Write([]byte("child marker\r\n"))
	s := snapshot(58)
	r.Update(s)
	scanner := bufio.NewScanner(os.Stdin)
	for step := 0; scanner.Scan(); step++ {
		var cols, rows int
		if _, err := fmt.Sscanf(scanner.Text(), "%d %d", &cols, &rows); err != nil {
			t.Fatal(err)
		}
		if cols == 0 {
			if err := r.Stop(); err != nil {
				t.Fatal(err)
			}
			restored, err := readConsoleRows()
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(strings.Join(restored, "\n"), "original shell screen") {
				t.Fatalf("exit lost original shell screen: %q", restored)
			}
			return
		}
		deadline := time.Now().Add(3 * time.Second)
		for {
			w, h, e := term.GetSize(os.Stdout.Fd())
			if e == nil && w == cols && h == rows {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("console never reached %dx%d: %dx%d %v", cols, rows, w, h, e)
			}
			time.Sleep(10 * time.Millisecond)
		}
		if err := r.Resize(cols, rows); err != nil {
			t.Fatal(err)
		}
		// The child may use its own alternate screen. It must stay inside the
		// wrapper's owned screen, and returning must restore its normal cells.
		if step == 2 {
			r.Write([]byte(CSI + "?1049h" + CSI + "H" + "child marker alternate"))
		}
		if step == 3 {
			r.Write([]byte(CSI + "?1049l"))
		}
		r.Update(s)
		actual, err := readConsoleRows()
		if err != nil {
			t.Fatal(err)
		}
		var want []string
		if r.reservedRows() != 0 {
			want = renderMeter(PrepareMeter(s), r.animation.displayedScore, r.layout().columns, ColorTrue, false, r.postureFrame, true)
			for i := range want {
				want[i] = plainFrame(want[i])
			}
		}
		data, _ := json.Marshal(screenCapture{Rows: actual, Want: want})
		if err := os.WriteFile(filepath.Join(dir, strconv.Itoa(step)+".json"), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRealConsoleCellsAfterRepeatedResize(t *testing.T) {
	p, err := xpty.NewConPty(80, 24)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	dir := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestResizeConsoleHelper$")
	cmd.Env = append(os.Environ(), "CORTISOL_SCREEN_TEST="+dir, "VIBECODE_ASCII=0")
	if err := p.Start(cmd); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()
	go io.Copy(io.Discard, p)
	for step, size := range [][2]int{{80, 24}, {40, 24}, {80, 24}, {77, 24}, {54, 24}, {20, 4}, {100, 30}, {80, 24}} {
		if err := p.Resize(size[0], size[1]); err != nil {
			t.Fatal(err)
		}
		if _, err := fmt.Fprintf(p, "%d %d\n", size[0], size[1]); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, strconv.Itoa(step)+".json")
		deadline := time.Now().Add(5 * time.Second)
		var capture screenCapture
		for {
			data, e := os.ReadFile(path)
			if e == nil && json.Unmarshal(data, &capture) == nil {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("capture timed out at", size)
			}
			time.Sleep(10 * time.Millisecond)
		}
		for y, row := range capture.Rows {
			if y < len(capture.Want) {
				if strings.TrimRight(row, " ") != strings.TrimRight(capture.Want[y], " ") {
					t.Errorf("size %v row %d\ngot  %q\nwant %q", size, y, row, capture.Want[y])
				}
			} else if strings.ContainsAny(row, "▀▄█●") {
				t.Errorf("size %v: stale status glyphs in child row %d: %q", size, y, row)
			}
		}
		if !strings.Contains(strings.Join(capture.Rows[len(capture.Want):], "\n"), "child marker") {
			t.Errorf("size %v lost child output", size)
		}
	}
	fmt.Fprint(p, "0 0\n")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := xpty.WaitProcess(ctx, cmd); err != nil {
		t.Fatal(err)
	}
}
