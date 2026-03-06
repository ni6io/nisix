# TOOLS

## Registered Tools
- `time_now`: returns current server timestamp.
- `shell`: runs a short `/bin/sh -lc` command from the workspace root, with bounded output and timeout.
- MCP tools are loaded at startup from `mcp.json` with name pattern `mcp_<server>_<tool>`.
- Example (filesystem server): `mcp_filesystem_list_directory`, `mcp_filesystem_read_file`, `mcp_filesystem_write_file`.
- Enabled now (policy allowlist): `mcp_filesystem_*`.

## Tool Usage Rules
- Use tools only when they improve correctness or speed.
- Validate inputs before invoking a tool.
- Handle tool errors with concise, actionable messages.
- Do not expose secrets from tool output.
- `shell` may be registered but blocked by `tools.allow`; check `/tools list` before use.
- Keep `shell` commands short, workspace-scoped, and read-oriented unless the user explicitly asks for mutations.
- If a requested MCP tool is unavailable, ask user to run `/tools list` and verify `tools.allow` policy.
- Preferred first call for filesystem MCP: `mcp_filesystem_list_directory({"path":"."})`.

## Planned Extensions
- `web.search`: retrieve authoritative references.
- `file.read` and `file.write`: controlled workspace access.
- stronger sandboxing/audit for local command execution.
