package main

import (
	"fmt"
	"log"
	"time"

	"github.com/heyLu/tiny-robots/zulip"
)

type SimpleClient struct {
	*zulip.Client
}

func New(endpoint, botEmail, keyPath string) (*SimpleClient, error) {
	c, err := zulip.New(endpoint, botEmail, keyPath)
	if err != nil {
		return nil, err
	}
	return &SimpleClient{c}, nil
}

// Send sends the message, logging the error if it occurs.
//
// The error is returned so that callers can change their control flow
// if errors happen.
func (c *SimpleClient) Send(msg zulip.Message) error {
	err := c.Client.Send(msg)
	if err != nil {
		log.Println("sending message:", err)
	}
	return err
}

// Reply replies to the message, logging the error if it occurs.
func (c *SimpleClient) Reply(msg zulip.Message, content string) error {
	return c.Send(msg.Reply(content))
}

// Reply replies to the message formatted according to `fmt.Sprintf`.
//
// Equivalent to calling `c.Reply(msg, fmt.Sprintf(fmt, args...))`.
func (c *SimpleClient) Replyf(msg zulip.Message, format string, args ...interface{}) error {
	return c.Send(msg.Reply(fmt.Sprintf(format, args...)))
}

func (c *SimpleClient) OnEachEvent(handle func(zulip.Event)) {
	r, err := c.Register("message")
	if err != nil {
		log.Fatal("registering queue:", err)
	}
	queueId := r.QueueId
	lastEventId := r.LastEventId.String()

	first := true
	for {
		if !first {
			time.Sleep(500 * time.Millisecond)
		}
		first = false

		events, err := c.Events(queueId, lastEventId)
		if err != nil {
			if zulip.IsBadQueue(err) {
				r, err := c.Register("message")
				if err != nil {
					log.Println("registering queue:", err)
				}
				queueId = r.QueueId
				lastEventId = r.LastEventId.String()
			}
			log.Println("getting events:", err)
			continue
		}

		for _, ev := range events {
			lastEventId = ev.Id()

			handle(ev)
		}
	}
}
