package notifications

import (
	"bytes"
	"encoding/xml"
	"strings"
	"unicode"
)

func sanitize(title, body string) (string, string) {
	clean := func(value string, max int) string {
		value = strings.Map(func(r rune) rune {
			if unicode.IsControl(r) && r != '\n' && r != '\t' {
				return -1
			}
			return r
		}, value)
		runes := []rune(strings.TrimSpace(value))
		if len(runes) > max {
			runes = runes[:max]
		}
		return string(runes)
	}
	title = clean(title, 120)
	if title == "" {
		title = "Netcatty"
	}
	return title, clean(body, 500)
}

func toastXML(title, body string) string {
	title, body = sanitize(title, body)
	var out bytes.Buffer
	out.WriteString(`<toast><visual><binding template="ToastGeneric"><text>`)
	_ = xml.EscapeText(&out, []byte(title))
	out.WriteString(`</text><text>`)
	_ = xml.EscapeText(&out, []byte(body))
	out.WriteString(`</text></binding></visual></toast>`)
	return out.String()
}
