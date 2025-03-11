package main

import (
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/http"
	"sync/atomic"
	"time"
)

type BalancingAlgorithm func([]*Listener) *Listener

func (b BalancingAlgorithm) pickListener(listeners []*Listener) (*Listener, error) {
	if len(listeners) == 0 {
		return nil, &ErrServerGenericError{reason: "no listeners available"}
	}
	return b(listeners), nil
}

var DefaultBalancingAlgorithm = Random()

func Random() BalancingAlgorithm {
	return func(listeners []*Listener) *Listener {
		return listeners[rand.Intn(len(listeners))]
	}
}

func RoundRobin() BalancingAlgorithm {
	var i int
	return func(listeners []*Listener) *Listener {
		i = (i + 1) % len(listeners)
		return listeners[i]
	}
}

func WeightedRoundRobin() BalancingAlgorithm {
	return func(listeners []*Listener) *Listener {
		// Simple algorithm that flattens the list of listeners by their weight
		// and then picks a random listener from the flattened list.
		// Not the most space-efficient algorithm, but it's simple and works.
		flatSlice := make([]int, 0)
		for i, listener := range listeners {
			for j := 0; j < listener.weight; j++ {
				flatSlice = append(flatSlice, i)
			}
		}
		return listeners[flatSlice[rand.Intn(len(flatSlice))]]
	}
}

type ServerConfig func(*Server)

func WithBalancingAlgorithm(b BalancingAlgorithm) ServerConfig {
	return func(s *Server) {
		s.balancingAlgorithm = b
	}
}

type Server struct {
	listeners          []*Listener
	unhealthyListeners map[string]bool
	ip                 net.IP
	stopped            atomic.Bool
	balancingAlgorithm BalancingAlgorithm
}

func NewServer(addr string, config ...ServerConfig) (*Server, error) {
	ip, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, &ErrServerCreate{reason: fmt.Sprintf("unable to parse host port address %s", addr)}
	}
	parsedIp := net.ParseIP(ip)
	if parsedIp == nil {
		return nil, &ErrServerCreate{reason: fmt.Sprintf("unable to parse ip %s", ip)}
	}

	s := &Server{ip: parsedIp}
	for _, c := range config {
		c(s)
	}
	if s.balancingAlgorithm == nil {
		s.balancingAlgorithm = DefaultBalancingAlgorithm
	}
	return s, nil
}

func (s *Server) AddListener(listener *Listener) {
	log.Println("Adding new listener to listen on", listener.getTargetAddr())
	s.listeners = append(s.listeners, listener)
}

func (s *Server) healthcheck() {
	if len(s.listeners) == 0 {
		log.Println("No listeners available")
		return
	}
	for _, listener := range s.listeners {
		go func() {
			response := listener.healthcheck()
			if response.err != nil {
				log.Printf("Listener %s is unhealthy: %s", listener.id, response.err)
				s.unhealthyListeners[listener.id.String()] = true
			} else {
				log.Printf("Listener %s is healthy", listener.id)
				delete(s.unhealthyListeners, listener.id.String())
			}
		}()
	}
}

func (s *Server) Start() error {
	if s.stopped.Load() {
		return &ErrServerStopped{}
	}
	for !s.stopped.Load() {
		s.healthcheck()
		time.Sleep(5 * time.Second)
	}
	return nil
}

func (s *Server) Stop() error {
	if s.stopped.Load() {
		return &ErrServerStopped{}
	}
	log.Println("Stopping server")
	s.stopped.Store(true)
	log.Println("Waiting for listeners to stop")
	for _, listener := range s.listeners {
		listener.wg.Wait()
	}
	log.Println("Server stopped")
	return nil
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	if s.stopped.Load() {
		http.Error(w, "server is stopped", http.StatusServiceUnavailable)
		return
	}
	var listener *Listener
	// Note: even with the check, there is no guarantee that the listener
	// selected is healthy. This is because the healthcheck is done in a
	// separate goroutine and the listener may only be marked unhealthy
	// after the healthcheck
	for listener == nil || s.unhealthyListeners[listener.id.String()] {
		selected, err := s.balancingAlgorithm.pickListener(s.listeners)
		if err != nil {
			log.Printf("Error selecting listener: %s", err)
			http.Error(w, "load balancer internal error", http.StatusServiceUnavailable)
			return
		}
		listener = selected
	}

	log.Printf("Request handled by listener %s", listener.id)
	listener.handle(w, r)
}
