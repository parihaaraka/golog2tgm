package golog2tgm

import (
	"context"
	"time"
)

type cfgCode int

const (
	cfgCaption cfgCode = iota
	cfgChat
	cfgPeriod
	cfgTimeZone
	uncork
)

type logMessage struct {
	level   int8
	hash    uint64
	message string
	pcs     []uintptr
}

type cfgCommand struct {
	code  cfgCode
	value interface{}
}

type Worker struct {
	m      merger
	cancel context.CancelFunc
	cMsg   chan logMessage
	cCfg   chan cfgCommand
	doneC  chan bool
}

// Push message to the engine.
//
// hash allows to identify a sample externally, e.g. via program counter.
// Pass 0 to match samples by golog2tgm.
//
// pcs allows to add call stack to the sample.
func (w *Worker) PushMessage(level int8, hash uint64, msg string, pcs []uintptr) {
	w.cMsg <- logMessage{level, hash, msg, pcs}
}

// Set teleram message caption (e.g. your daemon name)
func (w *Worker) SetCaption(caption string) {
	w.cCfg <- cfgCommand{cfgCaption, caption}
}

func (w *Worker) SetTargetChat(apiToken string, id int64) {
	w.cCfg <- cfgCommand{cfgChat, chat{apiToken: apiToken, id: id}}
}

// Set convenient time zone and its UTC offset in seconds
func (w *Worker) SetTimeZone(name string, offsetSec int) {
	w.cCfg <- cfgCommand{cfgTimeZone, struct {
		name   string
		offset int
	}{name, offsetSec}}
}

// Set alerting interval in seconds
func (w *Worker) SetPeriod(periodSec uint) {
	w.cCfg <- cfgCommand{cfgPeriod, periodSec}
}

// Force send aggregated alerts
func (w *Worker) Uncork() {
	w.cCfg <- cfgCommand{uncork, nil}
}

// Start worker.
//
// srcRoot is a common part of the sources path being printed in a call stack (to exclude external calls and to truncate long paths).
//
// intro arg is a callback to print an intro with prefered design.
//
// errorHandler allows to log golog2tgm internal errors with a logger being used by a caller.
func Start(
	ctx context.Context,
	srcRoot string,
	intro func(int8, int) string,
	errorHandler func(string),
) *Worker {

	w := Worker{
		cMsg:  make(chan logMessage, 20),
		cCfg:  make(chan cfgCommand, 10),
		doneC: make(chan bool, 1),
	}
	ctx, w.cancel = context.WithCancel(context.Background())

	go func() {
		var caption string
		var dest chat
		loc := time.FixedZone("UTC", 0)

		send := func() func(context.Context) {
			// separate settings pack to avoid access synchronization
			s := sender{
				intro:        intro,
				errorHandler: errorHandler,
				srcRoot:      srcRoot,
				caption:      caption,
				chat:         dest,
				loc:          loc,
			}
			messages := s.prepareMessages(w.m.FinalizedBatch())
			return func(ctx context.Context) {
				s.send(ctx, messages)
			}
		}

		sendAndClearBatch := func() {
			// ignore cancellation via ctx to finalyze sending when the caller is stopping
			go send()(context.Background())
			// ignore unsuccessful sendings
			w.m.finalizedBatch.clear()
		}

		period := 5 * time.Minute
		sendTicker := time.NewTicker(period)
		for {
			select {
			case <-ctx.Done():
				sendTicker.Stop()
				send()(context.Background())
				w.doneC <- true
				return
			case msg := <-w.cMsg:
				w.m.PushMsg(&msg)
			case cfg := <-w.cCfg:
				switch cfg.code {
				case cfgCaption:
					caption = cfg.value.(string)
				case cfgChat:
					dest = cfg.value.(chat)
				case cfgPeriod:
					period = time.Duration(cfg.value.(uint)) * time.Second
					fallthrough // uncork on period change
				case uncork:
					sendAndClearBatch()
					sendTicker.Reset(period)
				case cfgTimeZone:
					if c, ok := cfg.value.(struct {
						name   string
						offset int
					}); ok {
						loc = time.FixedZone(c.name, c.offset)
					}
				}
			case <-sendTicker.C:
				sendAndClearBatch()
			}
		}
	}()
	return &w
}

// Synchronously send messages that have not yet been sent and stop the worker.
func (w *Worker) Stop() {
	w.cancel()
	<-w.doneC
}
