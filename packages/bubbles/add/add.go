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
)

var (
	GITHUB_CONTENTS_API_URL = fmt.Sprintf("%s/repos/%s/%s/contents", GITHUB_API_URL, OWNER, REPO)
	GITHUB_PAT              = os.Getenv("GH_PAT")
	OWNER                   = os.Getenv("OWNER")
	REPO                    = os.Getenv("REPO")
	TOKEN                   = os.Getenv("TOKEN")
	NotFoundError           = errors.New("File not found")
)

type GitHubContent struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Sha     string `json:"sha"`
	Content string `json:"content"`
}

type Commit struct {
	Message string `json:"message"`
	Content string `json:"content"`
	Sha     string `json:"sha"`
}

type Highlight struct {
	host  string
	path  string
	title string
	text  string
	url   string
}

type Response struct {
	StatusCode int               `json:"statusCode,omitempty"`
	Headers    map[string]string `json:"headers,omitempty"`
	Body       string            `json:"body,omitempty"`
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
		host:  host,
		path:  path,
		title: title,
		text:  text,
		url:   url,
	}, nil
}

func pathFromHighlight(h Highlight) string {
	return fmt.Sprintf("%s/%s.md", url.PathEscape(h.host), url.PathEscape(h.title))
}

func simpleAuth(t string) bool {
	return t == TOKEN
}

// getTag creates a reverse tag from a host name
// blog.example.com will result in example
func getTag(host string) string {
	parts := strings.Split(host, ".")
	if len(parts) > 1 {
		return parts[len(parts)-2]
	}
	return parts[0]
}

func Main(args map[string]interface{}) (*Response, error) {
	if !simpleAuth(args["token"].(string)) {
		return &Response{
			StatusCode: http.StatusForbidden,
		}, nil
	}
	highlight, err := newHighlight(args)
	if err != nil {
		log.Printf("Received Bad Request %v", err)
		return &Response{
			StatusCode: http.StatusBadRequest,
		}, err
	}

	client := &http.Client{}
	page, err := getFile(pathFromHighlight(highlight), client)
	if err != nil {
		if err != NotFoundError {
			log.Printf("failed to get file, err %v", err)
			return &Response{
				StatusCode: http.StatusInternalServerError,
			}, err
		}
		// create new page and add tag #domain/subdomain
		page.Content = fmt.Sprintf("# [%s](%s)\n#%s\n", highlight.title, highlight.url, getTag(highlight.host))
	}
	page.Content += fmt.Sprintf("\n---\n\n%s\n", highlight.text)
	err = commit(Commit{
		Content: base64.StdEncoding.EncodeToString([]byte(page.Content)),
		Message: fmt.Sprintf("add new highlight from %s", highlight.host),
		Sha:     page.Sha,
	}, pathFromHighlight(highlight), client)
	if err != nil {
		log.Println("failed to commit content")
		return &Response{
			StatusCode: http.StatusInternalServerError,
		}, err
	}
	return &Response{
		StatusCode: http.StatusCreated,
	}, nil
}

func getFile(path string, client *http.Client) (GitHubContent, error) {
	var page GitHubContent
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/%s", GITHUB_CONTENTS_API_URL, path), nil)
	req.Header.Add("Accept", `application/vnd.github+json`)
	req.Header.Add("Authorization", fmt.Sprintf("token %s", GITHUB_PAT))
	resp, err := client.Do(req)
	if err != nil {
		return GitHubContent{}, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return GitHubContent{}, NotFoundError
	}
	if resp.StatusCode >= 400 {
		return GitHubContent{}, fmt.Errorf("Something went wrong: %v", resp)
	}
	err = json.NewDecoder(resp.Body).Decode(&page)
	if err != nil {
		return GitHubContent{}, err
	}
	defer resp.Body.Close()
	content, err := base64.StdEncoding.DecodeString(page.Content)
	if err != nil {
		return page, err
	}
	page.Content = string(content)
	return page, nil
}

func commit(commit Commit, path string, client *http.Client) error {
	c, err := json.Marshal(commit)
	req, err := http.NewRequest(http.MethodPut, fmt.Sprintf("%s/%s", GITHUB_CONTENTS_API_URL, path), bytes.NewReader(c))
	if err != nil {
		return err
	}
	req.Header.Add("Accept", "application/vnd.github+json")
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", fmt.Sprintf("token %s", GITHUB_PAT))
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("Something went wrong: %v", resp)
	}
	return nil
}
