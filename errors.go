package main

import "fmt"

type ErrServerCreate struct {
	reason string
}

func (e *ErrServerCreate) Error() string {
	return fmt.Sprintf("error creating server: %s", e.reason)
}

type ErrServerStopped struct {
}

func (e *ErrServerStopped) Error() string {
	return "server is stopped"
}

type ErrServerGenericError struct {
	reason string
}

func (e *ErrServerGenericError) Error() string {
	return fmt.Sprintf("server error: %s", e.reason)
}

type ErrListenerCreate struct {
	reason string
}

func (e *ErrListenerCreate) Error() string {
	return fmt.Sprintf("error creating listener: %s", e.reason)
}
