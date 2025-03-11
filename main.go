package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"time"
)

func createAndStartMockServer(serverAddr string) *http.Server {
	mockServerMux := http.NewServeMux()
	mockServerMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("Hello from mock server", serverAddr)
		fmt.Println("Request received from", r.Host)
		fmt.Println("Original request from", r.Header.Get("X-Forwarded-For"))
		// Return status code 200
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Hello from mock server"))
	})
	mockServerMux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("Health check from mock server", serverAddr)
		// Return health status code 200 with a probability of 0.8
		if rand.Intn(10) > 1 {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
	})
	mockServer := http.Server{Addr: serverAddr, Handler: mockServerMux}
	log.Println("Starting mock server", serverAddr)
	go mockServer.ListenAndServe()
	return &mockServer
}

func main() {
	// Create a new loadbalancer
	loadbalancerAddr := "127.0.0.1:8080"
	loadbalancer, err := NewServer(loadbalancerAddr)
	if err != nil {
		log.Fatalf("error creating server: %s", err)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	go loadbalancer.Start()
	// Create mock servers
	mockServers := make([]*http.Server, 0)
	for i := 2; i <= 9; i++ {
		serverAddr := fmt.Sprintf("127.0.0.1:808%d", i)
		mockServer := createAndStartMockServer(serverAddr)
		mockServers = append(mockServers, mockServer)
		// Associate listener for mock server
		listener, err := NewListener(serverAddr, WithListenerHealthCheck(8080+i, "/health", 5*time.Second))
		if err != nil {
			log.Fatalf("error creating listener: %s", err)
		}
		// Add listener to loadbalancer
		loadbalancer.AddListener(listener)
	}

	mainServerMux := http.NewServeMux()
	// TODO: allow loadbalancer to service different domains
	mainServerMux.HandleFunc("/", loadbalancer.handle)
	mainServer := &http.Server{Addr: loadbalancerAddr, Handler: mainServerMux}
	go mainServer.ListenAndServe()
	<-sigCh
	mainServer.Shutdown(context.Background())
	for _, s := range mockServers {
		s.Shutdown(context.Background())
	}
	loadbalancer.Stop()
}
