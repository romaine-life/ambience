package main

import (
	"html"
	"net/http"
	"strings"
)

const socialMetaPlaceholder = "<!-- __AMBIENCE_SOCIAL_META__ -->"

type socialPageMeta struct {
	Title       string
	Description string
	URL         string
	Image       string
}

func injectSocialMeta(body string, meta socialPageMeta) string {
	block := renderSocialMeta(meta)
	if strings.Contains(body, socialMetaPlaceholder) {
		return strings.Replace(body, socialMetaPlaceholder, block, 1)
	}
	if idx := strings.Index(strings.ToLower(body), "</head>"); idx >= 0 {
		return body[:idx] + block + "\n" + body[idx:]
	}
	return body
}

func renderSocialMeta(meta socialPageMeta) string {
	title := html.EscapeString(meta.Title)
	description := html.EscapeString(meta.Description)
	pageURL := html.EscapeString(meta.URL)
	imageURL := html.EscapeString(meta.Image)
	return strings.Join([]string{
		`<meta property="og:type" content="website">`,
		`<meta property="og:site_name" content="ambience">`,
		`<meta property="og:title" content="` + title + `">`,
		`<meta property="og:description" content="` + description + `">`,
		`<meta property="og:url" content="` + pageURL + `">`,
		`<meta property="og:image" content="` + imageURL + `">`,
		`<meta property="og:image:width" content="1200">`,
		`<meta property="og:image:height" content="630">`,
		`<meta property="og:image:type" content="image/png">`,
		`<meta name="twitter:card" content="summary_large_image">`,
		`<meta name="twitter:title" content="` + title + `">`,
		`<meta name="twitter:description" content="` + description + `">`,
		`<meta name="twitter:image" content="` + imageURL + `">`,
	}, "\n")
}

func devSocialTitle(effect string) string {
	return "ambience dev - " + effect
}

func devSocialDescription(effect string) string {
	return "Tune the " + effect + " ambience effect in an isolated browser session."
}

func effectSocialTitle(effect string) string {
	return "ambience effect - " + effect
}

func effectSocialDescription(effect string) string {
	return "Preview the " + effect + " ambience effect and open its live controls."
}

func absoluteRequestURL(req *http.Request, path, rawQuery string) string {
	scheme := req.Header.Get("X-Forwarded-Proto")
	if scheme == "" {
		scheme = "https"
		if req.TLS == nil && strings.HasPrefix(req.Host, "localhost") {
			scheme = "http"
		}
	}
	host := req.Host
	if forwardedHost := req.Header.Get("X-Forwarded-Host"); forwardedHost != "" {
		host = forwardedHost
	}
	if host == "" {
		host = "ambience.romaine.life"
	}
	out := scheme + "://" + host + path
	if rawQuery != "" {
		out += "?" + rawQuery
	}
	return out
}
