package docker

import (
	"bytes"
	"strings"
	"testing"
)

func frame(stream byte, payload string) []byte {
	n := len(payload)
	return append([]byte{stream, 0, 0, 0, byte(n >> 24), byte(n >> 16), byte(n >> 8), byte(n)}, payload...)
}

func TestDemux(t *testing.T) {
	var in bytes.Buffer
	in.Write(frame(1, "hello\nwor"))
	in.Write(frame(2, "an error\n"))
	in.Write(frame(1, "ld\n"))
	var out, errOut strings.Builder
	if err := demux(&in, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if out.String() != "hello\nworld\n" || errOut.String() != "an error\n" {
		t.Fatalf("stdout %q, stderr %q", out.String(), errOut.String())
	}
}

func TestDemuxTruncatedFrame(t *testing.T) {
	in := bytes.NewReader(frame(1, "hello")[:10])
	var out strings.Builder
	if err := demux(in, &out, &out); err == nil {
		t.Fatal("want an error for a truncated frame")
	}
}

func TestLines(t *testing.T) {
	var got []string
	w := &lineWriter{fn: func(s string) { got = append(got, s) }}
	w.Write([]byte("a\nb"))
	w.Write([]byte("c\n\nd"))
	w.Flush()
	if strings.Join(got, "|") != "a|bc||d" {
		t.Fatalf("got %q", got)
	}
}
