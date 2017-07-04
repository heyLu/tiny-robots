package main

import (
	"fmt"
	"log"
	"strings"
	"time"

	"./zulip"
)

func main() {
	client, err := zulip.New("https://zulip.papill0n.org", "announcy-bot@zulip.papill0n.org", "api_key.txt")
	if err != nil {
		log.Println("creating client:", err)
	}

	onEachEvent(client, func(ev zulip.Event) {
		switch ev := ev.(type) {
		case zulip.Message:
			switch {
			case strings.HasPrefix(ev.Content, "!hi"):
				r := ev.Reply(fmt.Sprintf("%s said hi!", ev.SenderEmail))
				err := client.Send(r)
				if err != nil {
					log.Println("sending message", err)
				}
			}
		case zulip.Heartbeat:
			fmt.Println("heartbeat")
		default:
			log.Println("unhandled message")
		}
	})
}

func onEachEvent(client *zulip.Client, handle func(zulip.Event)) {
	r, err := client.Register("message")
	if err != nil {
		log.Println("registering queue:", err)
	}
	queueId := r.QueueId
	lastEventId := r.LastEventId.String()

	first := true
	for {
		if !first {
			time.Sleep(500 * time.Millisecond)
		}
		first = false

		events, err := client.Events(queueId, lastEventId)
		if err != nil {
			if zulip.IsBadQueue(err) {
				r, err := client.Register("message")
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
