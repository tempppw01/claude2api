package service

import "testing"

func TestParseImportSessionCandidatesPreservesCookieSemicolons(t *testing.T) {
	req := ImportSessionsRequest{
		Sessions: "sk-ant-sid-test|org-test|cf-test|sessionKey=value; other=value",
	}

	candidates := parseImportSessionCandidates(req)
	if len(candidates) != 1 {
		t.Fatalf("expected one candidate, got %d", len(candidates))
	}

	session := candidates[0]
	if session.SessionKey != "sk-ant-sid-test" {
		t.Fatalf("expected session key to be parsed, got %q", session.SessionKey)
	}
	if session.OrgID != "org-test" {
		t.Fatalf("expected org id to be parsed, got %q", session.OrgID)
	}
	if session.CFClearance != "cf-test" {
		t.Fatalf("expected cf clearance to be parsed, got %q", session.CFClearance)
	}
	if session.CookieString != "sessionKey=value; other=value" {
		t.Fatalf("expected cookie string to preserve semicolons, got %q", session.CookieString)
	}
}
