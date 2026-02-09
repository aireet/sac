{{/*
Full image reference
*/}}
{{- define "sac.image" -}}
{{ .registry }}/{{ .name }}:{{ .tag }}
{{- end -}}

{{/*
Common labels
*/}}
{{- define "sac.labels" -}}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version }}
{{- end -}}

{{/*
Selector labels for a component
*/}}
{{- define "sac.selectorLabels" -}}
app: {{ . }}
{{- end -}}
