package reflector

import (
	"io"
	"net/http"
	"time"
)

// HealthServer exposes the /health/live and /health/ready endpoints expected by the
// Helm chart probes. Liveness is always OK once the process is up; readiness flips
// to OK after the informer caches have synced.
type HealthServer struct {
	ready func() bool
}

func NewHealthServer(ready func() bool) *HealthServer {
	return &HealthServer{ready: ready}
}

func (h *HealthServer) Start(addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/health/live", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "Healthy")
	})
	mux.HandleFunc("/health/ready", func(w http.ResponseWriter, _ *http.Request) {
		if h.ready() {
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, "Healthy")
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, "Unhealthy")
	})

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	return srv.ListenAndServe()
}
