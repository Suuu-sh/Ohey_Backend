package supabase

import "testing"

func TestPostgRESTEq(t *testing.T) {
	if got := PostgRESTEq(" pending "); got != "eq.pending" {
		t.Fatalf("PostgRESTEq = %q", got)
	}
}

func TestPostgRESTIn(t *testing.T) {
	if got := PostgRESTIn(" pending ", "", "accepted"); got != "in.(pending,accepted)" {
		t.Fatalf("PostgRESTIn = %q", got)
	}
}
