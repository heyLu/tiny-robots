package main

import (
	"fmt"
	"log"
	"time"

	"./zulip"
)

func main() {
	client, err := zulip.New("https://zulip.papill0n.org", "announcy-bot@zulip.papill0n.org", "api_key.txt")
	if err != nil {
		log.Println("creating client:", err)
	}

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

			switch ev := ev.(type) {
			case zulip.Message:
				fmt.Println("message:", ev.Content)
			case zulip.Heartbeat:
				fmt.Println("heartbeat")
			default:
				log.Println("unhandled message")
			}
		}

		fmt.Println("last id:", lastEventId)
	}
}
