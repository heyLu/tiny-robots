package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type EventsResponse struct {
	Result string  `json:"result"`
	Msg    string  `json:"msg"`
	Events []Event `json:"events"`
}

type Event struct {
	Type      string      `json:"type"`
	Id        int         `json:"id"`
	Message   Message     `json:"message"`
	Heartbeat interface{} `json:"heartbeat"`
}

type Message struct {
	Id               int       `json:"id"`
	Type             string    `json:"type"`
	SenderEmail      string    `json:"sender_email"`
	DisplayRecipient Recipient `json:"display_recipient"`
	Subject          string    `json:"subject"`
	Content          string    `json:"content"`
}

type Recipient struct {
	Stream     string
	Recipients []string
}

func (r *Recipient) UnmarshalJSON(data []byte) error {
	var vals interface{}
	err := json.Unmarshal(data, &vals)
	if err != nil {
		return err
	}

	switch vs := vals.(type) {
	case string:
		r.Stream = vs
	case []interface{}:
		for _, v := range vs {
			e := v.(map[string]interface{})["email"].(string)
			r.Recipients = append(r.Recipients, e)
		}
	default:
		return fmt.Errorf("recipient: unhandled value %#v", vals)
	}

	return nil
}

func main() {
	k, err := ioutil.ReadFile("api_key.txt")
	if err != nil {
		exit("reading api key", err)
	}
	apiKey := strings.TrimSpace(string(k))

	if len(os.Args) != 3 {
		fmt.Fprintf(os.Stderr, "Usage: %s [queue-id] [last-event-id]\n", os.Args[0])
		os.Exit(1)
	}
	queueId := os.Args[1]
	lastEventId := os.Args[2]

	for {
		req := newAPIRequest("GET", "events")
		req.Header.Set("Authorization", basicAuth("announcy-bot@zulip.papill0n.org", apiKey))
		q := req.URL.Query()
		q.Set("queue_id", queueId)
		q.Set("last_event_id", lastEventId)
		req.URL.RawQuery = q.Encode()

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			exit("getting events", err)
		}

		data, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			exit("reading response", err)
		}
		resp.Body.Close()
		os.Stdout.Write(data)

		var events EventsResponse
		err = json.Unmarshal(data, &events)
		if err != nil {
			exit("parsing response", err)
		}
		//fmt.Printf("%#v\n", events)

		if events.Result == "error" {
			if strings.HasPrefix(events.Msg, "Bad event queue id:") {
				queueId, lastEventId = newQueue(apiKey)
				fmt.Printf("new queue: queue_id=%s, last_event_id=%s\n", queueId, lastEventId)
			}
			continue
		}

		for _, ev := range events.Events {
			lastEventId = strconv.Itoa(ev.Id)

			switch ev.Type {
			case "message":
				fmt.Println("message:", ev.Message)

				if strings.HasPrefix(ev.Message.Content, "!hi") {
					req := newAPIRequest("POST", "messages")
					req.Header.Set("Authorization", basicAuth("announcy-bot@zulip.papill0n.org", apiKey))
					var to string
					switch ev.Message.Type {
					case "private":
						to = fmt.Sprintf(`[%q]`, ev.Message.SenderEmail)
					case "stream":
						to = ev.Message.DisplayRecipient.Stream
					default:
						exit("send hi", fmt.Errorf("unknown message type %q", ev.Message.Type))
					}
					form := url.Values{}
					form.Set("type", ev.Message.Type)
					form.Set("subject", ev.Message.Subject)
					form.Set("to", to)
					form.Set("content", fmt.Sprintf("%s said hi!", ev.Message.SenderEmail))
					req.Body = ioutil.NopCloser(strings.NewReader(form.Encode()))

					resp, err := http.DefaultClient.Do(req)
					if err != nil {
						exit("send hi", err)
					}

					io.Copy(os.Stdout, resp.Body)
					resp.Body.Close()
				}
			case "heartbeat":
				fmt.Println("heartbeat")
				continue
			default:
				exit("handle message", fmt.Errorf("unknown message: %s", ev.Type))
			}
		}

		time.Sleep(500 * time.Millisecond)
	}
}

func newAPIRequest(method string, action string) *http.Request {
	req, err := http.NewRequest(method, fmt.Sprintf("https://zulip.papill0n.org/api/v1/%s", action), nil)
	if err != nil {
		exit("creating API request", err)
	}
	return req
}

func basicAuth(username, password string) string {
	return fmt.Sprintf("Basic %s", base64.StdEncoding.EncodeToString([]byte(username+":"+password)))
}

func newQueue(apiKey string) (string, string) {
	req := newAPIRequest("POST", "register")
	req.Header.Set("Authorization", basicAuth("announcy-bot@zulip.papill0n.org", apiKey))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Body = ioutil.NopCloser(strings.NewReader(url.Values{"event_types": []string{`["message"]`}}.Encode()))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		exit("register queue", err)
	}
	defer resp.Body.Close()

	dec := json.NewDecoder(resp.Body)
	var queueInfo map[string]interface{}
	err = dec.Decode(&queueInfo)
	if err != nil {
		exit("parsing queue info", err)
	}

	if queueInfo["result"] == "error" {
		exit("register queue", fmt.Errorf(queueInfo["msg"].(string)))
	}

	return queueInfo["queue_id"].(string), fmt.Sprintf("%0.f", queueInfo["last_event_id"].(float64))
}

func exit(msg string, err error) {
	fmt.Fprintf(os.Stderr, "Error: %s: %s\n", msg, err)
	os.Exit(1)
}
