package core

import "encoding/json"

// inspectionJSON removes JSONC trivia from the installed binary's formatted
// output for a read-only view. It never writes this representation to a source.
func inspectionJSON(src []byte) ([]byte, error) {
	out := make([]byte, 0, len(src))
	quoted, escaped := false, false
	for i := 0; i < len(src); i++ {
		c := src[i]
		if quoted {
			out = append(out, c)
			if escaped {
				escaped = false
			} else if c == '\\' {
				escaped = true
			} else if c == '"' {
				quoted = false
			}
			continue
		}
		if c == '"' {
			quoted = true
			out = append(out, c)
			continue
		}
		if c == '/' && i+1 < len(src) {
			if src[i+1] == '/' {
				i += 2
				for i < len(src) && src[i] != '\n' {
					i++
				}
				out = append(out, '\n')
				continue
			}
			if src[i+1] == '*' {
				i += 2
				for i+1 < len(src) && !(src[i] == '*' && src[i+1] == '/') {
					i++
				}
				i++
				out = append(out, ' ')
				continue
			}
		}
		out = append(out, c)
	}
	// JSONC accepted by the installed parser can also contain trailing commas.
	cleaned := make([]byte, 0, len(out))
	quoted = false
	escaped = false
	for i, c := range out {
		if quoted {
			cleaned = append(cleaned, c)
			if escaped {
				escaped = false
			} else if c == '\\' {
				escaped = true
			} else if c == '"' {
				quoted = false
			}
			continue
		}
		if c == '"' {
			quoted = true
		}
		if c == ',' {
			j := i + 1
			for j < len(out) && (out[j] == ' ' || out[j] == '\n' || out[j] == '\r' || out[j] == '\t') {
				j++
			}
			if j < len(out) && (out[j] == ']' || out[j] == '}') {
				continue
			}
		}
		cleaned = append(cleaned, c)
	}
	var target any
	if err := json.Unmarshal(cleaned, &target); err != nil {
		return nil, err
	}
	return cleaned, nil
}
