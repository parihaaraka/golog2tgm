package golog2tgm

import (
	"runtime"
	"strings"
)

func CallerSrcPath() string {
	var srcRoot string
	_, file, _, ok := runtime.Caller(1)
	if ok {
		if i := strings.LastIndexByte(file, '/'); i > 0 {
			srcRoot = file[:i+1]
		}
	}
	return srcRoot
}
