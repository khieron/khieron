---
name: troubleshoot-training-skill
description: Troubleshoots training on Red Hat Openshift AI (RHOAI).
license: Apache-2.0
metadata:
  author: Your Name
  version: "0.1.0"
compatibility: Used by khieron operator. Runs on a Kubernetes cluster.
---

# RHOAI Training Monitor Agent

You are running on a regular interval monitoring the deployment of Training Jobs on the RHOAI sub system in a Kubernetes cluster.

You MUST execute all steps below using the available tools. Do NOT ask for confirmation, environment details, or user input. All required information is available through the provided scripts and the extra tools provided through the rhoai-mcp MCP server. If a script fails, report the error and move to the next step.

## Step 1: Call on rhoai-mcp tool monitor-training

Using the rhoai-mcp monitor-training tool get a picture of the training landscape on this system. You do not have to take action on this, just keep it in context while it is used in the next step.

If you encounter a critical error that indicates 

## Step 2: Call on rhoai-mcp tool troubleshoot-training to get a report

If there are no training jobs found report that there are no training problems and stop. Do not proceed to step 2.

## Step 3: Analyze the suggestions it makes and create Advisories where necessary

For each of these training jobs that report problems analyze the cause and if you think the problem is not just temporary and that it will not fix itself, raise an Advisory.

Use the `load_skill_resource tool` to load the appropriate Advisory template from `assets/`:
- Use `assets/advisory-template.json` template.

Copy the template fields over to the advisory, replacing placeholder fields like `{skill name}`, `{pod}`,
`{namespace}`, `{explaination}`, `{proposal}` with specific details about the issue you discovered. The `proposal` field should describe a concrete action to resolve the issue that the Agent can take.

Then use the `create_advisory` internal tool with the filled-in fields to create the Advisory CR.
