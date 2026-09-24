package zerolog

import "io"

type Level int

const (
    InfoLevel Level = iota
)

func SetGlobalLevel(l Level) {}

type ConsoleWriter struct {
    Out        io.Writer
    TimeFormat string
}

type Logger struct{}

func New(w io.Writer) Logger { return Logger{} }

func (l Logger) With() Logger { return l }
func (l Logger) Timestamp() Logger { return l }
func (l Logger) Logger() Logger { return l }

// Add minimal methods to satisfy usage
func (l Logger) Info() Logger { return l }
func (l Logger) Error() Logger { return l }
func (l Logger) Msg(msg string) {}
