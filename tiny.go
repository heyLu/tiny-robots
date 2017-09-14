package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"strings"

	"github.com/go-yaml/yaml"

	"github.com/heyLu/tiny-robots/zulip"
)

type cfg struct {
	Endpoint      string `yaml:"endpoint"`
	BotEmail      string `yaml:"bot-email"`
	GiphyAPIKey   string `yaml:"giphy"`
	GitLabAPIKey  string `yaml:"gitlab"`
	GitLabBaseURL string `yaml:"gitlab-url"`
}

var config cfg
var configPath string

func init() {
	flag.StringVar(&config.Endpoint, "endpoint", "https://chat.zulip.org", "The URL of the Zulip instance")
	flag.StringVar(&config.BotEmail, "bot", "tiny-bot@chat.zulip.org", "The email address of the bot")
	flag.StringVar(&config.GiphyAPIKey, "giphy", "", "The API key for Giphy")

	flag.StringVar(&configPath, "config", "", "The path to the config file (flags will be ignored)")
}

func main() {
	flag.Parse()

	if configPath != "" {
		err := readConfig(configPath, &config)
		if err != nil {
			log.Fatal("reading config: ", err)
		}
	}

	client, err := New(config.Endpoint, config.BotEmail, "api_key.txt")
	if err != nil {
		log.Println("creating client:", err)
	}

	go pipelineServer(client, ":12001")

	client.OnEachMessage(func(ev Message) {
		switch {
		case strings.HasPrefix(ev.Content(), "!hi"):
			client.Replyf(ev, "%s said hi!", ev.Author())
		case strings.HasPrefix(ev.Content(), "!failed"):
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
		case strings.HasPrefix(ev.Content(), "!rm") || strings.HasPrefix(ev.Content(), "!sh"):
			client.Replyf(ev, "```\n$ %s\n```\n\n... haha %s, very funny, but no thanks!", ev.Content()[1:], ev.Author())
		case strings.HasPrefix(ev.Content(), "!gif"):
			search := "elephant" // error elephant
			fs := strings.Fields(ev.Content())
			if len(fs) >= 2 {
				search = fs[1]
			}
			gifInfo, err := getJSON("https://api.giphy.com/v1/gifs/random?api_key=" + config.GiphyAPIKey + "&tag=" + search)
			if err != nil {
				log.Println("random gif:", err)
				return
			}
			imageURL := gifInfo.(map[string]interface{})["data"].(map[string]interface{})["image_url"].(string)
			u, _ := url.Parse(imageURL)
			u.Scheme = "https"
			client.Replyf(ev, "here's some %s: %s", search, u)
		case strings.HasPrefix(ev.Content(), "!godoc"):
			fs := strings.Fields(ev.Content())
			if len(fs) < 2 {
				return
			}
			client.Replyf(ev, "https://godoc.org/%s", fs[1])
		case strings.HasPrefix(ev.Content(), "!test"):
			fs := strings.Fields(ev.Content())
			if len(fs) < 3 {
				client.Reply(ev, "usage: !test <project> <branch-or-ref>")
				return
			}
			project := fs[1]
			projectPath := path.Join("projects", project)
			ref := fs[2]

			var buf bytes.Buffer
			cmd := exec.Command("git", "-C", projectPath, "fetch")
			cmd.Stdout = io.MultiWriter(&buf, os.Stdout)
			cmd.Stderr = io.MultiWriter(&buf, os.Stderr)
			err := cmd.Run()
			if err != nil {
				log.Println("git fetch:", err)
				client.Replyf(ev, "git fetch: %q", strings.TrimSpace(buf.String()))
				return
			}

			cmd = exec.Command("git", "-C", projectPath, "checkout", ref)
			err = cmd.Run()
			if err != nil {
				log.Println("git checkout:", ref, err)
				client.Replyf(ev, "no such branch: %s", ref)
				return
			}

			buf.Reset()
			cmd = exec.Command("/bin/sh", "-c", fmt.Sprintf("cd %q && make test", projectPath))
			cmd.Stdout = io.MultiWriter(&buf, os.Stdout)
			cmd.Stderr = io.MultiWriter(&buf, os.Stderr)
			err = cmd.Run()
			emoji := "🎉"
			if err != nil {
				log.Println("make:", err)
				emoji = "⛈"
			}
			client.Replyf(ev, "%s\n```\n%s\n```", emoji, buf.String())
		case strings.HasPrefix(ev.Content(), "!ci"):
			fs := strings.Fields(ev.Content())
			if len(fs) < 2 {
				client.Reply(ev, "usage: !ci <project> [<branch-or-ref>]")
				return
			}
			project := fs[1]
			ref := "master"
			if len(fs) >= 3 {
				ref = fs[2]
			}

			req, err := http.NewRequest("POST", config.GitLabBaseURL+"/api/v4/projects/"+url.PathEscape(project)+"/pipeline", nil)
			if err != nil {
				log.Println("creating request:", err)
				return
			}
			q := req.URL.Query()
			q.Set("ref", ref)
			req.URL.RawQuery = q.Encode()
			req.Header.Set("Private-Token", config.GitLabAPIKey)

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				log.Println("gitlab request:", err)
				return
			}
			defer resp.Body.Close()

			var v interface{}
			dec := json.NewDecoder(resp.Body)
			err = dec.Decode(&v)
			if err != nil {
				log.Println("gitlab parse:", err)
				return
			}

			if resp.StatusCode >= 400 {
				log.Printf("gitlab error: %#v\n", v)
				client.Replyf(ev, "Could not start pipeline for %q: %s", project, findKey(v, "message"))
				return
			}

			id := findKey(v, "id").(float64)
			status := findKey(v, "status").(string)
			client.Replyf(ev, "Pipeline %s for %s@%s (%s/%s/pipelines/%0.f)", status, project, ref, config.GitLabBaseURL, project, id)
		}
	})
}

var templateFuncs = template.FuncMap{
	"lines": func(s string) []string {
		return strings.Split(s, "\n")
	},
}
var pipelineTmpl = template.Must(template.New("").Funcs(templateFuncs).Parse(`{{ if eq .object_attributes.status "success" }}🎉{{ else }}⛈{{ end}} Build for {{ .project.name }} ("{{ index (lines .commit.message) 0 }}", {{ .object_attributes.ref }}) ran with status {{ .object_attributes.status }} (took {{ .object_attributes.duration }}s)`))

func pipelineServer(client Client, addr string) {
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
			Type:       "stream",
			Stream:     "platform",
			Subject:    findKey(v, "project", "name").(string),
			RawContent: buf.String(),
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

func readConfig(p string, config *cfg) error {
	data, err := ioutil.ReadFile(p)
	if err != nil {
		return err
	}

	err = yaml.Unmarshal(data, config)
	if err != nil {
		return err
	}

	return nil
}
