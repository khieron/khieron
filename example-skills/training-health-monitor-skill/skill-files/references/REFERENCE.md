# Training Health Monitor — Reference

This skill monitors RHOAI training jobs using the rhoai-mcp MCP server.

## RHOAI MCP Tools Used

- `list_training_jobs` — discover all training jobs and their status
- `get_training_progress` — retrieve loss, throughput, epoch, step, ETA
- `manage_checkpoints` — check checkpoint status and recency
- `get_job_events` — retrieve Kubernetes events (OOM, scheduling, image pull)
- `analyze_training_failure` — diagnose why a failed job failed

## Advisory Thresholds

These are guidelines for the agent, not hard-coded rules:

| Issue | Signal | Typical threshold |
|-------|--------|-------------------|
| Loss plateau | Loss stable across epochs | No decrease over 3+ epochs |
| Loss divergence | Loss increasing | Sustained increase over 2+ epochs |
| Low GPU utilization | DCGM metrics | Below 30% |
| High GPU memory | DCGM metrics | Above 95% |
| Stale checkpoints | Checkpoint step vs current step | Large gap relative to total steps |
| No checkpoints | No checkpoint data | Any job with 100+ steps and no checkpoint |

## Scripts

- `scripts/check-gpu-utilization.sh` — queries Prometheus/Thanos for DCGM GPU metrics
