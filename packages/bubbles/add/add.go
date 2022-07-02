package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
)

const (
	GITHUB_API_URL = "https://api.github.com"
	OWNER = "mo-rieger"
	REPO="foambubble-highlights"
)
var (
	GITHUB_PAT = os.Getenv("GH_PAT")
	NotFoundError = errors.New("File not found")
)

type GitHubContent struct {
	name string
	path string
	sha string
	content string
}

type Commit struct {
	message string
	content string
	sha string
}

type Highlight struct {
	host string
	path string
	title string
	text string
	url string
}

func newHighlight(args map[string]interface{}) (Highlight, error) {
	path, ok := args["path"].(string)
	if !ok {
		return Highlight{}, errors.New("missing path")
	}
	text, ok := args["text"].(string)
	if !ok {
		return Highlight{}, errors.New("missing text")
	}

	host, ok := args["host"].(string)
	if !ok {
		return Highlight{}, errors.New("missing host")
	}
	url, ok := args["url"].(string)
	if !ok {
		return Highlight{}, errors.New("missing url")
	}
	title, ok := args["title"].(string)
	if !ok {
		title = strings.ReplaceAll(path, "/", "-")
	}
	return Highlight{
		host: host,
		path: path,
		title: title,
		text: text,
		url: url,
	}, nil
}

func pathFromHighlight(h Highlight) string {
	return url.PathEscape(fmt.Sprintf("%s/%s.md", h.host,h.title))
}

func Main(args map[string]interface{}) map[string]interface{} {
	highlight, err := newHighlight(args)
	if err != nil {
		log.Fatalf("Bad Request %v", err)
	}
	
	client := &http.Client{}
	
	page, err := getFile(pathFromHighlight(highlight), client)
	if err != nil {
		if err != NotFoundError {
			log.Fatalf("failed to get file, err %v", err)
		}
		// create new page and add tag #host/title for better searchability in foam
		page.content = fmt.Sprintf("# [%s](%s)\n#%s/%s\n", highlight.title, highlight.url, highlight.host,strings.ReplaceAll(highlight.title, " ", "-"))
	}
	page.content += fmt.Sprintf("\n---\n\n%s\n" ,highlight.text)
	err = commit(Commit{
		content: base64.StdEncoding.EncodeToString([]byte(page.content)),
		message: fmt.Sprintf("add new highlight from %s", highlight.host),
		sha:     page.sha,
	}, pathFromHighlight(highlight), client)
	if err != nil {
		log.Println("failed to commit content")
	}
	msg := make(map[string]interface{})
	msg["body"] = fmt.Sprintf("add new highlight from %s", highlight.host)
	return msg
}

func getFile(path string, client *http.Client) (GitHubContent, error) {
	var page GitHubContent
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/repos/%s/%s/%s", GITHUB_API_URL, OWNER, REPO, path), nil)
	req.Header.Add("Accept", `application/vnd.github+json`)
	req.Header.Add("Authorization", fmt.Sprintf("token %s", GITHUB_PAT))
	resp, err := client.Do(req)
	if err != nil {
		if resp.StatusCode == http.StatusNotFound {
			return GitHubContent{}, NotFoundError
		}
		return GitHubContent{}, err
	}

	err = json.NewDecoder(resp.Body).Decode(&page)
	if err != nil {
		log.Printf("cannot encode file from GitHub, err: %v", err)
		return GitHubContent{}, err
	}
	defer resp.Body.Close()
	content, err := base64.StdEncoding.DecodeString(page.content)
	if err != nil {
		return page, err
	}
	page.content = string(content)
	return page, nil
}

func commit(commit Commit, path string, client *http.Client) error {
	var buf bytes.Buffer
	err := json.NewEncoder(&buf).Encode(commit)

	req, err := http.NewRequest(http.MethodPut, fmt.Sprintf("%s/repos/%s/%s/%s", GITHUB_API_URL, OWNER, REPO, path), &buf)
	if err != nil {
		return err
	}
	req.Header.Add("Accept", "application/vnd.github+json")
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", fmt.Sprintf("token %s", GITHUB_PAT))
	_, err = client.Do(req)
	if err != nil {
		return err
	}
	return nil
}
