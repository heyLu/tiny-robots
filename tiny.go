package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"

	"./zulip"
)

var config struct {
	endpoint    string
	botEmail    string
	giphyAPIKey string
}

func init() {
	flag.StringVar(&config.endpoint, "endpoint", "https://zulip.papill0n.org", "The URL of the Zulip instance")
	flag.StringVar(&config.botEmail, "bot", "announcy-bot@zulip.papill0n.org", "The email address of the bot")
	flag.StringVar(&config.giphyAPIKey, "giphy", "", "The API key for Giphy")
}

func main() {
	flag.Parse()

	client, err := New(config.endpoint, config.botEmail, "api_key.txt")
	if err != nil {
		log.Println("creating client:", err)
	}

	go pipelineServer(client, ":12001")

	client.OnEachEvent(func(ev zulip.Event) {
		switch ev := ev.(type) {
		case zulip.Message:
			switch {
			case strings.HasPrefix(ev.Content, "!hi"):
				client.Replyf(ev, "%s said hi!", ev.SenderEmail)
			case strings.HasPrefix(ev.Content, "!failed"):
				var buf bytes.Buffer
				cmd := exec.Command("systemctl", "--failed")
				cmd.Stdout = &buf
				cmd.Stderr = &buf
				err := cmd.Run()
				if err != nil {
					log.Println("systemctl --failed:", err)
					return
				}

				client.Replyf(ev, "```\n$ systemctl --failed\n%s```", buf.String())
			case strings.HasPrefix(ev.Content, "!rm") || strings.HasPrefix(ev.Content, "!sh"):
				client.Replyf(ev, "```\n$ %s\n```\n\n... haha %s, very funny, but no thanks!", ev.Content[1:], ev.SenderEmail)
			case strings.HasPrefix(ev.Content, "!gif"):
				search := "elephant" // error elephant
				fs := strings.Fields(ev.Content)
				if len(fs) >= 2 {
					search = fs[1]
				}
				gifInfo, err := getJSON("https://api.giphy.com/v1/gifs/random?api_key=" + config.giphyAPIKey + "&tag=" + search)
				if err != nil {
					log.Println("random gif:", err)
					return
				}
				imageURL := gifInfo.(map[string]interface{})["data"].(map[string]interface{})["image_url"].(string)
				u, _ := url.Parse(imageURL)
				u.Scheme = "https"
				client.Replyf(ev, "here's some %s: %s", search, u)
			case strings.HasPrefix(ev.Content, "!godoc"):
				fs := strings.Fields(ev.Content)
				if len(fs) < 2 {
					return
				}
				client.Replyf(ev, "https://godoc.org/%s", fs[1])
			}
		case zulip.Heartbeat:
		default:
			log.Println("unhandled message")
		}
	})
}

var templateFuncs = template.FuncMap{
	"lines": func(s string) []string {
		return strings.Split(s, "\n")
	},
}
var pipelineTmpl = template.Must(template.New("").Funcs(templateFuncs).Parse(`{{ if eq .object_attributes.status "success" }}🎉{{ else }}⛈{{ end}} Build for {{ .project.name }} ("{{ index (lines .commit.message) 0 }}", {{ .object_attributes.ref }}) ran with status {{ .object_attributes.status }} (took {{ .object_attributes.duration }}s)`))

func pipelineServer(client *SimpleClient, addr string) {
	http.HandleFunc("/pipeline-status", func(w http.ResponseWriter, req *http.Request) {
		r := io.TeeReader(req.Body, os.Stdout)
		dec := json.NewDecoder(r)
		var v interface{}
		err := dec.Decode(&v)
		fmt.Println()
		if err != nil {
			log.Println("parsing pipeline status:", err)
			return
		}

		var buf bytes.Buffer
		err = pipelineTmpl.Execute(&buf, v)
		if err != nil {
			log.Println("rendering template:", err)
			return
		}

		status := findKey(v, "object_attributes", "status").(string)
		if status == "pending" || status == "running" {
			return
		}

		client.Send(zulip.Message{
			Type:    "stream",
			Stream:  "platform",
			Subject: findKey(v, "project", "name").(string),
			Content: buf.String(),
		})
	})
	err := http.ListenAndServe(addr, nil)
	if err != nil {
		log.Println("serve:", err)
	}
}

func findKey(val interface{}, keys ...interface{}) interface{} {
	for _, key := range keys {
		switch v := val.(type) {
		case map[string]interface{}:
			val = v[key.(string)]
		case []interface{}:
			val = v[key.(int)]
		default:
			log.Printf("error, unhandled nested value: %v (%T)\n", v, v)
		}
	}
	return val
}

func getJSON(url string) (interface{}, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	dec := json.NewDecoder(resp.Body)
	var res interface{}
	err = dec.Decode(&res)
	if err != nil {
		return nil, err
	}

	return res, nil
}
