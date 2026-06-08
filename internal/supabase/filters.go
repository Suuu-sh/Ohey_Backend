package supabase

import "strings"

func PostgRESTEq(value string) string {
	return "eq." + strings.TrimSpace(value)
}

func PostgRESTIn(values ...string) string {
	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		cleaned = append(cleaned, trimmed)
	}
	return "in.(" + strings.Join(cleaned, ",") + ")"
}
