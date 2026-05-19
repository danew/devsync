package logging

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

type Logger struct {
	debug bool
	trace bool
	out   io.Writer
}

func New(debug bool, trace bool, out io.Writer) Logger {
	if os.Getenv("DEVSYNC_DEBUG") != "" {
		debug = true
	}
	if os.Getenv("DEVSYNC_TRACE") != "" {
		debug = true
		trace = true
	}
	if out == nil {
		out = os.Stderr
	}
	return Logger{debug: debug, trace: trace, out: out}
}

func (l Logger) Debug(event string, fields map[string]string) {
	if !l.debug {
		return
	}
	l.write("debug", event, fields)
}

func (l Logger) Trace(event string, fields map[string]string) {
	if !l.trace {
		return
	}
	l.write("trace", event, fields)
}

func (l Logger) write(level string, event string, fields map[string]string) {
	parts := []string{fmt.Sprintf("ts=%s", time.Now().UTC().Format(time.RFC3339)), "level=" + level, "event=" + quote(event)}
	for key, value := range fields {
		parts = append(parts, key+"="+quote(value))
	}
	fmt.Fprintln(l.out, strings.Join(parts, " "))
}

func quote(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return `"` + value + `"`
}
