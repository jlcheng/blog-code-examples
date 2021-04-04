package main

import (
	"context"
	"fmt"
	"go.temporal.io/sdk/worker"
	"net/http"
	"sync"
	"time"
)

type (
	MultiServer struct {
		svrs []Server
	}

	Server interface {
		String() string
		Start()
		Stop()
	}

	HTTPServer struct {
		d  *http.Server
		rs *TransmitService
	}

	TemporalWorker struct {
		ch     chan interface{}
		worker worker.Worker
	}
)

var _ Server = (*HTTPServer)(nil)

func NewHTTPServer(d *http.Server, rs *TransmitService) *HTTPServer {
	return &HTTPServer{d, rs}
}

func (s *HTTPServer) String() string {
	return "HTTPServer"
}

func (s *HTTPServer) Start() {
	if err := s.d.ListenAndServe(); err != nil {
		if err != http.ErrServerClosed {
			panic(err)
		}
	}
}

func (s *HTTPServer) Stop() {
	if err := s.d.Shutdown(context.Background()); err != nil {
		panic(err)
	}
}

var _ Server = (*TemporalWorker)(nil)

func NewTemporalWorker(w worker.Worker) *TemporalWorker {
	return &TemporalWorker{ch: make(chan interface{}), worker: w}
}

func (s *TemporalWorker) String() string {
	return "TemporalWorker"
}

func (s *TemporalWorker) Start() {
	if err := s.worker.Run(s.ch); err != nil {
		panic(err)
	}
}

func (s *TemporalWorker) Stop() {
	close(s.ch)
}

func (r *TransmitService) Start() {
	ticker := time.NewTicker(time.Second)
LOOP:
	for {
		select {
		case <-r.done:
			ticker.Stop()
			break LOOP
		case <-ticker.C:
			r.process()
		}
	}
}

func (r *TransmitService) Stop() {
	close(r.done)
}

func NewMultiServer(svrs ...Server) *MultiServer {
	return &MultiServer{svrs: svrs}
}

func (s *MultiServer) Start() {
	var wg sync.WaitGroup
	wg.Add(len(s.svrs))
	for _, s := range s.svrs {
		go func(s Server) {
			defer wg.Done()
			fmt.Printf("starting %s\n", s.String())
			s.Start()
		}(s)
	}
	wg.Wait()
}

func (s *MultiServer) Stop() {
	for _, s := range s.svrs {
		func(s Server) {
			fmt.Printf("stopping %s\n", s.String())
			s.Stop()
		}(s)
	}
}
