package main

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"time"

	. "github.com/parihaaraka/golog2tgm/zerolog"

	"github.com/parihaaraka/golog2tgm"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var testMessages = []string{
	"Никто не вправе вмешиваться в деятельность руководителей ликвидации " +
		"чрезвычайных ситуаций по руководству работами по ликвидации " +
		"чрезвычайных ситуаций, иначе как отстранив их в установленном " +
		"порядке от исполнения обязанностей и приняв руководство на себя " +
		"или назначив другое должностное лицо.",
	"Да, человек смертен, но это было бы еще полбеды.\n" +
		"Плохо то, что он иногда внезапно смертен, вот в чем фокус!",
	"Праздновать будем на природе - другого помещения у нас нет.",
	"Иногда люблю побаловать себя одиночеством. И людям хорошо, и мне приятно.",
	"Когда мы вспоминаем, что все мы безумны, тайны исчезают, и жизнь объясняется.",
	"special caracters wrapped by ```:\n\\`",
	"non-utf8 characters: \xff\xff",
	" message to be skipped",
}

func main() {
	apiToken := os.Getenv("TGM_API_TOKEN")
	chatId, _ := strconv.ParseInt(os.Getenv("TGM_CHAT_ID"), 10, 64)
	if len(apiToken) == 0 || chatId == 0 {
		fmt.Println("set TGM_API_TOKEN and TGM_CHAT_ID environment varibales to make it work")
		return
	}

	ctx := context.Background()
	w := golog2tgm.Start(
		golog2tgm.CallerSrcPath(),
		Intro,
		func(err string) {
			log.Ctx(ctx).Error().Msg(err)
		},
		nil,
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
	log.Logger = zerolog.New(output).With().Timestamp().Caller().Logger().Hook(
		TgmHook{
			PushMessage:       w.PushMessage,
			MinLevel:          zerolog.InfoLevel,
			CallStackMinLevel: zerolog.ErrorLevel,
		},
	)
	ctx = log.Logger.WithContext(ctx)

	log.Ctx(ctx).Info().Msg("start")
	w.Uncork(false)

	cctx, cancelSender := context.WithCancel(ctx)
	go func() {
		for range time.Tick(50 * time.Millisecond) {
			i := rand.Intn(len(testMessages))
			LogMessage(ctx, zerolog.Level(i%4+1), testMessages[i])
			select {
			case <-cctx.Done():
				return
			default:
			}
		}
	}()

	time.Sleep(time.Second * 3)
	cancelSender()
	log.Ctx(ctx).Error().Msg("stop")
	<-w.Uncork(true)
}
