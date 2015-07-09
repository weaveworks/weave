// Based on net/http/internal
package proxy

import (
	"bytes"
	"io"
	"io/ioutil"
	"testing"
)

func TestChunk(t *testing.T) {
	r := NewChunkedReader(bytes.NewBufferString(
		"7\r\nhello, \r\n17\r\nworld! 0123456789abcdef\r\n0\r\n",
	))

	assertNextChunk(t, r, "hello, ")
	assertNextChunk(t, r, "world! 0123456789abcdef")
	assertNoMoreChunks(t, r)
}

func TestIncompleteReadOfChunk(t *testing.T) {
	r := NewChunkedReader(bytes.NewBufferString(
		"7\r\nhello, \r\n17\r\nworld! 0123456789abcdef\r\n0\r\n",
	))

	// Incomplete read of first chunk
	{
		if !r.Next() {
			t.Fatalf("Expected chunk, but ran out early: %v", r.Err())
		}
		if r.Err() != nil {
			t.Fatalf("Error reading chunk: %q", r.Err())
		}
		// Read just 2 bytes
		buf := make([]byte, 2)
		if _, err := io.ReadFull(r.Chunk(), buf[:2]); err != nil {
			t.Fatalf("Error reading first bytes of chunk: %q", err)
		}
		if buf[0] != 'h' || buf[1] != 'e' {
			t.Fatalf("Unexpected first 2 bytes of chunk: %s", string(buf))
		}
	}

	// Second chunk still reads ok
	assertNextChunk(t, r, "world! 0123456789abcdef")

	assertNoMoreChunks(t, r)
}

func TestMalformedChunks(t *testing.T) {
	r := NewChunkedReader(bytes.NewBufferString(
		"7\r\nhello, GARBAGEBYTES17\r\nworld! 0123456789abcdef\r\n0\r\n",
	))

	// First chunk is ok
	assertNextChunk(t, r, "hello, ")

	// Second chunk fails
	{
		if r.Next() {
			t.Errorf("Expected failure when reading chunks, but got one")
		}
		e := "malformed chunked encoding"
		if r.Err() == nil || r.Err().Error() != e {
			t.Errorf("chunk reader errored %q; want %q", r.Err(), e)
		}
		data, err := ioutil.ReadAll(r.Chunk())
		if len(data) != 0 {
			t.Errorf("chunk should have been empty. got %q", string(data))
		}
		if err != nil {
			t.Logf(`data: "%s"`, data)
			t.Errorf("reading chunk: %v", err)
		}
	}

	if r.Next() {
		t.Errorf("Expected no more chunks, but found too many")
	}
}

func assertNextChunk(t *testing.T, r *ChunkedReader, expected string) {
	if !r.Next() {
		t.Fatalf("Expected chunk, but ran out early: %v", r.Err())
	}
	if r.Err() != nil {
		t.Fatalf("Error reading chunk: %q", r.Err())
	}
	data, err := ioutil.ReadAll(r.Chunk())
	if g := string(data); g != expected {
		t.Errorf("chunk reader read %q; want %q", g, expected)
	}
	if err != nil {
		t.Logf(`data: "%s"`, data)
		t.Fatalf("reading chunk: %v", err)
	}
}

func assertNoMoreChunks(t *testing.T, r *ChunkedReader) {
	if r.Next() {
		t.Errorf("Expected no more chunks, but found too many")
	}
	if r.Err() != nil {
		t.Errorf("Expected no error, but found: %q", r.Err())
	}
}
