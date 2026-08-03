# Model Deployment Monitor — Reference

This skill monitors RHOAI model deployments served by vLLM, using the rhoai-mcp MCP server and Prometheus metrics.

## Deployment Context

The file `references/SKILL-CONTEXT.md` is injected at deploy time via the Helm chart's `extraFiles` value. It should contain the model name, namespace, resource allocation, and performance targets (TTFT, ITL, E2E, QPS) from the deployment recommendation. Without this file the skill cannot operate.

## RHOAI MCP Tools Used

- `get_inference_service` — check InferenceService readiness, replica status, and configuration

## Scripts

- `scripts/check-vllm-metrics.sh` — queries Prometheus for vLLM serving metrics (TTFT, ITL, E2E latency percentiles, queue depth, KV cache usage, preemptions, request rates)
- `scripts/check-gpu-metrics.sh` — queries Prometheus for DCGM GPU metrics (utilization, memory, temperature, framebuffer)
- `scripts/check-model-endpoint.sh` — health-checks the model's OpenAI-compatible `/v1/models` endpoint

## vLLM Prometheus Metrics

| Metric | Description |
|--------|-------------|
| `vllm:time_to_first_token_seconds` | Histogram — time from request receipt to first generated token |
| `vllm:time_per_output_token_seconds` | Histogram — inter-token latency during generation |
| `vllm:e2e_request_latency_seconds` | Histogram — total request latency including queuing |
| `vllm:num_requests_waiting` | Gauge — requests queued waiting for processing |
| `vllm:num_requests_running` | Gauge — requests currently being processed |
| `vllm:gpu_cache_usage_perc` | Gauge — KV cache utilization (0–1) |
| `vllm:num_preemptions_total` | Counter — requests evicted from KV cache under memory pressure |
| `vllm:request_success_total` | Counter — successful inference requests |
| `vllm:request_failure_total` | Counter — failed inference requests |
| `vllm:generation_tokens_total` | Counter — total tokens generated |

## DCGM Prometheus Metrics

| Metric | Description |
|--------|-------------|
| `DCGM_FI_DEV_GPU_UTIL` | GPU compute utilization (%) |
| `DCGM_FI_DEV_MEM_COPY_UTIL` | GPU memory bandwidth utilization (%) |
| `DCGM_FI_DEV_GPU_TEMP` | GPU temperature (°C) |
| `DCGM_FI_DEV_FB_USED` | GPU framebuffer memory used (MB) |
| `DCGM_FI_DEV_FB_FREE` | GPU framebuffer memory free (MB) |

## Advisory Thresholds

These are guidelines for the agent, not hard-coded rules. The agent should compare observed metrics against the performance targets in `SKILL-CONTEXT.md`:

| Issue | Signal | Typical threshold |
|-------|--------|-------------------|
| TTFT degraded | p95 TTFT vs target | Exceeds target by >50% |
| ITL degraded | p95 ITL vs target | Exceeds target by >50% |
| KV cache pressure | KV cache utilization | Above 90% |
| Request queue saturated | Sustained requests waiting | Queue depth >0 for multiple cycles |
| GPU memory critical | DCGM framebuffer used | Above 95% of total |
| GPU thermal throttling | GPU temperature | Above 85°C |
| Request failures | Failure rate | Above 1% of total requests |
| Low GPU utilization | DCGM GPU util on active model | Below 10% (model may not be loaded) |
