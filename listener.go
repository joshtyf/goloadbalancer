package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
)

type ListenerConfig func(*Listener)

type ListenerHealthCheckConfig struct {
	port    int
	path    string
	timeout time.Duration
}

func WithListenerHealthCheck(port int, path string, timeout time.Duration) ListenerConfig {
	return func(l *Listener) {
		l.healthCheckConfig = &ListenerHealthCheckConfig{port: port, path: path, timeout: timeout}
	}
}

func WithWeight(weight int) ListenerConfig {
	if weight < 0 {
		weight = 0
	} else if weight > 100 {
		weight = 100
	}
	return func(l *Listener) {
		l.weight = weight
	}
}

type Listener struct {
	id                uuid.UUID
	targetIp          net.IP
	targetPort        int
	httpClient        http.Client
	healthCheckConfig *ListenerHealthCheckConfig
	weight            int
	wg                *sync.WaitGroup
}

func NewListener(targetAddr string, config ...ListenerConfig) (*Listener, error) {
	ip, port, err := net.SplitHostPort(targetAddr)
	if err != nil {
		return nil, &ErrListenerCreate{reason: fmt.Sprintf("unable to parse address %s", targetAddr)}
	}
	parsedIp := net.ParseIP(ip)
	if parsedIp == nil {
		return nil, &ErrListenerCreate{reason: fmt.Sprintf("unable to parse IP address %s", ip)}
	}
	parsedPort, err := strconv.Atoi(port)
	if err != nil {
		return nil, &ErrListenerCreate{reason: fmt.Sprintf("unable to convert port %s from string to int", port)}
	}
	l := &Listener{id: uuid.New(), targetIp: parsedIp, targetPort: parsedPort, httpClient: http.Client{}, weight: 1}
	for _, c := range config {
		c(l)
	}
	return l, nil
}

func (l *Listener) getTargetAddr() string {
	return net.JoinHostPort(l.targetIp.String(), strconv.Itoa(l.targetPort))
}

type ListenerHealthcheckResponse struct {
	err    error
	status string
}

func (l *Listener) healthcheck() ListenerHealthcheckResponse {
	if l.healthCheckConfig == nil {
		log.Println("no healthcheck configured")
		return ListenerHealthcheckResponse{status: "OK", err: nil}
	}
	req, err := http.NewRequest("GET", fmt.Sprintf("http://%s:%d%s", l.targetIp.String(), l.healthCheckConfig.port, l.healthCheckConfig.path), nil)
	if err != nil {
		log.Println("error creating healthcheck request", err)
		return ListenerHealthcheckResponse{status: "ERROR", err: err}
	}

	// TODO: set timeout
	resp, err := l.httpClient.Do(req)
	if err != nil {
		log.Println("error sending healthcheck request", err)
		return ListenerHealthcheckResponse{status: "ERROR", err: err}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Println("healthcheck failed with status", resp.StatusCode)
		return ListenerHealthcheckResponse{status: "ERROR", err: fmt.Errorf("healthcheck failed with status %d", resp.StatusCode)}
	}
	return ListenerHealthcheckResponse{status: "OK", err: nil}
}

func (l *Listener) handle(w http.ResponseWriter, r *http.Request) {
	l.wg.Add(1)
	defer l.wg.Done()
	proxyReq, err := http.NewRequest(r.Method, fmt.Sprintf("http://%s", l.getTargetAddr()), r.Body)
	if err != nil {
		log.Println("error creating forwarded request", err)
		http.Error(w, "unable to create forwarded request", http.StatusInternalServerError)
		return
	}
	// Copy original headers
	proxyReq.Header = r.Header
	// Append X-Forwarded-For header
	if r.Header.Get("X-Forwarded-For") != "" {
		proxyReq.Header.Set("X-Forwarded-For", r.Header.Get("X-Forwarded-For")+","+r.RemoteAddr)
	} else {
		proxyReq.Header.Set("X-Forwarded-For", r.RemoteAddr)
	}
	// Update X-Forwarded-Port header
	_, connectedLoadBalancerPort, err := net.SplitHostPort(r.Host)
	if err != nil {
		log.Println("error setting X-Forwarded-Port header", err)
		http.Error(w, "unable to create forwarded request", http.StatusInternalServerError)
		return
	}
	proxyReq.Header.Set("X-Forwarded-Port", connectedLoadBalancerPort)
	resp, err := l.httpClient.Do(proxyReq)
	if err != nil {
		http.Error(w, "error forwarding request", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()
	// Return response from target server to client
	io.Copy(w, resp.Body)
}
