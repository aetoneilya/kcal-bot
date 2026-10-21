package server

import (
	"net/http"
	"time"
)

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(p []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(p)
	r.bytes += n
	return n, err
}

// logRequests emits one structured line per API request. It runs inside
// authenticate so the user id is known; query strings and headers stay out —
// init data travels in a header, and nothing in the path identifies more than
// a meal id.
func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(recorder, r)

		log := s.cfg.Log.Info
		if recorder.status >= http.StatusInternalServerError {
			log = s.cfg.Log.Error
		} else if recorder.status >= http.StatusBadRequest && recorder.status != http.StatusNotFound {
			log = s.cfg.Log.Warn
		}
		log("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", recorder.status,
			"bytes", recorder.bytes,
			"ms", time.Since(start).Milliseconds(),
			"user", userFrom(r.Context()).ID,
		)
	})
}
