package supervisor

import (
	"reflect"
	"testing"
)

func TestLogBufferHoldsPartialLine(t *testing.T) {
	b := NewLogBuffer(10)
	b.Write([]byte("hello")) // no newline yet
	if got := b.Lines(); len(got) != 0 {
		t.Fatalf("partial line should not surface: got %v", got)
	}
	b.Write([]byte(" world\nnext\n"))
	if got, want := b.Lines(), []string{"hello world", "next"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("lines = %v, want %v", got, want)
	}
}

func TestLogBufferCapsAndStripsCR(t *testing.T) {
	b := NewLogBuffer(3)
	b.Write([]byte("a\r\nb\n"))
	b.Write([]byte("par"))
	b.Write([]byte("tial\nc\nd\n")) // appended: a,b,partial,c,d → keep last 3
	if got, want := b.Lines(), []string{"partial", "c", "d"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("lines = %v, want %v", got, want)
	}
}
