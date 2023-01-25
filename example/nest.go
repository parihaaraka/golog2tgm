package main

import (
	"context"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// just to increase call stack and expose different program counters
func LogMessage(ctx context.Context, level zerolog.Level, message string) {
	switch level {
	case zerolog.TraceLevel:
		log.Ctx(ctx).WithLevel(level).Msg(message)
	case zerolog.DebugLevel:
		log.Ctx(ctx).WithLevel(level).Msg(message)
	case zerolog.InfoLevel:
		log.Ctx(ctx).WithLevel(level).Msg(message)
	case zerolog.WarnLevel:
		log.Ctx(ctx).WithLevel(level).Msg(message)
	case zerolog.ErrorLevel:
		log.Ctx(ctx).WithLevel(level).Msg(message)
	case zerolog.FatalLevel:
		log.Ctx(ctx).WithLevel(level).Msg(message)
	default:
		log.Ctx(ctx).WithLevel(level).Msg(message)
	}
}
