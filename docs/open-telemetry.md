# Observing agent with Open Telemetry

The traces and logs from the agent can be exported to an Open Telemetry provider such as MLFlow or OpenObserve, so that the individual actions of the agent can be tracked.

This can be useful in debugging skills and understanding new models.

## Configuring the endpoint

There are 3 configuration parameters passed in through environment variables:

* **OTEL_EXPORTER_OTLP_ENDPOINT** the endpoint to export to.
  * For traces `/v1/traces` will be automatically appended.
  * For logs `/v1/logs` will be automatically appended.
  * When the endpoint is deployed inside the cluster, the internal service name with port number should be used
  * For OpenObserve an example is `http://o2-openobserve-standalone.openobserve.svc.cluster.local:5080/api/default`
    * **default** is the Org name in OpenObserve - change this for other orgs.
  * For MLFlow and example is `http://mlflow-mlflow.mlflow.svc.cluster.local:5000`
  * With the Helm chart this value may be set with `controllerManager.manager.env.otelExporterOtlpEndpoint`
  * For MLFlow, only traces are accepted. Logs are **not** supported, so there will be an error `failed to send logs to ...`

  
* **OTEL_EXPORTER_OTLP_HEADERS** - headers to pass when exporting OTEL
  * This is mainly used to pass an Authorization Header
  * For OpenObserve the value is required, and may be got from the OpenObserve Web UI -> Data Sources -> Custom -> Logs -> OTEL Collector and the header value will be shown
  * When setting the value `=` should be used as the separator in place of `:`, and no quotes should be given
    * Example: `Authorization=Basic <base64 value>`
  * With the Helm chart this value may be set with `otelHeadersSecret.otelExporterOtlpHeaders`


* **MLFLOW_EXPERIMENT_ID** - the ID of an MLFLOW experiment
  * The value must be numeric, and the experiment must already exist in MLFlow
    * For any given experiment in MLFlow the numeric ID can be found in the URL when inspecting the Expeiment in detail
    * Example: `http://localhost:5000/#/experiments/2/overview/usage` has a value `2`
  * The value is only used when `OTEL_EXPORTER_OTLP_ENDPOINT` is set
  * With the Helm chart this value may be set with `controllerManager.manager.env.mlflowExperimentId`