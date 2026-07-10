# Observing agent with Open Telemetry

The traces and logs from the agent can be exported to an Open Telemetry provider such as MLFlow or OpenObserve, so that the individual actions of the agent can be tracked.

This can be useful in debugging skills and understanding new models.

## Configuring the endpoint

There are 2 configuration parameters passed in through environment variables:

* **OTEL_EXPORTER_OTLP_ENDPOINT** the endpoint to export to.
  * For traces `/v1/traces` will be automatically appended.
  * For logs `/v1/logs` will be automatically appended.
  * When the endpoint is deployed inside the cluster, the internal service name with port number should be used
  * For OpenObserve an example is `http://o2-openobserve-standalone.openobserve.svc.cluster.local:5080/api/default`
    * **default** is the Org name in OpenObserve - change this for other orgs.
  * For MLFlow and example is `http://mlflow-mlflow.mlflow.svc.cluster.local:5000`
  * With the Helm chart this value may be set with `controllerManager.manager.env.otelExporterOtlpEndpoint`
  * When the `x-mlflow-experiment-id` header is detected in `OTEL_EXPORTER_OTLP_HEADERS`, only traces are exported (logs are skipped), because MLFlow does not support OTLP log ingestion.

  
* **OTEL_EXPORTER_OTLP_HEADERS** - headers to pass when exporting OTEL
  * This is mainly used to pass an Authorization Header
  * For OpenObserve the value is required, and may be got from the OpenObserve Web UI -> Data Sources -> Custom -> Logs -> OTEL Collector and the header value will be shown
  * When setting the value `=` should be used as the separator in place of `:`, and no quotes should be given
    * Example: `Authorization=Basic <base64 value>`
    * Example: `x-mlflow-experiment-id=0` (necessary for MLFlow)
      * The value must be numeric, and the experiment must already exist in MLFlow
      * When this header is present, log export is automatically disabled
    * Example: `x-mlflow-workspace=khieron-system` (necessary for MLFlow on OpenShift where workspaces are enabled)
  * With the Helm chart this value may be set with `otelHeadersSecret.otelExporterOtlpHeaders`
    * when using the `--set` syntax with Helm and more than 1 header needs to be set seperate them with a `\,` (an escaped comma). 

## Examples

### Local MLFlow

```bash
MLFLOW_EXPERIMENT_ID=<id as an integer>
helm -n khieron-system install --create-namespace khieron oci://ghcr.io/khieron/charts/khieron \
--set googleApiKeySecret.googleApiKey=$GOOGLE_API_KEY \
--set controllerManager.manager.env.otelExporterOtlpEndpoint="http://mlflow-mlflow.mlflow.svc.cluster.local:5000" \
--set otelHeadersSecret.otelExporterOtlpHeaders="x-mlflow-experiment-id=$MLFLOW_EXPERIMENT_ID"
```

> MLFlow supports only Traces, not Logs

### Local OpenObserve

```bash
OPEN_OBSERVE_TOKEN=<token from OpenObserve>
helm -n khieron-system install --create-namespace khieron oci://ghcr.io/khieron/charts/khieron \
--set googleApiKeySecret.googleApiKey=$GOOGLE_API_KEY \
--set controllerManager.manager.env.otelExporterOtlpEndpoint="http://o2-openobserve-standalone.openobserve.svc.cluster.local:5080/api/default" \
--set otelHeadersSecret.otelExporterOtlpHeaders="Authorization=Basic $OPEN_OBSERVE_TOKEN"
```

> OpenObserve provides both Logs and Traces

### Openshift MLFlow

First enable the MLflow operator and create an mflow instance in `redhat-ods-applications` namespace. In the MLFlow UI create 

Then then using the RHOAI dashboard (Develop & Train -> Experiments) create an
experiment in the `khieron-system` Project (namespace).

The experiment ID (integer) can be got from the information button next to the experiment name or in the URL of the page. Use this to configure Khieron 

```bash
MLFLOW_EXPERIMENT_ID=1 # check your value. see notes
MLFLOW_WORKSPACE=khieron-system
helm -n khieron-system upgrade khieron oci://ghcr.io/khieron/charts/khieron -f dist/khieron/values-openshift.yaml \
--set googleApiKeySecret.googleApiKey=$GOOGLE_API_KEY \
--set controllerManager.manager.env.otelExporterOtlpEndpoint=https://mlflow.redhat-ods-applications.svc.cluster.local:8443 \
--set otelHeadersSecret.otelExporterOtlpHeaders="x-mlflow-experiment-id=$MLFLOW_EXPERIMENT_ID\,x-mlflow-workspace=$MLFLOW_WORKSPACE"
```