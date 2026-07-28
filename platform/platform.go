// @import { Bean, Configuration } from "github.com/StevenBuglione/spice/annotation/core"
// @import { OnStart, OnStop } from "github.com/StevenBuglione/spice/annotation/lifecycle"
// @import { Module } from "github.com/StevenBuglione/spice/annotation/modulith"

// Package platform owns the commerce application's HTTP transport lifecycle.
//
// @Module(allowedDependencies=["github.com/StevenBuglione/spice/examples/commerce/storage"])
package platform

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/StevenBuglione/spice/examples/commerce/storage"
)

// Settings contains safe HTTP server defaults.
//
// @Configuration(prefix="commerce.server")
type Settings struct {
	Address           string        `spice:"address,default=127.0.0.1:8081,env=SPICE_COMMERCE_ADDRESS"`
	ReadHeaderTimeout time.Duration `spice:"read-header-timeout,default=5s"`
	ReadTimeout       time.Duration `spice:"read-timeout,default=15s"`
	WriteTimeout      time.Duration `spice:"write-timeout,default=15s"`
	IdleTimeout       time.Duration `spice:"idle-timeout,default=60s"`
}

// Mux supplies the route table populated by generated controller adapters.
//
// @Bean
func Mux() *http.ServeMux {
	return http.NewServeMux()
}

// Server owns the commerce HTTP listener and graceful-drain lifecycle.
type Server struct {
	mu          sync.RWMutex
	httpServer  *http.Server
	serveErrors chan error
	listener    net.Listener
	started     bool
}

// NewServer constructs a lifecycle-managed server with bounded timeouts.
//
// @Bean
func NewServer(
	settings Settings,
	handler *http.ServeMux,
	database *storage.Database,
) (*Server, error) {
	if strings.TrimSpace(settings.Address) == "" {
		return nil, errors.New("construct commerce server: address is empty")
	}
	if settings.ReadHeaderTimeout <= 0 ||
		settings.ReadTimeout <= 0 ||
		settings.WriteTimeout <= 0 ||
		settings.IdleTimeout <= 0 {
		return nil, errors.New("construct commerce server: all HTTP timeouts must be positive")
	}
	if handler == nil {
		return nil, errors.New("construct commerce server: handler is nil")
	}
	if database == nil {
		return nil, errors.New("construct commerce server: database is nil")
	}
	return &Server{
		httpServer: &http.Server{
			Addr:              settings.Address,
			Handler:           handler,
			ReadHeaderTimeout: settings.ReadHeaderTimeout,
			ReadTimeout:       settings.ReadTimeout,
			WriteTimeout:      settings.WriteTimeout,
			IdleTimeout:       settings.IdleTimeout,
		},
		serveErrors: make(chan error, 1),
	}, nil
}

// Start binds the configured listener.
//
// @OnStart
func (server *Server) Start(ctx context.Context) error {
	if ctx == nil {
		return errors.New("start commerce server: context is nil")
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("start commerce server: %w", err)
	}
	server.mu.Lock()
	defer server.mu.Unlock()
	if server.started {
		return errors.New("start commerce server: server is already started")
	}
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", server.httpServer.Addr)
	if err != nil {
		return fmt.Errorf("start commerce server on %s: %w", server.httpServer.Addr, err)
	}
	server.listener = listener
	server.started = true
	go func() {
		server.serveErrors <- server.httpServer.Serve(listener)
	}()
	return nil
}

// Stop gracefully drains the listener and observes its serve result.
//
// @OnStop
func (server *Server) Stop(ctx context.Context) error {
	if ctx == nil {
		return errors.New("stop commerce server: context is nil")
	}
	server.mu.RLock()
	started := server.started
	server.mu.RUnlock()
	if !started {
		return nil
	}

	shutdownErr := server.httpServer.Shutdown(ctx)
	var serveErr error
	select {
	case serveErr = <-server.serveErrors:
		if errors.Is(serveErr, http.ErrServerClosed) {
			serveErr = nil
		}
	case <-ctx.Done():
		serveErr = ctx.Err()
	}
	return errors.Join(shutdownErr, serveErr)
}

// Address returns the bound listener address or the configured address before start.
func (server *Server) Address() string {
	server.mu.RLock()
	defer server.mu.RUnlock()
	if server.listener != nil {
		return server.listener.Addr().String()
	}
	return server.httpServer.Addr
}
