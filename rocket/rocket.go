package rocket

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"

	"github.com/gorilla/websocket"
)

type Message struct {
	ID         string `json:"_id"`
	RoomID     string `json:"rid"`
	RawContent string `json:"msg"`

	author string
}

func (m Message) Author() string {
	return m.author
}

func (m Message) Content() string {
	return m.RawContent
}

type Client struct {
	conn *websocket.Conn
}

func New(url string, userName, passwordSHA256 string, roomID string) (*Client, error) {
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		return nil, err
	}

	err = conn.WriteMessage(websocket.TextMessage, []byte(`{"msg": "connect", "version": "1", "support": ["1"] }`))
	if err != nil {
		return nil, err
	}

	err = conn.WriteJSON(map[string]interface{}{
		"msg":    "method",
		"method": "login",
		"id":     "1",
		"params": []interface{}{
			map[string]interface{}{
				"user": map[string]string{"username": userName},
				"password": map[string]string{
					"digest":    passwordSHA256,
					"algorithm": "sha-256",
				},
			},
		},
	})
	if err != nil {
		return nil, err
	}

	msg := fmt.Sprintf(`{"msg": "sub", "id": "2", "name": "stream-room-messages", "params":[%q, false]}`, roomID)
	err = conn.WriteMessage(websocket.TextMessage, []byte(msg))
	if err != nil {
		return nil, err
	}

	return &Client{
		conn: conn,
	}, nil
}

func (c *Client) Send(msg Message) error {
	return c.conn.WriteJSON(map[string]interface{}{
		"msg":    "method",
		"method": "sendMessage",
		"id":     fmt.Sprintf("%d", rand.Intn(1<<30)),
		"params": []interface{}{
			map[string]string{
				"_id": randomString(10),
				"rid": msg.RoomID,
				"msg": msg.RawContent,
			},
		},
	})

}

func (c *Client) Reply(msg Message, content string) error {
	return c.Send(Message{RoomID: msg.RoomID, RawContent: content})
}

func randomString(length int) string {
	buf := make([]byte, length)
	_, err := rand.Read(buf)
	if err != nil {
		log.Fatal("rand.Read", err)
	}
	return fmt.Sprintf("%x", buf)
}

func (c *Client) OnEachMessage(handle func(Message)) {
	for {
		messageType, data, err := c.conn.ReadMessage()
		if err != nil {
			log.Fatal("read", err)
		}

		if messageType != websocket.TextMessage {
			fmt.Printf("skipping message: type=%d data=%q\n", messageType, string(data))
			continue
		}

		var val map[string]interface{}
		err = json.Unmarshal(data, &val)
		if err != nil {
			log.Fatal("parse", err)
		}

		msg, ok := val["msg"].(string)
		if !ok {
			continue
		}
		switch {
		case msg == "ping":
			err = c.conn.WriteMessage(websocket.TextMessage, []byte(`{"msg": "pong"}`))
		case msg == "changed" && val["collection"] == "stream-room-messages":
			username := findValue(val, "fields", "args", 0, "u", "username").(string)
			content := findValue(val, "fields", "args", 0, "msg").(string)
			roomID := findValue(val, "fields", "args", 0, "rid").(string)
			handle(Message{author: username, RoomID: roomID, RawContent: content})
		default:
			fmt.Printf("unknown message: %v\n", val)
		}

		if err != nil {
			log.Fatal("response", err)
		}
	}
}

func findValue(val interface{}, keys ...interface{}) interface{} {
	for _, key := range keys {
		switch v := val.(type) {
		case map[string]interface{}:
			val = v[key.(string)]
		case []interface{}:
			val = v[key.(int)]
		case nil:
			return nil
		default:
			log.Fatalf("unhandled: %#v\n", val)
		}
	}
	return val
}
