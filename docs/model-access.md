# Model Access

Currently only Google models can be used with the **Go** version of ADK. This is down to the choices provided in the Go ADK implemntation at [https://github.com/google/adk-go/tree/main/model](https://github.com/google/adk-go/tree/main/model). This may be expanded in future.

The models cn be accessed in 2 ways:

* With a GOOGLE_API_KEY or
* with a Google Vertex AI Service Account

## Access with a Google API Key

This is the simplest method. If you don't already have Gemini API key, create a key in Google AI Studio on the API Keys page.

> Depending on your Google organization setting this may not be allowed.

Deploy the Khieron Helm chart with the GOOGLE_API_KEY set as a value:

```bash
NAMESPACE=khieron-system
GOOGLE_API_KEY=<your key from Google>
helm -n $NAMESPACE install --create-namespace khieron ./dist/khieron/ -f dist/khieron/values.yaml --set googleApiKeySecret.googleApiKey=$GOOGLE_API_KEY
```

> The default model is `gemini-2.5-flash` and is sufficient for most agent processing.

## Access through Vertex AI

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

Depending on the GCP project, the level of model available may vary. If the default model `gemini-2.5-flash` is not be available, substitue in the nearest capability flash model e.g. `gemini-3.5-flash`.

Check the controller logs after deployment for any errors related to the chosen model.

The `*-flash-lite` models are usually not suitable for use with the agent, but the capability will depend on the complexity of the skills you give it.
