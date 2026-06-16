package main

import "testing"

func TestExtractTraceChecksKeepsOnlyNamesAndStatuses(t *testing.T) {
	raw := []byte(`{
		"checks": [
			{"id": "unit_tests", "status": "PASS", "reason": "not emitted"},
			{"id": "evidence_gate", "status": "FAILED", "stderr": "not emitted"}
		],
		"events": [
			{"command_ref": "diff_whitespace", "outcome": "PASS"}
		]
	}`)

	checks := extractTraceChecks(raw)
	if len(checks) != 3 {
		t.Fatalf("checks = %d, want 3", len(checks))
	}
	if checks[0].Name != "unit_tests" || checks[0].Status != "PASS" {
		t.Fatalf("first check = %+v", checks[0])
	}
	if checks[1].Name != "evidence_gate" || checks[1].Status != "FAILED" {
		t.Fatalf("second check = %+v", checks[1])
	}
	if checks[2].Name != "diff_whitespace" || checks[2].Status != "PASS" {
		t.Fatalf("third check = %+v", checks[2])
	}
}

func TestStringListAcceptsRepeatedAndCommaSeparatedIssues(t *testing.T) {
	var values stringList
	if err := values.Set("C2-123,C2-142"); err != nil {
		t.Fatalf("Set returned error: %v", err)
	}
	if err := values.Set("C2-150"); err != nil {
		t.Fatalf("Set returned error: %v", err)
	}
	want := []string{"C2-123", "C2-142", "C2-150"}
	if len(values) != len(want) {
		t.Fatalf("len(values) = %d, want %d", len(values), len(want))
	}
	for i := range want {
		if values[i] != want[i] {
			t.Fatalf("values[%d] = %q, want %q", i, values[i], want[i])
		}
	}
}
