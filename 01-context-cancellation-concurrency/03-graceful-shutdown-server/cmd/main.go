package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

type Task struct {
	id      string
	payload string
	created time.Time
}

type ServerConfig struct {
	port    string
	timeout time.Duration
	workers int
}

type Server struct {
	logger     *slog.Logger
	httpServer *http.Server
	queue      chan Task
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	db         net.Conn
	workers    int
}

func NewServer(cfg *ServerConfig, logger *slog.Logger) *Server {
	ctx, cancel := context.WithCancel(context.Background())

	dbSide, _ := net.Pipe()

	srv := &Server{
		logger:  logger,
		queue:   make(chan Task, 100),
		ctx:     ctx,
		cancel:  cancel,
		db:      dbSide,
		workers: cfg.workers,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/task", srv.handleTask)

	srv.httpServer = &http.Server{
		Addr:         ":" + cfg.port,
		Handler:      mux,
		WriteTimeout: cfg.timeout,
		ReadTimeout:  cfg.timeout,
	}

	return srv
}

func (s *Server) handleTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	task := Task{
		id:      r.URL.Query().Get("id"),
		payload: "data from request",
		created: time.Now(),
	}

	select {
	case s.queue <- task:
		w.WriteHeader(http.StatusAccepted)
		fmt.Fprintf(w, "Task %s queued", task.id)
	case <-time.After(1 * time.Second):
		s.logger.Warn("queue is full, rejecting task", "id", task.id)
		http.Error(w, "Server busy", http.StatusServiceUnavailable)
	}
}

func (s *Server) cacheWarmer() {
	defer s.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.logger.Info("start cache warmer")
			time.Sleep(300 * time.Millisecond)

		case <-s.ctx.Done():
			s.logger.Debug("cache warmer stopping")
			return
		}
	}

}

func (s *Server) Start() error {
	s.logger.Info("starting server", "addr", s.httpServer.Addr)
	for i := range s.workers {
		s.wg.Add(1)
		go s.worker(i)
	}

	s.wg.Add(1)
	go s.cacheWarmer()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("server error", "err", err)
			s.cancel()
		}
	}()

	<-stop

	s.logger.Info("shutdown signal received")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return s.Stop(ctx)
}

func (s *Server) Stop(ctx context.Context) error {
	if err := s.httpServer.Shutdown(ctx); err != nil {
		s.logger.Error("http shutdown error", "err", err)
		return err
	}
	s.cancel()

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		s.logger.Error("shutdown timeout", "err", ctx.Err())
		return ctx.Err()
	}

	if err := s.db.Close(); err != nil {
		s.logger.Error("db close error", "err", err)
		return err
	}

	s.logger.Info("database connection closed")

	s.logger.Info("graceful shutdown complete")
	return nil
}

func (s *Server) worker(id int) {
	defer s.wg.Done()
	s.logger.Debug("worker started", "id", id)

	for {
		select {
		case task := <-s.queue:
			s.logger.Info("worker processing task", "worker_id", id, "task_id", task.id)
			time.Sleep(5 * time.Second)
			s.logger.Info("worker complete task", "worker_id", id, "task_id", task.id)
		case <-s.ctx.Done():
			s.logger.Debug("worker stopping", "id", id)
			return
		}
	}
}

func main() {
	logger := slog.New(
		slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}),
	)

	cfg := &ServerConfig{
		port:    "8080",
		timeout: 5 * time.Second,
		workers: 10,
	}

	server := NewServer(cfg, logger)
	if err := server.Start(); err != nil {
		logger.Error("server start error", "err", err)
		os.Exit(1)
	}
}
