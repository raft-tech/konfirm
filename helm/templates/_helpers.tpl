{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "konfirm.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "konfirm.labels" -}}
helm.sh/chart: {{ include "konfirm.chart" . }}
{{ include "konfirm.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "konfirm.selectorLabels" -}}
app.kubernetes.io/name: {{ include "konfirm.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "konfirm.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default "konfirm-controller-manager" .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}
