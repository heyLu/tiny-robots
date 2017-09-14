package main

import (
	"fmt"
	"io/ioutil"
	"log"

	"github.com/heyLu/tiny-robots/rocket"
)

type Client interface {
	Send(Message) error
	Reply(Message, string) error
	Replyf(Message, string, ...interface{}) error
	OnEachMessage(func(Message))
}

type Message interface {
	Author() string
	Content() string
}

type SimpleClient struct {
	*rocket.Client
}

func New(endpoint, userName, keyPath string, roomID string) (Client, error) {
	data, err := ioutil.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}

	c, err := rocket.New(endpoint, userName, string(data), roomID)
	if err != nil {
		return nil, err
	}
	return &SimpleClient{c}, nil
}

// Send sends the message, logging the error if it occurs.
//
// The error is returned so that callers can change their control flow
// if errors happen.
func (c *SimpleClient) Send(msg Message) error {
	err := c.Client.Send(msg.(rocket.Message))
	if err != nil {
		log.Println("sending message:", err)
	}
	return err
}

// Reply replies to the message, logging the error if it occurs.
func (c *SimpleClient) Reply(msg Message, content string) error {
	return c.Client.Reply(msg.(rocket.Message), content)
}

// Reply replies to the message formatted according to `fmt.Sprintf`.
//
// Equivalent to calling `c.Reply(msg, fmt.Sprintf(fmt, args...))`.
func (c *SimpleClient) Replyf(msg Message, format string, args ...interface{}) error {
	return c.Reply(msg.(rocket.Message), fmt.Sprintf(format, args...))
}

func (c *SimpleClient) OnEachMessage(handle func(Message)) {
	c.Client.OnEachMessage(func(msg rocket.Message) {
		handle(msg)
	})
}
