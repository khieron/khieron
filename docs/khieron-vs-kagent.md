# Khieron vs Kagent - Feature Comparison

A comparison of Khieron and Kagent for day 2 Kubernetes monitoring and agent-based operations.

Both are comparable in that the both allow Agent creation throgh CRDs, are focussed on SRE and are implemented in Go for a lightweight container.

| Aspect | Khieron | Kagent |
|--------|---------|--------|
| **Agent runtime** | Single controller binary (Go) runs all agents in-process. No child containers. ~74MB image. | Each agent gets its own Kubernetes pod (Python or Go ADK runtime). 1 controller + 1 UI + 1 DB + N agent pods + M tool server pods. |
| **Chat interface** | None by design — reduced attack surface, no prompt injection risk from interactive input. | Full Next.js web UI with streaming chat. Default `unsecure` auth mode has no authentication; `trusted-proxy` mode uses OIDC but network policies to prevent bypassing the proxy are still planned. |
| **Agent-per-skill model** | One agent per Skill CR, created dynamically from a ConfigMap. Agent exists only while the skill is active. | One persistent Deployment per Agent CR. Ships with 10 pre-configured agent pods. Skills in kagent are mounted instruction files, not separate agents. |
| **Execution model** | Periodic loop (default 5 min, configurable). Agent runs, completes, sleeps. No idle resource consumption between runs. | Long-running pods serving an A2A HTTP endpoint, waiting for messages. Always consuming cluster resources even when idle. Timeout defaults to unlimited for streaming. |
| **Task scope** | Each skill is a single-purpose loop with a narrow focus defined by a Skill.md file and its assets. | Agents are domain-scoped (k8s, helm, istio, cilium, etc.) with fine-grained tool whitelisting per agent (max 50 tools per MCP server reference). Broader scope per agent but constrained by tool selection. |
| **Tool support** | MCP servers via `MCPConfigRef` ConfigMap, internal Go tools for K8s API, external bash scripts in the Skill's `assets` folder. | MCP is the primary integration — `RemoteMCPServer` CRD with Streamable HTTP and SSE transports. Built-in `kagent-tools` MCP server. Agent-as-tool via A2A protocol. Built-in skill tools (read/write/edit/bash). |
| **Human in the loop** | Default for all skills — Advisories CRD. Agent raises an Advisory, human approves/rejects/comments, advisory resolves the issue. CRD-native workflow. | Opt-in per tool via `requireApproval` field. UI shows approve/reject buttons with rejection reasons. Also has `ask_user` tool for agent-initiated questions. Cascades through sub-agent A2A chains. Not on by default. |
| **Resource footprint** | Minimal — single pod for the operator, no database, no UI pod, no per-agent pods. | Heavier — controller pod + Postgres DB + UI pod + N agent pods + M tool server pods. Each agent is a separate deployment with its own service and secret. |
| **Observability** | OpenTelemetry traces and logs via ADK. Token usage tracked in Skill status. | Full chat history in Postgres. Token usage tracked. UI visualization of tool calls. |
| **Security model** | RBAC via per-skill ServiceAccounts. No interactive interface. Egress firewall on OpenShift restricts network access to model API only. | Per-agent ServiceAccounts. OIDC auth for UI (when configured). Cross-namespace tool access controlled via `AllowedNamespaces`. Header propagation for MCP auth. |
| **Model API support** | Gemini API (via Google ADK). OpenAI-compatible API support planned for self-hosted models. | 9 providers via Go ADK: Gemini, Gemini on Vertex AI, OpenAI, Azure OpenAI, Anthropic, Anthropic on Vertex AI, Ollama, AWS Bedrock (Converse API), and SAP AI Core. |

## Key Takeaway

Khieron optimizes for lightweight, periodic, autonomous monitoring with mandatory human approval gates — well suited for day 2 operations where you want narrow skills running continuously without interactive overhead. Kagent optimizes for interactive, multi-agent orchestration with a rich UI and flexible tool ecosystem — more capable but with a significantly larger deployment footprint and broader attack surface.
