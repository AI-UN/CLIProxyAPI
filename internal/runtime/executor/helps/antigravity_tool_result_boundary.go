package helps

import (
	"strconv"
	"strings"

	"github.com/MoeForge/moego/ojson"
)

// EnsureAntigravityGeminiToolResultBoundaryDoc marks a terminal pure tool-result
// turn as user content, naming the answered functions without repeating output.
// Historical turns stay unchanged so reasoning-replay context hashes still match.
func EnsureAntigravityGeminiToolResultBoundaryDoc(doc *ojson.Doc) bool {
	if doc == nil {
		return false
	}
	contents := doc.Get("request.contents")
	if !contents.IsArray() {
		return false
	}
	var last ojson.Result
	var lastIndex int64
	contents.ForEach(func(index, content ojson.Result) bool {
		lastIndex, last = index.Int(), content
		return true
	})
	if !last.Exists() {
		return false
	}
	if contentHasFunctionResponseMedia(last) {
		return false
	}
	parts := last.Get("parts")
	if !parts.IsArray() {
		return false
	}
	partArray := parts.Array()
	if len(partArray) == 0 {
		return false
	}

	labels := make([]string, 0, len(partArray))
	for _, part := range partArray {
		if part.Get("thought").Bool() || part.Get("functionCall").Exists() {
			return false
		}
		if response := part.Get("functionResponse"); response.Exists() {
			name := strings.TrimSpace(response.Get("name").String())
			if name == "" {
				return false
			}
			labels = append(labels, name)
			continue
		}
		if text := part.Get("text"); !text.Exists() || strings.TrimSpace(text.String()) != "" {
			return false
		}
	}
	if len(labels) == 0 {
		return false
	}

	path := "request.contents." + strconv.FormatInt(lastIndex, 10)
	if errSet := doc.Set(path+".role", "user"); errSet != nil {
		return false
	}
	return doc.Set(path+".parts.-1.text", strings.Join(labels, "\n")) == nil
}
