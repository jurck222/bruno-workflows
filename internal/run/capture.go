package run

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

type Response struct {
	Status int
	Header http.Header
	Body   string
}

func Capture(expr string, resp *Response) (string, error) {
	switch {
	case expr == "status":
		return strconv.Itoa(resp.Status), nil

	case expr == "body":
		return resp.Body, nil

	case strings.HasPrefix(expr, "header."):
		name := strings.TrimPrefix(expr, "header.")
		return resp.Header.Get(name), nil

	case strings.HasPrefix(expr, "body."):
		path := strings.TrimPrefix(expr, "body.")
		var v any
		if err := json.Unmarshal([]byte(resp.Body), &v); err != nil {
			return "", fmt.Errorf("%s: response body is not valid JSON: %w", expr, err)
		}
		return walkJSON(v, strings.Split(path, "."), expr)

	default:
		return "", fmt.Errorf("invalid capture expression %q", expr)
	}
}

func walkJSON(v any, segs []string, expr string) (string, error) {
	for _, seg := range segs {
		switch node := v.(type) {
		case map[string]any:
			val, ok := node[seg]
			if !ok {
				return "", fmt.Errorf("%s: no key %q in object", expr, seg)
			}
			v = val
		case []any:
			idx, err := strconv.Atoi(seg)
			if err != nil || idx < 0 {
				return "", fmt.Errorf("%s: %q is not a valid array index", expr, seg)
			}
			if idx >= len(node) {
				return "", fmt.Errorf("%s: index %d out of range (len %d)", expr, idx, len(node))
			}
			v = node[idx]
		default:
			return "", fmt.Errorf("%s: cannot descend into %q on a scalar", expr, seg)
		}
	}
	return stringifyScalar(v, expr)
}

func stringifyScalar(v any, expr string) (string, error) {
	switch t := v.(type) {
	case nil:
		return "", nil
	case string:
		return t, nil
	case bool:
		return strconv.FormatBool(t), nil
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64), nil
	default:
		return "", fmt.Errorf("%s: resolves to an object or array, not a scalar", expr)
	}
}

func ValidateCaptureExpr(expr string) error {
	if expr == "status" || expr == "body" || strings.HasPrefix(expr, "header.") || strings.HasPrefix(expr, "body.") {
		if strings.HasPrefix(expr, "header.") && strings.TrimPrefix(expr, "header.") == "" {
			return fmt.Errorf("invalid capture expression %q: missing header name", expr)
		}
		if strings.HasPrefix(expr, "body.") && strings.TrimPrefix(expr, "body.") == "" {
			return fmt.Errorf("invalid capture expression %q: missing path", expr)
		}
		return nil
	}
	return fmt.Errorf("invalid capture expression %q", expr)
}
