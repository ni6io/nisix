package toolpolicy

import "testing"

func TestPolicyAllowedSupportsExactAndWildcardEntries(t *testing.T) {
	policy := Policy{
		Allow: []string{"time_now", "mcp_filesystem_*"},
	}

	if !policy.Allowed("time_now") {
		t.Fatal("expected exact allow entry to pass")
	}
	if !policy.Allowed("mcp_filesystem_list_directory") {
		t.Fatal("expected wildcard allow entry to pass")
	}
	if policy.Allowed("shell") {
		t.Fatal("expected non-matching tool to stay blocked")
	}
}

func TestPolicyDenyWildcardOverridesAllow(t *testing.T) {
	policy := Policy{
		Allow: []string{"mcp_filesystem_*"},
		Deny:  []string{"mcp_filesystem_write_*"},
	}

	if !policy.Allowed("mcp_filesystem_read_file") {
		t.Fatal("expected read tool to remain allowed")
	}
	if policy.Allowed("mcp_filesystem_write_file") {
		t.Fatal("expected deny wildcard to override allow")
	}
}
