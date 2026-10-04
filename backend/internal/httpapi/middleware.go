package httpapi

import (
	"log"
	"net/http"
	"runtime/debug"
	"strconv"
	"time"
)

// maxBodyBytes caps a request body. Every request this API accepts is a small
// JSON object; without a cap, one client can make the server read for as long
// as it likes.
const maxBodyBytes = 64 << 10

// recordingWriter remembers the status and size so the access log can report
// them, and so a handler that wrote nothing is still logged as 200.
type recordingWriter struct {
	http.ResponseWriter
	status  int
	written int
}

func (w *recordingWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
		w.ResponseWriter.WriteHeader(status)
	}
}

func (w *recordingWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(b)
	w.written += n
	return n, err
}

// withMiddleware wraps the routes with the things every response needs:
// a body cap, headers that stop this data being cached or sniffed, an access
// log, and a recovery so one bad request cannot take the service down with it.
func (s *Server) withMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		rw := &recordingWriter{ResponseWriter: w}

		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		}

		// Almost every response here is somebody's personal data, so none of
		// it may be stored by a browser, a proxy, or a back button.
		h := rw.Header()
		h.Set("Cache-Control", "no-store, no-cache, must-revalidate, private")
		h.Set("Pragma", "no-cache")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Cross-Origin-Resource-Policy", "same-origin")
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")

		defer func() {
			if v := recover(); v != nil {
				log.Printf("panic serving %s %s: %v\n%s", r.Method, r.URL.Path, v, debug.Stack())
				if rw.status == 0 {
					writeJSON(rw, http.StatusInternalServerError,
						apiError{Error: "Something went wrong. Nothing was changed."})
				}
			}
			log.Printf("%s %s %s %s", r.Method, r.URL.Path,
				strconv.Itoa(statusOf(rw)), time.Since(started).Round(time.Millisecond))
		}()

		next.ServeHTTP(rw, r)
	})
}

func statusOf(w *recordingWriter) int {
	if w.status == 0 {
		return http.StatusOK
	}
	return w.status
}
