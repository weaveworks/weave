// Based on net/http/internal
package proxy

import (
	"bufio"
	"bytes"
	"strconv"
	"strings"
	"testing"
)

func TestChunk(t *testing.T) {
	r := bufio.NewScanner(bytes.NewBufferString(
		"7\r\nhello, \r\n17\r\nworld! 0123456789abcdef\r\n0\r\n",
	))
	r.Split(splitChunks)

	assertNextChunk(t, r, "hello, ")
	assertNextChunk(t, r, "world! 0123456789abcdef")
	assertNoMoreChunks(t, r)
}

func TestMalformedChunks(t *testing.T) {
	r := bufio.NewScanner(bytes.NewBufferString(
		"7\r\nhello, GARBAGEBYTES17\r\nworld! 0123456789abcdef\r\n0\r\n",
	))
	r.Split(splitChunks)

	// First chunk fails
	{
		if r.Scan() {
			t.Errorf("Expected failure when reading chunks, but got one")
		}
		e := "malformed chunked encoding"
		if r.Err() == nil || r.Err().Error() != e {
			t.Errorf("chunk reader errored %q; want %q", r.Err(), e)
		}
		data := r.Bytes()
		if len(data) != 0 {
			t.Errorf("chunk should have been empty. got %q", data)
		}
	}

	if r.Scan() {
		t.Errorf("Expected no more chunks, but found too many")
	}
}

func TestChunkTooLarge(t *testing.T) {
	data := make([]byte, maxChunkSize+1)
	r := bufio.NewScanner(bytes.NewBufferString(strings.Join(
		[]string{
			strconv.FormatInt(maxChunkSize+1, 16), string(data),
			"0", "",
		},
		"\r\n",
	)))
	r.Split(splitChunks)

	// First chunk fails
	{
		if r.Scan() {
			t.Errorf("Expected failure when reading chunks, but got one")
		}
		e := "chunk too long"
		if r.Err() == nil || r.Err().Error() != e {
			t.Errorf("chunk reader errored %q; want %q", r.Err(), e)
		}
		data := r.Bytes()
		if len(data) != 0 {
			t.Errorf("chunk should have been empty. got %q", data)
		}
	}

	if r.Scan() {
		t.Errorf("Expected no more chunks, but found too many")
	}
}

func TestInvalidChunkSize(t *testing.T) {
	r := bufio.NewScanner(bytes.NewBufferString(
		"foobar\r\nhello, \r\n0\r\n",
	))
	r.Split(splitChunks)

	// First chunk fails
	{
		if r.Scan() {
			t.Errorf("Expected failure when reading chunks, but got one")
		}
		e := "invalid byte in chunk length"
		if r.Err() == nil || r.Err().Error() != e {
			t.Errorf("chunk reader errored %q; want %q", r.Err(), e)
		}
		data := r.Bytes()
		if len(data) != 0 {
			t.Errorf("chunk should have been empty. got %q", data)
		}
	}

	if r.Scan() {
		t.Errorf("Expected no more chunks, but found too many")
	}
}

func TestBytesAfterLastChunkAreIgnored(t *testing.T) {
	r := bufio.NewScanner(bytes.NewBufferString(
		"7\r\nhello, \r\n0\r\nGARBAGEBYTES",
	))
	r.Split(splitChunks)

	assertNextChunk(t, r, "hello, ")
	assertNoMoreChunks(t, r)
}

func assertNextChunk(t *testing.T, r *bufio.Scanner, expected string) {
	if !r.Scan() {
		t.Fatalf("Expected chunk, but ran out early: %v", r.Err())
	}
	if r.Err() != nil {
		t.Fatalf("Error reading chunk: %q", r.Err())
	}
	data := r.Bytes()
	if string(data) != expected {
		t.Errorf("chunk reader read %q; want %q", data, expected)
	}
}

func assertNoMoreChunks(t *testing.T, r *bufio.Scanner) {
	if r.Scan() {
		t.Errorf("Expected no more chunks, but found too many")
	}
	if r.Err() != nil {
		t.Errorf("Expected no error, but found: %q", r.Err())
	}
}
