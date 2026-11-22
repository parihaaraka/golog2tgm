package zerolog

import (
	"fmt"
	"runtime"

	"github.com/rs/zerolog"
)

type TgmHook struct {
	PushMessage       func(level int8, hash uint64, msg string, pcs []uintptr)
	MinLevel          zerolog.Level
	CallStackMinLevel zerolog.Level
}

func Intro(level int8, count int) string {
	var mark, msg, fin string
	switch zerolog.Level(level) {
	case zerolog.TraceLevel:
		mark, msg = "🔬", "trace message"
	case zerolog.DebugLevel:
		mark, msg = "🔧", "debug message"
	case zerolog.InfoLevel:
		mark, msg = "🟢", "info message"
	case zerolog.WarnLevel:
		mark, msg = "🟡", "warning"
	case zerolog.ErrorLevel:
		mark, msg = "🔴", "error"
	case zerolog.FatalLevel:
		mark, msg = "💥", "critical message"
	default:
		msg = "message"
	}
	if count > 1 {
		fin = "s like"
	}
	return fmt.Sprintf("%s _*%d* %s%s:_\n", mark, count, msg, fin)
}

func (t TgmHook) Run(e *zerolog.Event, level zerolog.Level, message string) {
	if level >= t.MinLevel {
		// print call stack for Warn+ levels
		if level >= t.CallStackMinLevel {
			// limit number of stack items to be sent to tgm
			// * non-local project items will be skipped if srcRoot was specified
			pcs := make([]uintptr, 3)
			n := runtime.Callers(4, pcs[:]) // skip zerolog internals
			if n > 0 {
				// use program counter from the first frame if stack acquired
				f, _ := runtime.CallersFrames(pcs[0:1]).Next()
				if f.PC != 0 {
					// you may use pc as message hash in most cases
					// if the message's level is not dynamic
					hash := f.PC
					t.PushMessage(int8(level), uint64(hash), message, pcs[:n])
					return
				}
			}
		}
		t.PushMessage(int8(level), 0, message, nil)
	}
}
