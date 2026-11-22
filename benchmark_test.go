package golog2tgm

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

type rewriteTransport struct{ target *url.URL }

func (t *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r := req.Clone(req.Context())
	r.URL.Scheme = t.target.Scheme
	r.URL.Host = t.target.Host
	return http.DefaultTransport.RoundTrip(r)
}

func mockTG(b *testing.B) (*httptest.Server, *http.Client) {
	b.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}))
	u, _ := url.Parse(ts.URL)
	return ts, &http.Client{
		Transport: &rewriteTransport{target: u},
		Timeout:   5 * time.Second,
	}
}

// messages with hashing
func BenchmarkBatchPushMsg(b *testing.B) {
	msg := logMessage{
		level:   1,
		message: strings.Repeat("x", 500),
	}
	var bat batch
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bat.pushMsg(&msg)
	}
}

// messages without hashing
func BenchmarkBatchPushMsgWithHash(b *testing.B) {
	msg := logMessage{
		level:   1,
		hash:    123456789,
		message: strings.Repeat("x", 500),
	}
	var bat batch
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bat.pushMsg(&msg)
	}
}

// -benchtime=1000x (< Worker.cMsg cap) gives 2xBenchmarkBatchPushMsg time
func BenchmarkWorkerPushMsg(b *testing.B) {
	ts, client := mockTG(b)
	defer ts.Close()

	w := Start("", nil, nil, client)
	msg := strings.Repeat("x", 500)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.PushMessage(1, 0, msg, nil)
	}
	b.StopTimer()
	w.Stop()
}
