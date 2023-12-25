package main

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/parihaaraka/golog2tgm"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var testMessages = []string{
	"Бабушка, торгующая семечками, обеспечивала себе приток клиентов, подсыпая" +
		" в семечки героин. Её спалили на том, что голуби рядышком обсуждали" +
		" квантовую теорию поля и азиатский фондовый рынок.",
	`Письмо на Балабановскую спичечную фабрику: "Я 11 лет считаю спички у вас в` +
		` коробках — их то 59, то 60, а иногда и 58. Вы там е$анутые все что ли?"`,
	"Праздновать будем на природе - другого помещения у нас нет.",
	"Иногда люблю побаловать себя одиночеством. И людям хорошо, и мне приятно.",
	"У Запашных было много братьев. Остались самые невкусные.",
	"— Какая милая девочка! Сколько тебе?\n— Грамм.",
	"— Что ты попросишь у Деда Мороза в новом году?\n— Пощады!",
	"non-utf8 characters: \xff\xff",
}

type TgmHook struct {
	w *golog2tgm.Worker
}

func (t TgmHook) Run(e *zerolog.Event, level zerolog.Level, message string) {
	if level > zerolog.DebugLevel {
		// print call stack for Warn+ levels
		if level != zerolog.InfoLevel {
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
					t.w.PushMessage(int8(level), uint64(hash), message, pcs[:n])
					return
				}
			}
		}
		t.w.PushMessage(int8(level), 0, message, nil)
	}
}

func main() {
	apiToken := os.Getenv("TGM_API_TOKEN")
	chatId, _ := strconv.ParseInt(os.Getenv("TGM_CHAT_ID"), 10, 64)
	if len(apiToken) == 0 || chatId == 0 {
		fmt.Println("set TGM_API_TOKEN and TGM_CHAT_ID environment varibales to make it work")
		return
	}

	thisFilePath := "/test.go"
	srcRoot := ""
	_, file, _, ok := runtime.Caller(0)
	if ok && strings.HasSuffix(file, thisFilePath) {
		srcRoot = file[:len(file)-len(thisFilePath)+1]
	}

	ctx := context.Background()
	w := golog2tgm.Start(ctx,
		srcRoot,
		func(level int8, count int) string {
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
		},
		func(err string) {
			log.Ctx(ctx).Error().Msg(err)
		},
	)
	w.SetCaption("test daemon")
	w.SetTargetChat(apiToken, chatId)
	w.SetPeriod(2)
	w.SetTimeZone("MSK", 3*3600)
	defer func() {
		w.Stop()
	}()

	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	output := zerolog.ConsoleWriter{Out: os.Stdout, NoColor: false, TimeFormat: "15:04:05"}
	log.Logger = zerolog.New(output).With().Timestamp().Caller().Logger().Hook(TgmHook{w})
	ctx = log.Logger.WithContext(ctx)

	log.Ctx(ctx).Info().Msg("start")
	w.Uncork()
	go func() {
		for range time.Tick(50 * time.Millisecond) {
			i := rand.Intn(len(testMessages))
			LogMessage(ctx, zerolog.Level(i%4+1), testMessages[i])
		}
	}()

	timer := time.NewTimer(3 * time.Second)
	<-timer.C
	log.Ctx(ctx).Error().Msg("stop")
}
