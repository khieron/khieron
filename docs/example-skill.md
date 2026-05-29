# Example Skill to monitor pod deployments

To demonstrate the capabilities of Khieron, this page shows how to create a new Skill CR and
how to put it in to operation.

Defining the `SKILL.md` comes first, then the bash scripts that will act as internal tools and
finally the assets such as Advisory templates.

Once the Skill is defined, we declare a manifest whichg includes a configmap for the Skill files and a Skill CR and any necessary permissions that it needs. 

## Designing SKILL.md

Defining Skills generally follow the guidelines of the [Agent Skill specification](https://agentskills.io/specification). The structure of a Skill allows it to be loaded incrementally to minimize the impact on the operating context window of the agent.

In our example we want the Skill to be strongly focussed on monitoring Pods and making sure they start
and run properly. We start with the front matter and a structured set of steps to take.

`SKILL.md`:
```markdown
---
name: monitor pods
description: Looks out for pods that are deployed but aren't running for some reason.
license: Apache-2.0
metadata:
  author: Khieron documentation
  version: "0.1.0"
compatibility: Used by khieron operator. Runs on a Kubernetes cluster.
---

# Kubernetes Pod Monitor Agent

You are an autonomous SRE Agent for monitoring Kubernetes pods. You MUST execute all steps below using the available tools. Do NOT ask for confirmation, environment details, or user input. All required information is available through the provided scripts. If a script fails, report the error and move to the next step. 

TBC
```

## Defining the scripts



## Defining the assets



## Defining the manifest and permissions


## Bundling it all together