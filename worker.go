package golog2tgm

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"
)

type cfgCode int

const (
	cfgCaption cfgCode = iota
	cfgChat
	cfgPeriod
	cfgTimeZone
	uncork
	stop
)

type logMessage struct {
	level   int8
	hash    uint64
	message string
	pcs     []uintptr
}

type cfgCommand struct {
	code  cfgCode
	value any
}

type timeZone struct {
	name   string
	offset int
}

type Worker struct {
	m       merger
	cMsg    chan logMessage
	cCfg    chan cfgCommand
	closedC chan struct{}
	stopped bool
	mu      sync.Mutex // mutex to sync Uncork and Stop
}

func (w *Worker) sendCfgCommand(cmd *cfgCommand) {
	select {
	case w.cCfg <- *cmd:
	case <-w.closedC:
	}
}

// Push message to the engine.
// Pass U+202f as the first character to skip the message.
//
// hash allows to identify a sample externally, e.g. via program counter.
// Pass 0 to match samples by golog2tgm.
//
// pcs allows to add call stack to the sample.
func (w *Worker) PushMessage(level int8, hash uint64, msg string, pcs []uintptr) {
	if !strings.HasPrefix(msg, " ") {
		select {
		case w.cMsg <- logMessage{level, hash, msg, pcs}:
		case <-w.closedC:
		}
	}
}

// Set teleram message caption (e.g. your daemon name).
func (w *Worker) SetCaption(caption string) {
	w.sendCfgCommand(&cfgCommand{cfgCaption, caption})
}

func (w *Worker) SetTargetChat(apiToken string, id int64) {
	w.sendCfgCommand(&cfgCommand{cfgChat, chat{apiToken: apiToken, id: id}})
}

// Set convenient time zone and its UTC offset in seconds.
func (w *Worker) SetTimeZone(name string, offsetSec int) {
	w.sendCfgCommand(&cfgCommand{cfgTimeZone, timeZone{name, offsetSec}})
}

// Set alerting interval in seconds. Uncorks.
func (w *Worker) SetPeriod(periodSec uint) {
	if periodSec == 0 {
		return
	}
	w.sendCfgCommand(&cfgCommand{cfgPeriod, periodSec})
}

// Force send aggregated alerts.
func (w *Worker) Uncork(notify bool) chan struct{} {
	var doneC chan struct{}
	if notify {
		doneC = make(chan struct{})
	}

	w.mu.Lock()
	if w.stopped {
		w.mu.Unlock()
		if doneC != nil {
			close(doneC)
		}
		return doneC
	}
	w.cCfg <- cfgCommand{uncork, doneC}
	w.mu.Unlock()
	return doneC
}

// Stop the worker only if you really need to.
// Typically, a worker lives for the entire lifetime of the application
// and doesn't need to be stopped.
// Call Uncork() before Stop() to send buffered messages.
func (w *Worker) Stop() {
	w.mu.Lock()
	if w.stopped {
		w.mu.Unlock()
		<-w.closedC
		return
	}
	w.stopped = true
	w.mu.Unlock()

	w.cCfg <- cfgCommand{stop, nil}
	<-w.closedC
}

// Start the worker. It doesn't use context to avoid shutdown race.
// Just call <-Uncork(true) as the most final action to send all the messages
// the worker captured.
//
// srcRoot is a common part of the sources path being printed in a call stack (to shorten the output).
// Pass empty string to output the paths as is.
//
// intro arg is a callback to print an intro with prefered design.
//
// errorHandler allows to log golog2tgm internal errors with a logger being used by a caller.
func Start(
	srcRoot string,
	intro func(int8, int) string,
	errorHandler func(string),
	client *http.Client,
) *Worker {

	w := Worker{
		// small cMsg buffer leads to a huge park/wake overhead during a message surge
		cMsg:    make(chan logMessage, 1024),
		cCfg:    make(chan cfgCommand, 5),
		closedC: make(chan struct{}),
	}

	go func() {

		// Don't close w.cMsg and w.cCfg to avoid shutdown race.

		var caption string
		var dest chat
		loc := time.FixedZone("UTC", 0)

		send := func() func(context.Context, chan struct{}) {
			// separate settings pack to avoid access synchronization
			s := sender{
				intro:        intro,
				errorHandler: errorHandler,
				srcRoot:      srcRoot,
				caption:      caption,
				chat:         dest,
				loc:          loc,
				client:       client,
			}
			if s.client == nil {
				s.client = &http.Client{Timeout: 10 * time.Second}
			}

			messages := s.prepareMessages(w.m.FinalizedBatch())
			return func(ctx context.Context, c chan struct{}) {
				s.send(ctx, messages, c)
			}
		}

		sendAndClearBatch := func(c chan struct{}) {
			go send()(context.Background(), c)
			// ignore unsuccessful sendings
			w.m.finalizedBatch.clear()
		}

		isStopped := func() bool {
			w.mu.Lock()
			defer w.mu.Unlock()
			return w.stopped
		}

		period := 5 * time.Minute
		sendTicker := time.NewTicker(period)
		for {
			select {
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
					if !isStopped() {
						sendAndClearBatch(nil)
						sendTicker.Reset(period)
					}
				case uncork:
					c := cfg.value.(chan struct{})
					sendAndClearBatch(c)
					if !isStopped() {
						sendTicker.Reset(period)
					}
				case stop:
					sendTicker.Stop()
					close(w.closedC)
					return
				case cfgTimeZone:
					if c, ok := cfg.value.(timeZone); ok {
						loc = time.FixedZone(c.name, c.offset)
					}
				}
			case <-sendTicker.C:
				sendAndClearBatch(nil)
			}
		}
	}()
	return &w
}
