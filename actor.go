package main

type Actor interface {
	Start()
	Stop()
	Send(msg interface{})
}
