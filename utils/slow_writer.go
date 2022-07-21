package utils

import (
	"fmt"
	"io"

	"github.com/juju/ratelimit"
)

type SlowWriter struct {
	ratelimit float64
	w         io.Writer
}

func (sw *SlowWriter) SetRatelimit(ratelimit float64) {
	sw.ratelimit = ratelimit
}

func (sw *SlowWriter) Write(str string) {
	bucket := ratelimit.NewBucketWithRate(sw.ratelimit, 10)
	w := ratelimit.Writer(sw.w, bucket)
	fmt.Fprint(w, str)
}

func (sw *SlowWriter) WriteLn(str string) {
	sw.Write(fmt.Sprintf("%s\n", str))
}

func (sw *SlowWriter) WriteLnUnlimited(str string) {
	fmt.Fprint(sw.w, fmt.Sprintf("%s\n", str))
}

func NewSlowWriter(ratelimit float64, w io.Writer) *SlowWriter {
	sw := &SlowWriter{
		ratelimit: ratelimit,
		w:         w,
	}
	return sw
}
