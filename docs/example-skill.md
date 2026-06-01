# Example Skill to monitor pod deployments

To demonstrate the capabilities of Khieron, this page shows how to create a new Skill CR and
how to put it in to operation.

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

Change the name of the Skill in `values.yaml` e.g.:

```bash
sed -i 's/my-skill/monitor-pods-skill/g' monitor-pods-skill/values.yaml
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

In our example we want the Skill to be strongly focussed on monitoring Pods in its own namespace only and making sure they start and run properly. We start with the front-matter and a structured set of steps to take.

In the SKILL, we refer to internal tools and external tools (scripts).

The internal tools are Go functions written in [skill_controller.go](../internal/controller/skill_controller.go).

The current list are:

* `updateOwnerTool` - update a Skill (usually used by an Advisory controller to update a Skill CR when a user approves the Advisory).
* `setAdvisoryLabelsTool` - labels a Advisory with the related resource name and namespace, enabling the controller to clean up advisories when the resource is deleted
* `createAdvisoryTool` - creates Advisory CRs
* `runScriptTool` - executes scripts from the skill's scripts/ directory

External tools will be given in the `scripts/` directory of the Skill and can be mentioned by filename in the Skill.md


`SKILL.md`:
```markdown
---
name: monitor-pods-skill
description: Looks out for pods in the current namespace that are deployed but aren't running for some reason.
license: Apache-2.0
metadata:
  author: Khieron authors
  version: "0.1.0"
compatibility: Used by khieron operator. Runs on a Kubernetes cluster.
---

# Kubernetes Pod Monitor Agent

You are an autonomous SRE Agent for monitoring Kubernetes pods. You MUST execute all steps below using the available tools. Do NOT ask for confirmation, environment details, or user input. All required information is available through the provided scripts. If a script fails, report the error and move to the next step. 

## Step 1: List the pods in the current namespace and see if any are not running

Use the run_script tool to execute `scripts/get-pods-stuck.sh`. This script retrieves a
list of pods that have been created through Deployments or Jobs or otherwise, but yet cannot run for a variety
of reasons.

## Step 2: Raise an Advisory

For each of these stuck pods examine the cause and if you think the problem is not just temporary and that it will not fix itself, raise an Advisory.

Use the `load_skill_resource tool` to load the appropriate Advisory template from `assets/`:
- `assets/pods-stuck.json` -- when an a stuck job is found.

Fill in the template's placeholder fields (e.g. `{skill name}`, `{pod name}`, `{namespace}`, `{explaination}`, `{proposal}`) with specific details about the issue you discovered. The `proposal` field should describe a concrete action to resolve the issue.

Then use the `create_advisory` internal tool with the filled-in fields to create the Advisory CR.

## Step 3: Label the Advisory with the related Job

After creating each Advisory, use the `set_advisory_labels` internal tool to label the Advisory with the related Job's name and namespace. Pass the Advisory name (returned by create_advisory), and the Pod name and namespace from the stuck pod data. This allows the controller to track which Job an advisory relates to, and clean up the advisory when the Pod is deleted.

## Step 4: Repeat for other pods.

Steps 2-3 should be repeated for each stuck pod.

```

The system prompt that expains all the fixed parts of the Skill and Agent CRs is given in the Agent initial prompt
deployed with the controller in the ConfigMap [agent-instruction](../config/default/agent_instruction_configmap.yaml). These instructions should not be duplicated or contradicted by the Skill.md defintion.

## Defining the scripts

Scripts are currently limited to Bash scripts. We refer to these as external tools. They run as a sub process of the controller using `exec()`;

In `SKILL.md` you refer to these scripts by file name including the path starting with `skills/`.

`assets/get_stuck_pods.sh`:
```bash
#!/bin/bash

set -ex

TOKEN=$(cat /var/run/secrets/kubernetes.io/serviceaccount/token)
CACERT="--cacert /var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
API="https://${KUBERNETES_SERVICE_HOST}:${KUBERNETES_SERVICE_PORT}"

# Get namespaces labelled with kueue.openshift.io/managed=true
NAMESPACES=$(curl -sS -k -f ${CACERT} \
  -H "Authorization: Bearer ${TOKEN}" \
  -H "Accept: application/json" \
  -G --data-urlencode 'labelSelector=kueue.openshift.io/managed=true' \
  "${API}/api/v1/namespaces")

if [ $? -ne 0 ]; then
  echo "Failed to query namespaces: ${NAMESPACES}"
  exit -1
fi

NS_LIST=$(echo "${NAMESPACES}" | jq -r '.items[].metadata.name')

if [ -z "${NS_LIST}" ]; then
  echo "[]"
  exit 0
fi

# Collect non-running pods from each managed namespace
RESULTS="[]"
for NS in ${NS_LIST}; do
  PODS=$(curl -sS -k -f ${CACERT} \
    -H "Authorization: Bearer ${TOKEN}" \
    -H "Accept: application/json" \
    "${API}/api/v1/namespaces/${NS}/pods?fieldSelector=status.phase%21%3DRunning%2Cstatus.phase%21%3DSucceeded")

  if [ $? -ne 0 ]; then
    echo "Failed to query pods in namespace ${NS}: ${PODS}"
    continue
  fi

  NS_RESULTS=$(echo "${PODS}" | jq '[.items[] | {
    name: .metadata.name,
    namespace: .metadata.namespace,
    job: ([.metadata.ownerReferences[]? | select(.kind == "Job") | .name][0] // ""),
    workload: (.metadata.annotations["kueue.x-k8s.io/workload"] // ""),
    phase: .status.phase,
    reason: (.status.reason // ""),
    message: (.status.message // ""),
    conditions: [.status.conditions[]? | {type: .type, status: .status, reason: (.reason // ""), message: (.message // "")}],
    createdAt: .metadata.creationTimestamp
  }]')

  RESULTS=$(echo "${RESULTS}" "${NS_RESULTS}" | jq -s '.[0] + .[1]')
done

echo "${RESULTS}"

exit 0
```

You should lint bash scripts like this with the [shellcheck](https://www.shellcheck.net/) utility.

If necessary the scripts can be run and debugged inside the controller pod
by running them manually. They are held at `/tmp/<random-name>/<skill-name>/scripts/<script-name>`.


## Defining the assets

Json files that act as templates for the Advisory should be placed in the `assets` folder of the skill.

Replace the template contents with:

`monitor-pods-skill/skill-files/advisory-template.json`:
```json
{
    "name": "pod-stuck",
    "advisory": "Pod {pod} in namepsace {namespace} has been created but is stuck",
    "explaination": "{explaination}",
    "proposal": "Terminate the pod {pod} in namespace {namespace}"
}

```

## Defining the manifest and permissions

To allow pods to be accessed, add permissions to `role.yaml`:

`monitor-pods-skill/templates/skill.yaml`:
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: {{ include "skill.name" . }}
  labels:
    {{- include "skill.labels" . | nindent 4 }}
rules:
- apiGroups:
  - ""
  resources:
  - pods
  verbs:
  - get
  - list
  - watch
  - delete
```


## Bundling it all together

That's it! The `skill.yaml` and `configmap.yaml` to not need to be edited.

To see the manifest that will be produced use `helm template` command

```bash
helm -n my-namespace template --release-name foobar monitor-pods-skill
```
