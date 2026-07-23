# Model Access

Currently only Google models can be used with the **Go** version of ADK. This is down to the choices provided in the Go ADK implemntation at [https://github.com/google/adk-go/tree/main/model](https://github.com/google/adk-go/tree/main/model). This may be expanded in future.

The models cn be accessed in 3 ways:

* Gemini API with a GOOGLE_API_KEY or
* Gemini API with a Google Vertex AI Service Account
* OpenAI API with an OPENAI_API_KEY and an OPENAI_BASE_URL

## Gemini API Access with a Google API Key

This is the simplest method. If you don't already have Gemini API key, create a key in Google AI Studio on the API Keys page.

> Depending on your Google organization setting this may not be allowed.

Deploy the Khieron Helm chart with the GOOGLE_API_KEY set as a value:

```bash
NAMESPACE=khieron-system
GOOGLE_API_KEY=<your key from Google>
helm -n $NAMESPACE install --create-namespace khieron ./dist/khieron/ -f dist/khieron/values.yaml --set googleApiKeySecret.googleApiKey=$GOOGLE_API_KEY
```

> The default model is `gemini-2.5-flash` and is sufficient for most agent processing.

## Gemini API Access through Vertex AI

Access through Vertex AI may be preferred by some organizations.

This approach requires 3 pieces of information:

* GOOGLE_CLOUD_PROJECT name
* GOOGLE_CLOUD_LOCATION region name
* A key for a Service Account that allows access to the Vertex API.

### GCP Service Account

The GCP Account should have the Gemini API enabled.

Under "IAM and admin" add a Service Account.

The Service Account should be bound to the role: `roles/aiplatform.user`.

Add a Key to the service account and export it as a JSON file to your local system. This should be treated as a credential and should not be shared or checked in to git.

### Deploy Khieron

```bash
GOOGLE_CLOUD_PROJECT=<Google Cloud Project name>
GOOGLE_CLOUD_LOCATION=<Google cloud region name - 'global' by default>
KEY_FILE=<local location of service account file downloaded from GCP in JSON format>

helm -n khieron-system install khieron oci://ghcr.io/khieron/charts/khieron \
--set googleApiKeySecret.googleCloudProject=$GOOGLE_CLOUD_PROJECT \
--set googleApiKeySecret.googleCloudLocation=$GOOGLE_CLOUD_LOCATION \
--set-file googleServiceAccountKey.keyJson=$KEY_FILE
```

### Model choice

Depending on the GCP project, the level of model available may vary. Check the controller logs after deployment for any errors related to the chosen model.

At the time of writing (June 2026) `gemini-2.5-flash` is the most suitable model because of its pricing and capability, but will only be supported by Google until October 2026.

The next most suitable model is `gemini-3.5-flash`, and while it has greater capabilities it costs 5x more for input tokens and 3.6x more for output tokens than 2.5 Flash.

Specifying the model as `gemini-flash-latest` is a future proof option, that may be a good choice if lower maintenance is required. This will track the latest model as new versions of Gemini Flash become available, but with the risk of higher charges. 

The `*-flash-lite` models are usually not suitable for use with the agent, but the capability will depend on the complexity of the skills you give it.

The model may be changed through the [values.yaml](../dist/khieron/values.yaml) of the Helm chart with:

```--set modelName=gemini-2.5-flash"
```

## OpenAI API access

OpenAI access currenly uses the approach from ADK-go branch [openai_support](https://github.com/google/adk-go/tree/openai_support).

> This accesses the Responses API (rather than the older Chat Completions). vLLM deployments of models such as Gemma support this API

### Deploy Khieron

```bash
OPENAI_BASE_URL=<endpoint url>
MODEL_NAME=<model name from /v1/models>
OPENAI_API_KEY=<Your OpenAI API key>

helm -n khieron-system install khieron oci://ghcr.io/khieron/charts/khieron \
--set modelBackend=openai \
--set modelName=$MODEL_NAME \
--set openaiBaseUrl=$OPENAI_BASE_URL \
--set openaiApiKeySecret.openaiApiKey=$OPENAI_API_KEY
```

When a model is deployed as a Deployment on OpenShift AI through KServe it will be accessible:

* internally through an Service or
* externally through a HttpRoute

Internal access is possible where Khieron is deployed on the same cluster as the OpenAI compatible model. It will not require an OPENAI_API_KEY and can use the service name as the URL like `https://<model-name>.<namespace>.svc.cluster.local/v1`

External access is used when the model is deployed externally to the cluster Khieron sits on. It's URL can be found on the Route connected to the model deployment e.g. `https://gemma-4-31b-ls-evals.apps.ocp-gb.ibm.redhataicatalyst.com/v1`. Since this endpoint requires Authentication set the `OPENAI_BASE_URL` to the OpenShift access token.
> The token for `oc login` will expire after 24 hours, so it is recommended to get a `serviceaccount` token instead.
