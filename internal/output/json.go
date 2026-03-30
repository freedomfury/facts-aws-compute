package output

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// JSON marshals v as indented JSON and writes it to stdout.
func JSON(v interface{}) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

// Fatal writes a JSON error object to stderr and exits with code 1.
func Fatal(msg string) {
	fmt.Fprintf(os.Stderr, `{"error": %s}`+"\n", jsonString(msg))
	os.Exit(1)
}

// Fatalf is like Fatal but with formatting.
func Fatalf(format string, args ...interface{}) {
	Fatal(fmt.Sprintf(format, args...))
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

var (
	verboseMu sync.RWMutex
	verbose   bool
)

// SetVerbose enables or disables debug output to stderr.
func SetVerbose(v bool) {
	verboseMu.Lock()
	verbose = v
	verboseMu.Unlock()
}

// Debugf prints a debug message to stderr when verbose is enabled.
func Debugf(format string, args ...interface{}) {
	verboseMu.RLock()
	en := verbose
	verboseMu.RUnlock()
	if !en {
		return
	}
	fmt.Fprintf(os.Stderr, "[DEBUG] "+format+"\n", args...)
}
