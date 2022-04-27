package main

import (
	"fmt"
	"io"

	"github.com/juju/ratelimit"
)

type SlowWriter struct {
	ratelimit float64
	w         io.Writer
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

func NewSlowWriter(w io.Writer) *SlowWriter {
	sw := &SlowWriter{
		ratelimit: Conf.Ratelimit,
		w:         w,
	}
	return sw
}
