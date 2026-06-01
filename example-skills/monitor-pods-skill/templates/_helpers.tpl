{{- define "skill.name" -}}
{{- .Values.skill.name }}
{{- end }}

{{- define "skill.labels" -}}
app.kubernetes.io/name: {{ include "skill.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version }}
{{- end }}