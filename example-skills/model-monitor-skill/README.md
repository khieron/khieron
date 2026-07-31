# Model Monitor Skill

A Skill for use with RHOAI MCP.


## RHOAI MCP

The following format is expected


### .mcp.json
```json
{
  "mcpServers": {
    "rhoai": {
      "type": "http",
      "url": "https://rhoai-mcp-khieron-system.apps.ocp-gb.ibm.redhataicatalyst.com/mcp"
    }
  }
}
```

The MCP Server must be deployed from the Khieron Fork of the RHOAI MCP repo:

[https://github.com/khieron/rhoai-mcp](https://github.com/khieron/rhoai-mcp)

```
oc apply -k deploy/kustomize/overlays/openshift-khieron/
oc apply -f deploy/kustomize/overlays/openshift-khieron/networkpolicy.yaml
```

## llm-d Planner

The RHOAI MCP server expects the llm-d planner to be installed in the `planner` namespace on the same cluster.

It uses planner as tool in model recommendation.

> At present I use `v0.1.0` of llm-d-planner as there is some API change in `latest` that breaks compatibility with RHOAI MCP.


## Example use

In Claude Code make sure you have configured access to the `rhoai-mcp` server running remotely on the OpenShift cluster you want to deploy the model to.

Try starting a Claude Code session with 

```
hi claude - I need a recommendation for an LLM for document summarization. It should be fast, accurate and cost-effective. I want to deploy to OpenShift. Please use rhoai-mcp MCP server to help you. I have nvidia A100 GPUs locally and I'd like to use them for this model.
```

This should hopefully recommend Granite 3.1 8B or Qwen.

```
please proceed with the Granite 3.1 8B model FP8 on the nvidia A100 GPU in to the gpuaas-team-a-training namespace.
```