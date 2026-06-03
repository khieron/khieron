# Example Skill to monitor pod deployments

To demonstrate the capabilities of Khieron, this page shows how to create a new Skill CR and
how to put it in to operation.

> All the following code is in [example-skills/monitor-pods-skill](../example-skills/monitor-pods-skill/). Just jump ahead to [Bundling it all together](#bundling-it-all-together) to see the effect.

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

Change the name of the Skill in `values.yaml` to match the name given above e.g.:

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

Use the run_script tool to execute `scripts/get-stuck-pods.sh`, with the `{namespace}` that the Skill is
deployed in to as an argument. This script retrieves a list of pods that have been created through Deployments
or Jobs or otherwise, but yet cannot run for a variety of reasons.

## Step 2: Raise an Advisory

For each of these stuck pods examine the cause and if you think the problem is not just temporary and that it will not fix itself, raise an Advisory.

Use the `load_skill_resource tool` to load the appropriate Advisory template from `assets/`:
- Use `assets/pods-stuck.json` template if the pod image is not found.

Copy the template fields over to the advisory, replacing placeholder fields like `{skill name}`, `{pod}`,
`{namespace}`, `{explaination}`, `{proposal}` with specific details about the issue you discovered. The `proposal` field should describe a concrete action to resolve the issue that the Agent can take.

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

> Make sure the name you give the file below matches the name mentioned in `SKILL.md`. 

`assets/get-stuck-pods.sh`:
```bash
#!/bin/bash

set -e
if [ "$DEBUG" = "1" ] || [ "$DEBUG" = "true" ]; then
  set -x
fi

TOKEN=$(cat /var/run/secrets/kubernetes.io/serviceaccount/token)
CACERT="--cacert /var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
API="https://${KUBERNETES_SERVICE_HOST}:${KUBERNETES_SERVICE_PORT}"

if [ -z "$1" ]; then
  echo "Usage: $0 <namespace>"
  exit 1
fi

NS="$1"

if ! PODS=$(curl -sS -k -f ${CACERT} \
  -H "Authorization: Bearer ${TOKEN}" \
  -H "Accept: application/json" \
  "${API}/api/v1/namespaces/${NS}/pods?fieldSelector=status.phase%21%3DRunning%2Cstatus.phase%21%3DSucceeded"); then

  echo "Failed to query pods in namespace ${NS}: ${PODS}"
  exit 1
fi

NS_RESULTS=$(echo "${PODS}" | jq '[.items[] | {
  name: .metadata.name,
  namespace: .metadata.namespace,
  phase: .status.phase,
  reason: (.status.reason // ""),
  message: (.status.message // ""),
  conditions: [.status.conditions[]? | {type: .type, status: .status, reason: (.reason // ""), message: (.message // "")}],
  createdAt: .metadata.creationTimestamp,
  ownerReference: [.ownerReferences[0]? | {name: .name, kind: .kind, uid: .uid}],
  containers: .status.containerStatuses
}]')

echo "$NS_RESULTS"

exit 0
```


Add another script to handle deletion of the owner:

`delete-owner.sh`:
```bash
#!/bin/bash

set -e
if [ "$DEBUG" = "1" ] || [ "$DEBUG" = "true" ]; then
  set -x
fi

TOKEN=$(cat /var/run/secrets/kubernetes.io/serviceaccount/token)
CACERT="--cacert /var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
API="https://${KUBERNETES_SERVICE_HOST}:${KUBERNETES_SERVICE_PORT}"

if [ -z "$2" ]; then
  echo "Usage: $0 <namespace> <pod>"
  exit 1
fi

NS="$1"
POD="$2"

pluralize() {
  echo "$1" | tr '[:upper:]' '[:lower:]' | sed 's/$/s/'
}

api_path() {
  local api_version="$1"
  local kind_plural="$2"
  local ns="$3"
  local name="$4"
  if [ "$api_version" = "v1" ]; then
    echo "/api/v1/namespaces/${ns}/${kind_plural}/${name}"
  else
    echo "/apis/${api_version}/namespaces/${ns}/${kind_plural}/${name}"
  fi
}

fetch() {
  local path="$1"
  curl -sS -f ${CACERT} \
    -H "Authorization: Bearer ${TOKEN}" \
    -H "Accept: application/json" \
    "${API}${path}"
}

CURRENT_API_VERSION="v1"
CURRENT_KIND="Pod"
CURRENT_NAME="$POD"
CHAIN="Pod ${POD}"

while true; do
  PLURAL=$(pluralize "$CURRENT_KIND")
  PATH_URL=$(api_path "$CURRENT_API_VERSION" "$PLURAL" "$NS" "$CURRENT_NAME")

  OBJ=$(fetch "$PATH_URL") || {
    echo "Failed to fetch ${CURRENT_KIND}/${CURRENT_NAME}"
    exit 1
  }

  OWNER_REF=$(echo "$OBJ" | jq -r '.metadata.ownerReferences[0] // empty')

  if [ -z "$OWNER_REF" ]; then
    echo "Root owner found: ${CURRENT_KIND}/${CURRENT_NAME}"
    echo "Chain: ${CHAIN}"
    echo "Deleting ${CURRENT_KIND}/${CURRENT_NAME}..."

    curl -sS -f ${CACERT} \
      -X DELETE \
      -H "Authorization: Bearer ${TOKEN}" \
      -H "Accept: application/json" \
      "${API}${PATH_URL}"

    echo "Deleted ${CURRENT_KIND}/${CURRENT_NAME}"
    exit 0
  fi

  CURRENT_API_VERSION=$(echo "$OWNER_REF" | jq -r '.apiVersion')
  CURRENT_KIND=$(echo "$OWNER_REF" | jq -r '.kind')
  CURRENT_NAME=$(echo "$OWNER_REF" | jq -r '.name')
  CHAIN="${CHAIN} -> ${CURRENT_KIND} ${CURRENT_NAME}"
  echo "Following owner: ${CURRENT_KIND}/${CURRENT_NAME}"
done

```

You should lint bash scripts like this with the [shellcheck](https://www.shellcheck.net/) utility.

If necessary the scripts can be run and debugged inside the controller pod
by running them manually. They are held at `/tmp/<random-name>/<skill-name>/scripts/<script-name>`.


## Defining the assets

Json files that act as templates for the Advisory should be placed in the `assets` folder of the skill.

Replace the template contents with:

`monitor-pods-skill/skill-files/pods-stuck.json`:
```json
{
    "name": "pods-stuck",
    "advisory": "Pod {pod} in namepsace {namespace} has been created but is stuck. Is is owned by {ownerReference}",
    "explaination": "{explaination}",
    "proposal": "Terminate the pod (pod} in namespace {namespace} using the tool 'scripts/delete-owner.sh' passing {namespace} and {pod} as arguments"
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
  - delete
- apiGroups:
  - ""
  resources:
  - events
  verbs:
  - get
  - list
  - create
- apiGroups:
  - "apps"
  resources:
  - replicasets
  - deployments
  verbs:
  - get
  - list
  - delete
```

These allow a fine grained control over the permissions of the Skill in the particular namespace the Skill
runs in. The permissions should correspond to what the scripts actually do. If you get a 403 error in the
script, come back and examine these permissions.

For the role binding, the service account **must** be that of the deployed Khieron controller.

```bash
kubectl -n khieron-system get serviceaccounts

NAME                         AGE
default                       4d
khieron-controller-manager   16m
```

This needs to be changed in `values.yaml` to reflect where you deployed the Khieron controller:

```yaml
serviceAccount:
  name: khieron-controller-manager
  namespace: khieron-system
```


## Bundling it all together

That's it! The `skill.yaml` and `configmap.yaml` to not need to be edited.

To see the manifest that will be produced use `helm template` command

```bash
helm -n my-namespace template --release-name foobar ./monitor-pods-skill
```

To deploy, use `helm install` command:

```bash
helm -n my-namespace install --create-namespace monitor-pods-skill ./monitor-pods-skill
```

## Monitor the deployment

To see the Skill:

```bash
kubectl -n my-namespace describe skill
```

Create a broken deployment of a pod to see the skill analyze it and create an Advisory:

```bash
kubectl -n my-namespace apply -f example-skills/broken-deployment.yaml
```

To see any Advisories:

```bash
kubectl -n my-namespace describe advisory
```

> If you do not see an advisory yet, you might want to force the agent to run now:
> 
> ```bash
> kubectl -n my-namespace annotate skill monitor-pods-skill khieron.io/run-requested=$(date -u +%FT%TZ) --overwrite
> ```

A sample advistory based on the broken-deployment above, might read like:

```yaml
apiVersion: v1
items:
- apiVersion: agency.khieron.io/v1alpha1
  kind: Advisory
  metadata:
    creationTimestamp: "2026-06-02T19:38:46Z"
    generateName: monitor-pods-skill-monitor-pods-skill-broken-deployment-7f4d9c5f46-gcrmf-image-pull-backoff-
    generation: 1
    labels:
      khieron.io/job-name: broken-deployment-7f4d9c5f46-gcrmf
      khieron.io/job-namespace: example-skills
    name: monitor-pods-skill-monitor-pods-skill-broken-deployment-7fjk8m6
    namespace: example-skills
    ownerReferences:
    - apiVersion: agency.khieron.io/v1alpha1
      blockOwnerDeletion: true
      controller: true
      kind: Skill
      name: monitor-pods-skill
      uid: da5e79ad-f989-4e24-8e82-4dd1adc24586
    resourceVersion: "15181"
    uid: cd9d29da-cd72-4a9b-af90-3fba6dc8536a
  spec: {}
  status:
    advisory: Pod broken-deployment-7f4d9c5f46-gcrmf in namespace example-skills has
      been created but is stuck. It is owned by broken-deployment-7f4d9c5f46-gcrmf
    advisoryupdatedtime: "2026-06-02T19:38:46Z"
    explanation: The pod 'broken-deployment-7f4d9c5f46-gcrmf' in namespace 'example-skills'
      is stuck in 'Pending' phase because its container 'web-container' failed to
      pull the image 'nginx:this-tag-does-not-exist'. The image was not found.
    proposal: Terminate the pod broken-deployment-7f4d9c5f46-9bcxh in namespace example-skills using the tool 'scripts/delete-owner.sh' passing example-skills and broken-deployment-7f4d9c5f46-9bcxh as arguments
kind: List
metadata:
  resourceVersion: ""
```


To approve an Advisory:

```bash
$ADVISORY_NAME=<advisory name>
kubectl -n my-namespace patch advisory $ADVISORY_NAME --type merge -p '{"spec":{"approver":"admin"}}'
```

In this example case, approving the Advisory executes the Proposal which calls the `delete-owner.sh` script.
It recursively finds the owner of the Pod - first the parent ReplicaSet and then the grand-parent the
Deployment and deletes it.

The advisory events show the resolution:

```
Events:                                                                                                          
  Type    Reason            Age   From                 Message                                                                                                 
  ----    ------            ----  ----                 -------                                                                                                 
  Normal  Approved          22m   advisory-controller  Advisory approved by sean
  Normal  ProposalExecuted  22m   advisory-controller  Proposal executed: Terminate the pod
                                                       broken-deployment-7f4d9c5f46-9bcxh in namespace
                                                       example-skills using the tool 'scripts/delete-owner.sh'
                                                       passing example-skills and
                                                       broken-deployment-7f4d9c5f46-9bcxh as arguments
```

The Deployemnt is deleted. The Advisory can be deleted at this stage.
