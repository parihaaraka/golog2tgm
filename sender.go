package golog2tgm

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

type chat struct {
	apiToken string
	id       int64
}

type sender struct {
	intro        func(int8, int) string
	errorHandler func(err string)
	caption      string
	srcRoot      string
	loc          *time.Location
	chat
}

func putFixed(out *strings.Builder, value string) {
	for i, c := range value {
		if c == 0xfffd || !utf8.ValidRune(c) {
			_, s := utf8.DecodeRuneInString(value[i:])
			out.WriteString("\\\\x")
			out.WriteString(hex.EncodeToString([]byte(value[i : i+s])))
			continue
		}
		out.WriteRune(c)
	}
}

func putEscaped(out *strings.Builder, value string) {
	for i, c := range value {
		if c == 0xfffd || !utf8.ValidRune(c) {
			_, s := utf8.DecodeRuneInString(value[i:])
			out.WriteString("\\\\x")
			out.WriteString(hex.EncodeToString([]byte(value[i : i+s])))
			continue
		}

		if strings.ContainsRune("\\_*[]()~`>#+-=|{}.!", c) {
			out.WriteRune('\\')
		}
		out.WriteRune(c)
	}
}

func (s *sender) prepareMessages(b *batch) []string {
	if len(b.chronoIndex) == 0 {
		return []string{}
	}

	maxSize := 4096 // telegram max message size
	out := &strings.Builder{}
	if len(s.caption) > 0 {
		out.WriteRune('*') // begin bold
		putEscaped(out, s.caption)
		out.WriteRune('*') // end bold
		out.WriteRune('\n')
	}

	if b.tsStart != b.tsFinish {
		out.WriteString(fmt.Sprintf(
			"%s \\- %s %s\n",
			b.tsStart.In(s.loc).Format("15:04:05"),
			b.tsFinish.In(s.loc).Format("15:04:05"),
			s.loc.String(),
		))
	}

	format_msg := func(smp *msgSample) *strings.Builder {
		buf := &strings.Builder{}
		buf.Grow(maxSize)
		if s.intro == nil {
			buf.WriteString(fmt.Sprintf("_*%d* messages like:_\n", smp.count))
		} else {
			buf.WriteString(s.intro(smp.level, smp.count))
		}

		if smp.tsFirst == smp.tsLast {
			buf.WriteString(fmt.Sprintf("time: `%s`\n", smp.tsFirst.In(s.loc).Format("15:04:05.000")))
		} else {
			delta := smp.tsLast.Sub(smp.tsFirst)
			buf.WriteString(fmt.Sprintf("first: `%s`\nlast: `+%.3f sec`\n",
				smp.tsFirst.In(s.loc).Format("15:04:05.000"), delta.Seconds()))
		}

		// Openinig ``` needs trailing \n to save first word (place for highlighter name).
		// Space here prints as is.
		// \n before closing ``` prints as is, so there is a space (just to be on the safe side).
		if strings.ContainsRune(smp.message, '\n') {
			buf.WriteString("message:\n```\n")
			putFixed(buf, smp.message)
			buf.WriteString(" ```")
		} else {
			buf.WriteString("message: `")
			putEscaped(buf, smp.message)
			buf.WriteString("`")
		}

		if smp.pcs != nil && len(smp.pcs) > 0 {
			callers := []string{}
			addCallerIfLocal := func(f *runtime.Frame) {
				if strings.HasPrefix(f.File, s.srcRoot) {
					callers = append(callers, f.File[len(s.srcRoot):]+":"+strconv.Itoa(f.Line))
				}
			}
			frames := runtime.CallersFrames(smp.pcs)
			f, hasNext := frames.Next()
			if f.PC != 0 {
				addCallerIfLocal(&f)
				for hasNext {
					f, hasNext = frames.Next()
					addCallerIfLocal(&f)
				}

				if len(callers) == 1 {
					buf.WriteString("\ncaller: `")
					buf.WriteString(callers[0])
					buf.WriteString("`")
				} else {
					buf.WriteString("\ncallers:\n```")
					for _, c := range callers {
						buf.WriteRune('\n')
						buf.WriteString(c)
					}
					buf.WriteString("\n```")
				}
			}
		}

		return buf
	}

	// compose messages
	res := []string{}
	for _, hash := range b.chronoIndex {
		sample := b.samples[hash]
		tmp := format_msg(sample)
		if out.Len()+tmp.Len() <= maxSize {
			// separate messages
			{
				tmp := out.String()
				if !strings.HasSuffix(tmp, "\n") && !strings.HasSuffix(tmp, "```") {
					out.WriteRune('\n')
				}
			}
			out.WriteRune('\n')
			out.WriteString(tmp.String())
		} else {
			res = append(res, out.String())
			out.Reset()
			out = tmp
		}
	}
	if out.Len() > 0 {
		res = append(res, out.String())
	}
	return res
}

func (s *sender) send(ctx context.Context, messages []string) {
	if s.chat.id == 0 {
		return
	}

	var wg sync.WaitGroup
	client := &http.Client{Timeout: 10 * time.Second}
	u := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", s.chat.apiToken)

	for _, msg := range messages {
		data := url.Values{
			"parse_mode": {"MarkdownV2"},
			"chat_id":    {strconv.FormatInt(s.chat.id, 10)},
			"text":       {msg},
		}
		req, err := http.NewRequestWithContext(ctx, "POST", u, strings.NewReader(data.Encode()))
		if err != nil {
			if s.errorHandler != nil {
				s.errorHandler(err.Error())
			}
			break
		}
		req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
		wg.Add(1)

		go func() {
			defer wg.Done()
			resp, err := client.Do(req)
			if err != nil {
				return
			}
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return
			}
			if !strings.Contains(string(body), `"ok":true`) {
				if s.errorHandler != nil {
					s.errorHandler(fmt.Sprintf("tgm response: %s\nrequest data: %s", string(body), data.Encode()))
				}
			}
		}()
	}

	wg.Wait()
}
