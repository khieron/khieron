# Skill Developer Guide for Khieron

More details to be provided later. For the moment, see the
[skills examples](../example-skills/)

## Start a new Skill with **helm create**

To start a new Skill use the helm chart template in [skill-helm-chart-template](../skill-helm-chart-template/)

```bash
# Clone the repo
git clone https://github.com/khieron/khieron
KHIERON_LOCATION=$PWD/khieron

# Change to your target folder
cd <my folder>
helm create monitor-pods-skill --starter $KHIERON_LOCATION/skill-helm-chart-template
```

This will create a new folder with placeholder files:

```
monitor-pods-skill
├── Chart.yaml
├── skill-files
│   ├── assets
│   │   └── advisory-template.json
│   ├── scripts
│   │   └── my-script.sh
│   └── SKILL.md
├── templates
│   ├── configmap.yaml
│   ├── mcpconfig.yaml
│   ├── _helpers.tpl
│   ├── rolebinding.yaml
│   ├── role.yaml
│   ├── serviceaccount.yaml
│   └── skill.yaml
└── values.yaml
```

Defining the `SKILL.md` comes first, then the bash `scripts` that will act as internal tools and
finally the `assets` such as Advisory templates.

Once the Skill is defined the helm chart deploys it as a configmap and points to it from Skill CR.

RBAC roles needed to run the scripts should be added to `role.yaml`. Add a ClusterRole and ClusterRoleBinding
for accessing from multiple namespaces.

Helm template command can then be used to preview the deployment.

## Defining SKILL.md

Defining Skills generally follow the guidelines of the [Agent Skill specification](https://agentskills.io/specification). The structure of a Skill allows it to be loaded incrementally to minimize the impact on the operating context window of the agent.

In the SKILL, we refer to internal tools and external tools (scripts).

The internal tools are Go functions written in [skill_controller.go](../internal/controller/skill_controller.go).

The current list are:

* `updateOwnerTool` - update a Skill (usually used by an Advisory controller to update a Skill CR when a user approves the Advisory).
* `setAdvisoryLabelsTool` - labels a Advisory with the related resource name and namespace, enabling the controller to clean up advisories when the resource is deleted
* `createAdvisoryTool` - creates Advisory CRs
* `runScriptTool` - executes scripts from the skill's scripts/ directory

External tools will be given in the `scripts/` directory of the Skill and can be mentioned by filename in the `SKILL.md`.

## Defining the scripts

Scripts are currently limited to Bash scripts. We refer to these as external tools. They run as a sub process of the controller using `exec()`;

In `SKILL.md` you refer to these scripts by file name including the path starting with `skills/`.

You should lint bash scripts like this with the [shellcheck](https://www.shellcheck.net/) utility.

If necessary the scripts can be run and debugged inside the controller pod
by running them manually. They are held at `/tmp/<random-name>/<skill-name>/scripts/<script-name>`.

## Defining the assets

Json files that act as templates for the Advisory should be placed in the `assets` folder of the skill.

## Defining the manifest and permissions

To allow pods to be accessed, add permissions to `role.yaml`.

These allow a fine grained control over the permissions of the Skill in the particular namespace the Skill runs in. The permissions should correspond to what the scripts actually do. If you get a 403 error in the script, come back and examine these permissions.

For the role binding, the service account **must** be that of the deployed Khieron controller.

```bash
kubectl -n khieron-system get serviceaccounts

NAME                         AGE
default                       4d
khieron   16m
```

This needs to be changed in `values.yaml` to reflect where you deployed the Khieron controller:

```yaml
serviceAccount:
  name: khieron
  namespace: khieron-system
```

## Evaluating the Skill

It can be useful to evaluate your Skill with a linting tool like [skilleval](https://github.com/natifridman/skilleval).

```bash
npx skilleval check ./example-skills/monitor-pods-skill/skill-files/
```


## MCP Integration

Khieron can support MCP servers when specified in a Skill CR.

The MCP server details should be specificed in a ConfigMap as **mcp.json**, and reference in the Skills `MCPConfigRef` attribute.

The format of **mcp.json** should follow the format of [MCP JSON Coniguration Standard](https://gofastmcp.com/integrations/mcp-json-configuration#mcp-json-configuration-standard)

In the Helm chart for your Skill specify the config in the `values.yaml` field `mcpConfig.mcpJson`.

### Referring to MCP tools

In your `SKILL.md` you can refer to tools provided the configured MCP server as you would any other tool (internat or external).

### Remote streamable HTTP only supported at present

Because the Khieron container image is build from UBI 10 Minimal, it does not have a Python, Node or Java runtime environment, and
so it is more favorable to connect to MCP servers remotely using Streamable Http format.

> Native `stdio` MCP hosting will be supported in future.

Because the Go library `github.com/modelcontextprotocol/go-sdk/mcp` which Kieron uses prefers Streamable HTTP, it is preferred to
run MCP servers with `--transport streamable-http` (rather than `--transport sse`).
