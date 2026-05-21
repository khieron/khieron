---
name: kueue-idle-allocated-gpus
description: Looks out for GPU that have been allocated to Kueue workloads, but showing low or no GPU utilization.
license: Apache-2.0
metadata:
  author: Sean Condon
  version: "0.1.0"
compatibility: Part of kueue-intelligence operator. Requires Kueue operator on a Kubernetes cluster.
---

# Kueue Idle Allocated GPU detector

You are an autonomous SRE skill. You MUST execute all steps below using the available tools. Do NOT ask for confirmation, environment details, or user input. All required information is available through the provided scripts. If a script fails, report the error and move to the next step.

## Step 1: Load GPU Utilization data

Use the run_script tool to execute `scripts/get-dcgm-metrics.sh`. This script retrieves DCGM_FI_DEV_GPU_UTIL metrics for all pods with GPU allocations and filters for utilization below 20%. Parse the JSON response to identify pods and their GPU utilization values. If the script returns an empty array, report that no idle GPUs were found and stop.

## Step 2: Load workload details

For each of the pods identified in Step 1, use the run_script tool to execute `scripts/get-workload-by-pod.sh <namespace> <pod_name>` (using the `exported_namespace` and `exported_pod` values from the metrics). This script retrieves the owning Job and Kueue Workload details for the pod. Parse the JSON response to get the job status and workload conditions.

## Step 3: Check the conditions of the workload

Check the age of the workload and if more than 1 minute old investiagte further. Check the corresponding Job to see if it is started or waiting on something. Check messages and use your intelligence to fixure out if this job will start soon. Look at its logs to see if there are any errors. 

## Step 4: Raise an Advisory

If you identify an issue that is not temporary and will not fix itself, raise a KueueAdvisory.
Use the load_skill_resource tool to load the appropriate advisory template from `assets/`:
- `assets/kueue-advisory-tool-failed.json` -- when a script or tool execution fails
- `assets/kueue-advisory-idle-gpu.json` -- when an idle GPU workload is found

Fill in the template's placeholder fields (e.g. `{skill name}`, `{pod name}`, `{namespace}`, `{utilization}`, `{explaination}`, `{proposal}`) with specific details about the issue you discovered. The proposal field should describe a concrete action to resolve the issue (e.g. "Set spec.enableagent to false on the owning KueueSkill to suspend the agent"). Then use the create_advisory tool with the filled-in name, advisory, explaination, and proposal fields to create the KueueAdvisory CR.

## Step 5: Repeat for other workloads.

Steps 2-5 should be repeated for each workload.

## Error Handling

If any script in Steps 1-3 fails, load the `assets/kueue-advisory-tool-failed.json` template using load_skill_resource, fill in the details of the failure, and use the create_advisory tool to raise an advisory. Do not stop execution -- continue with remaining steps if possible.

## Prerequisites

- The metrics API URL is available via the run_script tool by executing `scripts/get-metrics-api.sh`.
- All scripts must be executed using the run_script tool. Do not attempt to access the filesystem or environment variables directly.
- Advisory templates are in the `assets/` directory and must be loaded using the load_skill_resource tool.
- Advisories must be created using the create_advisory tool with name, advisory, explaination, and proposal fields.
- When an advisory proposal is approved by a human, you may be invoked to execute it. Use the update_owner tool to modify the owning KueueSkill (e.g. set enableagent to false to suspend the agent).