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
please proceed with the RedHatAI/granite-3.1-8b-instruct on the nvidia A100 GPU in to the gpuaas-team-a-training namespace. Please don't use openvino.
```

In this scenario the SKILL_CONTEXT.md might appear like below. The important point is that the Planner recomendations have been included so that the Skill will be able to compare them to actual metrics.

```markdown
model-monitor-granite-3-1-8b-instruct-model-monitor-skill___references___SKILL-CONTEXT.md:
----
# Model Deployment Context

## Model Parameters
- **Model**: RedHatAI/granite-3.1-8b-instruct
- **Format**: pytorch
- **Estimated Parameters**: 8.0B
- **Estimated Size**: 26.0 GB
- **Is LLM**: True
- **Runtime**: kserve-ovms
- **Storage URI**: oci://registry.redhat.io/rhelai1/modelcar-granite-3-1-8b-instruct:1.5

## Resource Requirements
- **GPUs**: 1
- **GPU Memory**: 31 GB
- **Memory Request**: 16Gi
- **Memory Limit**: 32Gi
- **CPU**: 2

## Planner Recommendations

### Performance / Cost / Balanced
- **Model**: Granite 3.1 8B Instruct
- **GPU Config**: 1x A100-80
- **Cost**: $2,555/month
- **TTFT (p95)**: 116ms
- **ITL (p95)**: 21ms
- **E2E (p95)**: 7958ms
- **Throughput**: 3.0 qps
- **Reasoning**: Selected Granite 3.1 8B Instruct (8B) for chatbot_conversational use case. Deploying on A100-80 GPUs. Expected performance: TTFT=116ms (p95), ITL=21ms (p95)
```


## Testing with Guide LLM

Testing the model deployment with GuideLLM can push the deployment to its limits. This can be useful to see advisories being created.


```bash
guidellm run \
  --backend kind=openai_http,target=http://localhost:8080 \
  --tokenizer kind=huggingface_auto,model=ibm-granite/granite-3.1-8b-instruct \
  --profile kind=sweep \
  --constraint kind=max_duration,seconds=30 \
  --data kind=synthetic_text,prompt_tokens=256,output_tokens=128
```