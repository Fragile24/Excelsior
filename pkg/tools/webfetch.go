package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"excelsior/pkg/util"
	"golang.org/x/net/html"
)

type WebFetchTool struct{}

func (t *WebFetchTool) Name() string   { return "web_fetch" }
func (t *WebFetchTool) ReadOnly() bool { return true }
func (t *WebFetchTool) Description() string {
	return "Fetch a concrete HTTP(S) URL. HTML becomes readable markdown by default; format may be markdown, text, or html. Text and JSON pass through. max_bytes defaults to 2MB, maximum 10MB; oversized responses are flagged. Returns final URL and content type. Binary responses return metadata. HTTP/network errors retain a short response preview. Read-only; no web search."
}
func (t *WebFetchTool) Parameters() any {
	return jsonSchema(map[string]any{
		"url":       map[string]any{"type": "string", "pattern": "^https?://"},
		"format":    map[string]any{"type": "string", "enum": []string{"markdown", "text", "html"}, "default": "markdown"},
		"max_bytes": map[string]any{"type": "integer", "minimum": 1, "maximum": 10 << 20, "default": 2 << 20},
	}, []string{"url"})
}
func (t *WebFetchTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var a struct {
		URL      string `json:"url"`
		Format   string `json:"format"`
		MaxBytes int    `json:"max_bytes"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return "", errf("web_fetch", "validate", "", fmt.Errorf("%w: %v", ErrInvalidArguments, err))
	}
	parsed, err := url.Parse(a.URL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", errf("web_fetch", "validate", a.URL, fmt.Errorf("%w: URL must use http or https", ErrInvalidArguments))
	}
	if a.Format == "" {
		a.Format = "markdown"
	}
	if a.Format != "markdown" && a.Format != "text" && a.Format != "html" {
		return "", errf("web_fetch", "validate", "", fmt.Errorf("%w: format must be markdown, text, or html", ErrInvalidArguments))
	}
	if a.MaxBytes == 0 {
		a.MaxBytes = 2 << 20
	}
	if a.MaxBytes < 1 || a.MaxBytes > 10<<20 {
		return "", errf("web_fetch", "validate", "", fmt.Errorf("%w: max_bytes must be 1..10485760", ErrInvalidArguments))
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", a.URL, nil)
	if err != nil {
		return "", errf("web_fetch", "fetch", a.URL, err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", errf("web_fetch", "fetch", a.URL, err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, int64(a.MaxBytes)+1))
	if err != nil {
		return "", errf("web_fetch", "fetch", a.URL, err)
	}
	truncated := len(b) > a.MaxBytes
	b = b[:min(len(b), a.MaxBytes)]
	contentType, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if contentType == "" {
		contentType, _, _ = mime.ParseMediaType(http.DetectContentType(b))
	}
	header := fmt.Sprintf("final_url: %s\nstatus: %d\ncontent_type: %s\ntruncated: %t\n\n", resp.Request.URL, resp.StatusCode, contentType, truncated)
	text := strings.ToValidUTF8(string(b), "�")
	if resp.StatusCode >= 400 {
		return header + util.Truncate(text, 500), errf("web_fetch", "fetch", a.URL, fmt.Errorf("HTTP %d", resp.StatusCode))
	}
	isHTML := contentType == "text/html" || contentType == "application/xhtml+xml"
	readable := strings.HasPrefix(contentType, "text/") || strings.Contains(contentType, "json") || strings.Contains(contentType, "xml") || contentType == "application/javascript"
	if !readable {
		return header + fmt.Sprintf("Binary response: %d bytes captured. Use bash for byte access.", len(b)), nil
	}
	if isHTML && a.Format != "html" {
		text, err = readableHTML(text, a.Format == "markdown", resp.Request.URL)
		if err != nil {
			return "", errf("web_fetch", "parse", a.URL, err)
		}
	}
	return header + text, nil
}

// ponytail: covers documentation structure, links and code; this is not a
// browser renderer or a general purpose HTML-to-Markdown library.
func readableHTML(source string, markdown bool, base *url.URL) (string, error) {
	doc, err := html.Parse(strings.NewReader(source))
	if err != nil {
		return "", err
	}
	var out strings.Builder
	var render func(*html.Node, bool)
	render = func(node *html.Node, pre bool) {
		if node.Type == html.TextNode {
			if pre {
				out.WriteString(node.Data)
			} else {
				// Collapse HTML whitespace while preserving spaces around inline nodes.
				text := strings.Join(strings.Fields(node.Data), " ")
				if len(node.Data) > 0 && strings.ContainsAny(node.Data[:1], " \n\t\r") {
					out.WriteByte(' ')
				}
				out.WriteString(text)
				if len(node.Data) > 0 && strings.ContainsAny(node.Data[len(node.Data)-1:], " \n\t\r") {
					out.WriteByte(' ')
				}
			}
			return
		}
		tag := node.Data
		if node.Type == html.ElementNode {
			switch tag {
			case "head", "script", "style", "svg", "noscript", "template":
				return
			}
		}
		before, after := "", ""
		switch tag {
		case "p", "div", "section", "article", "ul", "ol", "table", "blockquote":
			before, after = "\n\n", "\n\n"
		case "br":
			after = "\n"
		case "tr":
			before, after = "\n", "\n"
		case "td", "th":
			after = " | "
		case "li":
			before, after = "\n- ", "\n"
		case "h1", "h2", "h3", "h4", "h5", "h6":
			before, after = "\n\n", "\n\n"
			if markdown {
				before += strings.Repeat("#", int(tag[1]-'0')) + " "
			}
		case "pre":
			before, after = "\n\n", "\n\n"
			if markdown {
				before += "```\n"
				after = "\n```\n\n"
			}
		case "code":
			if markdown && !pre {
				before, after = "`", "`"
			}
		case "strong", "b":
			if markdown {
				before, after = "**", "**"
			}
		case "em", "i":
			if markdown {
				before, after = "*", "*"
			}
		case "a":
			if markdown {
				for _, attr := range node.Attr {
					if attr.Key == "href" {
						if href, err := url.Parse(attr.Val); err == nil {
							before, after = "[", "]("+base.ResolveReference(href).String()+")"
						}
					}
				}
			}
		}
		out.WriteString(before)
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			render(child, pre || tag == "pre")
		}
		out.WriteString(after)
	}
	render(doc, false)
	return strings.TrimSpace(out.String()), nil
}
