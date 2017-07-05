package zulip

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type Client struct {
	endpoint    string
	credentials string

	Debug bool
}

func New(endpoint string, username string, keyFile string) (*Client, error) {
	keyRaw, err := ioutil.ReadFile(keyFile)
	if err != nil {
		return nil, err
	}
	key := strings.TrimSpace(string(keyRaw))

	return &Client{
		endpoint:    endpoint,
		credentials: base64.StdEncoding.EncodeToString([]byte(username + ":" + key)),
	}, nil
}

type Message struct {
	RawId          json.Number `json:"id"`
	Type           string      `json:"type"`
	RawRecipient   interface{} `json:"display_recipient"`
	Subject        string      `json:"subject"`
	Content        string      `json:"content"`
	SenderEmail    string      `json:"sender_email"`
	SenderFullName string      `json:"sender_full_name"`

	Stream     string
	Recipients []string
}

func (m Message) Id() string {
	return m.RawId.String()
}

func (m Message) Reply(content string) Message {
	reply := Message{
		Type:    m.Type,
		Subject: m.Subject,
		Content: content,
	}
	switch m.Type {
	case "private":
		reply.Recipients = m.Recipients
	case "stream":
		reply.Stream = m.Stream
	default:
		panic(fmt.Errorf("unknown message type: %s", m.Type))
	}
	return reply
}

type Heartbeat struct {
	RawId json.Number `json:"id"`
}

func (h Heartbeat) Id() string {
	return h.RawId.String()
}

type baseResponse struct {
	Result string `json:"result"`
	Msg    string `json:"msg"`
}

type RegisterResponse struct {
	baseResponse

	QueueId     string      `json:"queue_id"`
	LastEventId json.Number `json:"last_event_id,number"`
}

func (c Client) Register(eventTypes ...string) (*RegisterResponse, error) {
	var vs url.Values
	if len(eventTypes) > 0 {
		ts, _ := json.Marshal(eventTypes)
		vs = url.Values{"event_types": []string{string(ts)}}
	}
	req, err := c.newRequest("POST", "register", vs)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var register RegisterResponse
	dec := json.NewDecoder(resp.Body)
	err = dec.Decode(&register)
	if err != nil {
		return nil, err
	}

	if register.Result != "success" {
		return nil, fmt.Errorf("register: %s: %s", register.Result, register.Msg)
	}

	return &register, nil
}

type Event interface {
	Id() string
}

type EventsResponse struct {
	baseResponse

	Events []struct {
		Type    string          `json:"type"`
		RawId   json.Number     `json:"id"`
		Message json.RawMessage `json:"message"`
	} `json:"events"`
}

func (c Client) Events(queueId, lastEventId string) ([]Event, error) {
	req, err := c.newRequest("GET", "events", url.Values{
		"queue_id":      []string{queueId},
		"last_event_id": []string{lastEventId},
	})
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var r io.Reader = resp.Body
	if c.Debug {
		r = io.TeeReader(resp.Body, os.Stdout)
	}

	var events EventsResponse
	dec := json.NewDecoder(r)
	err = dec.Decode(&events)
	if err != nil {
		return nil, err
	}

	if events.Result != "success" {
		return nil, fmt.Errorf("events: %s: %s", events.Result, events.Msg)
	}

	parsedEvents := make([]Event, len(events.Events))
	for i, rawEv := range events.Events {
		var ev Event
		var err error
		var data []byte

		switch rawEv.Type {
		case "message":
			data = rawEv.Message
			var v Message
			err = json.Unmarshal(data, &v)
			switch recipient := v.RawRecipient.(type) {
			case string:
				v.Stream = recipient
			case []interface{}:
				v.Recipients = make([]string, len(recipient))
				for i, r := range recipient {
					r, ok := r.(map[string]interface{})
					if !ok {
						return nil, fmt.Errorf("unknown recipient type: %T", r)
					}
					v.Recipients[i] = r["email"].(string)
				}
			default:
				return nil, fmt.Errorf("unknown recipient type: %T", recipient)
			}
			ev = v
		case "heartbeat":
			ev = Heartbeat{RawId: rawEv.RawId}
		default:
			return nil, fmt.Errorf("unknown event type: %s: %s", rawEv.Type)
		}

		if err != nil {
			return nil, fmt.Errorf("parsing event: type %s: %q: %s", rawEv.Type, string(data), err)
		}

		parsedEvents[i] = ev.(Event)
	}

	return parsedEvents, nil
}

func (c Client) newRequest(method string, action string, params url.Values) (*http.Request, error) {
	req, err := http.NewRequest(method, c.endpoint+"/api/v1/"+action, nil)
	if err != nil {
		return req, err
	}
	req.Header.Set("Authorization", "Basic "+c.credentials)
	switch method {
	case "GET":
		req.URL.RawQuery = params.Encode()
	case "POST":
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Body = ioutil.NopCloser(strings.NewReader(params.Encode()))
	default:
		return nil, fmt.Errorf("parameters for %s request not implemented", method)
	}
	return req, nil
}

func (c Client) Send(msg Message) error {
	vs := url.Values{}
	vs.Set("type", msg.Type)
	vs.Set("content", msg.Content)
	vs.Set("subject", msg.Subject)
	switch msg.Type {
	case "private":
		rs, _ := json.Marshal(msg.Recipients)
		vs.Set("to", string(rs))
	case "stream":
		vs.Set("to", msg.Stream)
	default:
		return fmt.Errorf("unknown message type: %s", msg.Type)
	}
	req, err := c.newRequest("POST", "messages", vs)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	dec := json.NewDecoder(resp.Body)
	var r baseResponse
	err = dec.Decode(&r)
	if err != nil {
		return err
	}

	if r.Result != "success" {
		return fmt.Errorf("sending message: %s", r.Msg)
	}

	return nil
}

func (c Client) OnEachEvent(handle func(Event)) {
	r, err := c.Register()
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

		events, err := c.Events(queueId, lastEventId)
		if err != nil {
			if IsBadQueue(err) {
				r, err := c.Register()
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
			handle(ev)
		}
	}
}

func IsBadQueue(err error) bool {
	return strings.HasPrefix("Bad event queue id:", err.Error())
}
