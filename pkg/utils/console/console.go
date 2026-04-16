package console

import (
	"io"
	"os"
	"runtime"
	"sync"

	"github.com/mattn/go-isatty"
	"golang.org/x/sys/windows"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/transform"
)

var (
	stdout io.Writer
	stderr io.Writer
	once   sync.Once
)

func init() {
	once.Do(func() {
		stdout = WrapWriter(os.Stdout)
		stderr = WrapWriter(os.Stderr)
	})
}

// Stdout returns a wrapped os.Stdout for correct encoding
func Stdout() io.Writer {
	return stdout
}

// Stderr returns a wrapped os.Stderr for correct encoding
func Stderr() io.Writer {
	return stderr
}

// Global setup for Windows console encoding.
// If output is a terminal or on Windows without redirection, 
// it returns a writer that converts UTF-8 to the appropriate encoding.
func WrapWriter(w *os.File) io.Writer {
	if runtime.GOOS != "windows" {
		return w
	}

	// Always respect explicit override
	if env := os.Getenv("PHOTO_AI_ENCODING"); env != "" {
		if env == "utf8" || env == "utf-8" {
			return w
		}
		if env == "cp932" || env == "sjis" {
			return transform.NewWriter(w, japanese.ShiftJIS.NewEncoder())
		}
	}

	// Detect console output code page.
	// 932 is the standard code page for Japanese Windows (Shift-S/CP932).
	cp, _ := windows.GetConsoleOutputCP()
	if cp == 932 {
		return transform.NewWriter(w, japanese.ShiftJIS.NewEncoder())
	}

	// On Windows, if it's a terminal, use CP932
	if isatty.IsTerminal(w.Fd()) || isatty.IsCygwinTerminal(w.Fd()) {
		return transform.NewWriter(w, japanese.ShiftJIS.NewEncoder())
	}

	return w
}
