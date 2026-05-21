# Khieron - the lightweight kubernetes native SRE agent framework

If you find the major agentic frameworks heavy and overengineered for simple agents to manage SRE tasks, you're not alone.

That's why we developed Project Khieron (pronounced Kay-ron), to bring kubernetes native operator design together with
a Go native SDK [adk-go](https://adk.dev), to create a lightweight framework that brings together two simple concepts
**Skills** (for a flexible and dynamic way of creating single minded single task agents) and **Advisories** (a Human In
the Loop mechanism in a familiar style).

## The lightweight advantage

Commonly cited agentic frameworks for Kubernetes tend to be general purpose behemoths, capable of doing many external facing tasks, but also bringing with them several external systems that and other projects that seem to have a combinatorial effect in increasing complexity.

If you're trying to acheieve a simple task to manage your cluster, it can feel like using a sledgehammer to crack a nut.

## Simplicity throughout

Khieron uses the Agent Development Tookit (Go version) bundled inside the controller, with no Python dependencies (greatly reducing the size of the pod and the startup time), making it suitable for Edge use cases.

There are no child containers to manage. No LiteLLM. No LangChain/LangGrpah.

## Flexibility

As a backend model the ADK provides several options for connecting to models such as Gemini or Claude or to self hosted models like Gemma.

## Dynamic configuration

The `Skill` CRD is linked to a `ConfigMap` that contains a **Skill.md** file and some `assets`, `scripts` and `references`. Each Skill dynamically creates **one** Agent and runs in a loop doing one task.

## Internal and External Tooling

The Skill CRD contains an `assets` folder to contain bash scripts used as Externally defined tools.

The controller also has some internally defined tools, written in Go, covering many useful gernal puspose tasks on the Kubernetes API. These are available to the Skills to perform tasks.

## Kubernetes Native Human In the Loop

The creation and management of `Advisories` objects (defined as a CRD) gives an easily manageable interface for humans to see, and approve or reject or comment on some anomaly the Agent finds.

The fact that they are CRDs allow them to have their own control loop intercting with their owner `Skill` as necessary, using its tools and assets.

### Protections on overuse

To protect from the Agent calling the Model too many times, the agent runs every 5 minutes (configurable). Additionally if the agent finds that a tool is returning a failure that needs attention the Agent will pause the Skill.

## Non goals

The Project does not intend to be a comprehensive Agentic framework and is not designed to create a
RAG system.

## Getting Started

The Agent requires a GOOGLE_API_KEY. Create an API Key in [Google AI Studio](https://aistudio.google.com/app/apikey).

Create a secret to keep this key:

```bash
NAMESPACE=<your-namespace>
GOOGLE_API_KEY=<your api key>
kubectl create secret generic google-api-key \
  --from-literal=GOOGLE_API_KEY="$GOOGLE_API_KEY" \
  -n $NAMESPACE
```