package middleware

import (
	"bufio"
	"net"
	"net/http"
	"testing"
)

func TestStatusRecorderHijackDelegatesToUnderlyingWriter(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	readWriter := bufio.NewReadWriter(bufio.NewReader(serverConn), bufio.NewWriter(serverConn))
	writer := &hijackableResponseWriter{
		header:     make(http.Header),
		conn:       serverConn,
		readWriter: readWriter,
	}

	recorder := &statusRecorder{ResponseWriter: writer}

	conn, rw, err := recorder.Hijack()
	if err != nil {
		t.Fatalf("Hijack() error = %v", err)
	}
	if conn != serverConn {
		t.Fatal("Hijack() returned unexpected connection")
	}
	if rw != readWriter {
		t.Fatal("Hijack() returned unexpected read writer")
	}
}

func TestStatusRecorderHijackRejectsUnsupportedWriter(t *testing.T) {
	recorder := &statusRecorder{ResponseWriter: httptestResponseWriter{header: make(http.Header)}}

	_, _, err := recorder.Hijack()
	if err != http.ErrNotSupported {
		t.Fatalf("Hijack() error = %v, want %v", err, http.ErrNotSupported)
	}
}

type hijackableResponseWriter struct {
	header     http.Header
	conn       net.Conn
	readWriter *bufio.ReadWriter
}

func (w *hijackableResponseWriter) Header() http.Header {
	return w.header
}

func (w *hijackableResponseWriter) Write(payload []byte) (int, error) {
	return len(payload), nil
}

func (w *hijackableResponseWriter) WriteHeader(int) {}

func (w *hijackableResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return w.conn, w.readWriter, nil
}

type httptestResponseWriter struct {
	header http.Header
}

func (w httptestResponseWriter) Header() http.Header {
	return w.header
}

func (w httptestResponseWriter) Write(payload []byte) (int, error) {
	return len(payload), nil
}

func (w httptestResponseWriter) WriteHeader(int) {}
