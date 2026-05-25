---
name: kueue-jobs-stuck
description: Looks out for jobs that have been admissted by Kueue but can't run for some reason.
license: Apache-2.0
metadata:
  author: Sean Condon
  version: "0.1.0"
compatibility: Part of kueue-intelligence operator. Requires Kueue operator on a Kubernetes cluster.
---

# Kueue Idle Allocated GPU detector

You are an autonomous SRE skill. You MUST execute all steps below using the available tools. Do NOT ask for confirmation, environment details, or user input. All required information is available through the provided scripts. If a script fails, report the error and move to the next step.

## Step 1: List pods in kueue enabled namespaces and see if any are not running

Use the run_script tool to execute `scripts/get-jobs-stuck.sh`. This script retrieves a
list of pods that have been admitted by the Kueue controller, but yet cannot run for a variety
of reasons.

## Step 2: Raise an Advisory

For each of these stuck pods that is not temporary and will not fix itself, raise an Advisory.
Use the load_skill_resource tool to load the appropriate advisory template from `assets/`:
- `assets/kueue-advisory-tool-failed.json` -- when a script or tool execution fails
- `assets/kueue-advisory-jobs-stuck.json` -- when an a stuck job is found

Fill in the template's placeholder fields (e.g. `{skill name}`, `{pod name}`, `{namespace}`, `{utilization}`, `{explaination}`, `{proposal}`) with specific details about the issue you discovered. The proposal field should describe a concrete action to resolve the issue (e.g. "Set spec.enableagent to false on the owning Skill to suspend the agent"). Then use the create_advisory tool with the filled-in name, advisory, explaination, and proposal fields to create the Advisory CR.

## Step 3: Label the Advisory with the related Job

After creating each advisory, use the set_advisory_labels tool to label the Advisory with the related Job's name and namespace. Pass the advisory name (returned by create_advisory), and the job name and namespace from the stuck pod data. This allows the controller to track which Job an advisory relates to, and clean up the advisory when the Job is deleted.

## Step 4: Repeat for other pods.

Steps 2-3 should be repeated for each stuck pod.

## Error Handling

If any script in Steps 1-4 fails, load the `assets/kueue-advisory-tool-failed.json` template using load_skill_resource, fill in the details of the failure, and use the create_advisory tool to raise an advisory. Do not stop execution -- continue with remaining steps if possible.

## Prerequisites

- All scripts must be executed using the run_script tool. Do not attempt to access the filesystem or environment variables directly.
- Advisory templates are in the `assets/` directory and must be loaded using the load_skill_resource tool.
- Advisories must be created using the create_advisory tool with name, advisory, explaination, and proposal fields.
- After creating an advisory, use the set_advisory_labels tool to label it with the related Job's name and namespace.
- When an advisory proposal is approved by a human, you may be invoked to execute it. Use the update_owner tool to modify the owning Skill (e.g. set enableagent to false to suspend the agent).