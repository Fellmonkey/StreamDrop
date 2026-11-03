package engine

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"streamdrop/internal/apierr"
)

const maxRedirects = 10

// HeaderProvider applies authorization headers to an outgoing request.
//
// The interface is declared in the consumer package on purpose: engine then
// does not depend on auth and can be driven by any header source.
type HeaderProvider interface {
	Apply(req *http.Request)
}

// noHeaders is used when a probe runs without an auth profile.
type noHeaders struct{}

func (noHeaders) Apply(*http.Request) {}

// FileMetadata is the result of a pre-flight inspection.
type FileMetadata struct {
	OriginalURL   string `json:"original_url"`
	ResolvedURL   string `json:"resolved_url"`
	Filename      string `json:"filename"`
	SizeBytes     int64  `json:"size_bytes"`
	ContentType   string `json:"content_type"`
	IsHTML        bool   `json:"is_html"`
	SupportsRange bool   `json:"supports_range"`
	EngineHint    string `json:"engine_hint"` // "direct" | "yt-dlp"
}

// Inspect performs the full pre-flight cycle: manual redirects -> HEAD ->
// Range-GET when HEAD is refused.
func Inspect(ctx context.Context, c *http.Client, rawURL string, hdr HeaderProvider) (*FileMetadata, error) {
	if hdr == nil {
		hdr = noHeaders{}
	}
	resolved, err := resolveRedirects(ctx, c, rawURL, hdr)
	if err != nil {
		return nil, err
	}

	meta := &FileMetadata{OriginalURL: rawURL, ResolvedURL: resolved}

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, resolved, nil)
	if err != nil {
		return nil, apierr.Wrap(apierr.CodeInspectFailed, err, map[string]any{"url": rawURL})
	}
	hdr.Apply(req)

	resp, err := c.Do(req)
	if err != nil {
		return nil, apierr.Wrap(apierr.CodeInspectFailed, err, map[string]any{"url": rawURL})
	}
	defer resp.Body.Close()

	// HEAD is often refused: 405/501 mean it is not implemented, 403 means the
	// CDN blocks HEAD specifically. All of them fall back to Range-GET.
	if resp.StatusCode == http.StatusMethodNotAllowed ||
		resp.StatusCode == http.StatusNotImplemented ||
		resp.StatusCode == http.StatusForbidden {
		return inspectWithRangeGet(ctx, c, resolved, hdr, meta)
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, apierr.New(apierr.CodeAuthRequired, map[string]any{"url": rawURL})
	}
	if resp.StatusCode >= 400 {
		return nil, apierr.New(apierr.CodeInspectHTTPError, map[string]any{
			"status_code": resp.StatusCode,
			"url":         rawURL,
		})
	}

	fillMetaFromHeaders(meta, resp)
	if meta.Filename == "" {
		meta.Filename = filenameFromURL(resolved)
	}
	return meta, nil
}

// filenameFromURL is the fallback name source when Content-Disposition is
// absent: the last meaningful path segment, percent-decoded.
func filenameFromURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	segments := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i := len(segments) - 1; i >= 0; i-- {
		seg := segments[i]
		if seg == "" {
			continue
		}
		if decoded, err := url.PathUnescape(seg); err == nil {
			seg = decoded
		}
		return seg
	}
	return ""
}

// inspectWithRangeGet requests the first byte: a 206 response carries the full
// size in Content-Range while Content-Length is 1.
func inspectWithRangeGet(ctx context.Context, c *http.Client, resolved string, hdr HeaderProvider, meta *FileMetadata) (*FileMetadata, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, resolved, nil)
	if err != nil {
		return nil, apierr.Wrap(apierr.CodeInspectFailed, err, nil)
	}
	req.Header.Set("Range", "bytes=0-0")
	hdr.Apply(req)

	resp, err := c.Do(req)
	if err != nil {
		return nil, apierr.Wrap(apierr.CodeInspectFailed, err, map[string]any{"url": resolved})
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return nil, apierr.New(apierr.CodeAuthRequired, map[string]any{"url": resolved})
	case resp.StatusCode == http.StatusForbidden:
		return nil, apierr.New(apierr.CodeAuthRejected, map[string]any{"url": resolved})
	case resp.StatusCode >= 400:
		return nil, apierr.New(apierr.CodeInspectHTTPError, map[string]any{
			"status_code": resp.StatusCode,
			"url":         resolved,
		})
	}

	fillMetaFromHeaders(meta, resp)
	if meta.Filename == "" {
		meta.Filename = filenameFromURL(resolved)
	}

	// Content-Range wins: "bytes 0-0/123456" -> 123456.
	if cr := resp.Header.Get("Content-Range"); cr != "" {
		if total, ok := parseContentRangeTotal(cr); ok {
			meta.SizeBytes = total
			return meta, nil
		}
	}
	if resp.StatusCode != http.StatusPartialContent {
		// The server ignored Range and started a full stream; the length is
		// the real file size.
		if cl := resp.Header.Get("Content-Length"); cl != "" {
			if n, err := strconv.ParseInt(cl, 10, 64); err == nil {
				meta.SizeBytes = n
			}
		}
	}
	return meta, nil
}

// parseContentRangeTotal extracts the full size from Content-Range.
func parseContentRangeTotal(value string) (int64, bool) {
	idx := strings.LastIndex(value, "/")
	if idx < 0 {
		return 0, false
	}
	total := strings.TrimSpace(value[idx+1:])
	if total == "*" {
		return 0, false
	}
	n, err := strconv.ParseInt(total, 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

// fillMetaFromHeaders fills metadata from response headers.
func fillMetaFromHeaders(meta *FileMetadata, resp *http.Response) {
	meta.ContentType = resp.Header.Get("Content-Type")
	meta.IsHTML = strings.Contains(strings.ToLower(meta.ContentType), "text/html")
	meta.SupportsRange = strings.EqualFold(resp.Header.Get("Accept-Ranges"), "bytes") ||
		resp.StatusCode == http.StatusPartialContent
	meta.Filename = parseContentDisposition(resp.Header.Get("Content-Disposition"))
	if cl := resp.Header.Get("Content-Length"); cl != "" {
		if n, err := strconv.ParseInt(cl, 10, 64); err == nil {
			meta.SizeBytes = n
		}
	}
	if meta.IsHTML {
		meta.EngineHint = "yt-dlp"
	} else {
		meta.EngineHint = "direct"
	}
}

// resolveRedirects follows the redirect chain manually (at most 10 hops) so the
// headers always belong to the target file, not to the redirect page.
func resolveRedirects(ctx context.Context, c *http.Client, rawURL string, hdr HeaderProvider) (string, error) {
	current := rawURL
	for i := 0; i < maxRedirects; i++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodHead, current, nil)
		if err != nil {
			return "", apierr.Wrap(apierr.CodeInspectFailed, err, map[string]any{"url": current})
		}
		hdr.Apply(req)

		resp, err := c.Do(req)
		if err != nil {
			return "", apierr.Wrap(apierr.CodeInspectFailed, err, map[string]any{"url": current})
		}
		resp.Body.Close()

		switch resp.StatusCode {
		case http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther,
			http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
			loc := resp.Header.Get("Location")
			if loc == "" {
				return current, nil
			}
			next, err := resp.Location() // resolves relative Location values
			if err != nil {
				return "", apierr.Wrap(apierr.CodeInspectRedirectLimit, err, nil)
			}
			current = next.String()
			continue
		default:
			return current, nil
		}
	}
	return "", apierr.New(apierr.CodeInspectRedirectLimit, map[string]any{"max_redirects": maxRedirects})
}

// parseContentDisposition understands both RFC 5987 filename* (UTF-8, higher
// priority) and Latin-1 filename.
func parseContentDisposition(value string) string {
	if value == "" {
		return ""
	}
	parts := strings.Split(value, ";")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if strings.HasPrefix(strings.ToLower(p), "filename*=") {
			raw := strings.TrimSpace(p[len("filename*="):])
			// RFC 5987 value shape: charset''percent-encoded-name. The charset
			// marker is dropped and the name is percent-decoded; a plain value
			// without the marker still works.
			if idx := strings.Index(raw, "''"); idx >= 0 {
				raw = raw[idx+2:]
			}
			raw = strings.Trim(raw, `"`)
			if dec, err := url.PathUnescape(raw); err == nil {
				return dec
			}
			return raw
		}
	}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if strings.HasPrefix(strings.ToLower(p), "filename=") {
			name := strings.Trim(strings.TrimSpace(p[len("filename="):]), `"`)
			return transliterate(name)
		}
	}
	return ""
}

// transliterate turns a Latin-1 file name into safe ASCII: Cyrillic is
// transliterated, spaces and unsafe characters become underscores.
func transliterate(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.' || r == '-' || r == '_':
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('_')
		case r > 127:
			if t, ok := cyrillicTranslit[r]; ok {
				b.WriteString(t)
			}
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}

var cyrillicTranslit = map[rune]string{
	'а': "a", 'б': "b", 'в': "v", 'г': "g", 'д': "d", 'е': "e", 'ё': "e",
	'ж': "zh", 'з': "z", 'и': "i", 'й': "y", 'к': "k", 'л': "l", 'м': "m",
	'н': "n", 'о': "o", 'п': "p", 'р': "r", 'с': "s", 'т': "t", 'у': "u",
	'ф': "f", 'х': "h", 'ц': "ts", 'ч': "ch", 'ш': "sh", 'щ': "sch",
	'ъ': "", 'ы': "y", 'ь': "", 'э': "e", 'ю': "yu", 'я': "ya",
	'А': "A", 'Б': "B", 'В': "V", 'Г': "G", 'Д': "D", 'Е': "E", 'Ё': "E",
	'Ж': "Zh", 'З': "Z", 'И': "I", 'Й': "Y", 'К': "K", 'Л': "L", 'М': "M",
	'Н': "N", 'О': "O", 'П': "P", 'Р': "R", 'С': "S", 'Т': "T", 'У': "U",
	'Ф': "F", 'Х': "H", 'Ц': "Ts", 'Ч': "Ch", 'Ш': "Sh", 'Щ': "Sch",
	'Ъ': "", 'Ы': "Y", 'Ь': "", 'Э': "E", 'Ю': "Yu", 'Я': "Ya",
}

// NewInspectClient builds the HTTP client used by the inspector: redirects are
// disabled because they are followed manually.
func NewInspectClient() *http.Client {
	return &http.Client{
		Timeout: 0, // per-request context controls the deadline
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}
