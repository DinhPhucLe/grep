package main

import "testing"

func TestThreadApprovalPolicyKeepsSandbox(t *testing.T) {
	for _, policy := range []string{"", "on-request", "never"} {
		params, err := threadStartParams(t.TempDir(), policy)
		if err != nil {
			t.Fatal(err)
		}
		want := policy
		if want == "" {
			want = "on-request"
		}
		if params["approvalPolicy"] != want || params["sandbox"] != "workspace-write" {
			t.Fatalf("unexpected permissions: %+v", params)
		}
	}
	if _, err := threadStartParams(".", "untrusted"); err == nil {
		t.Fatal("unsupported policy accepted")
	}
}
