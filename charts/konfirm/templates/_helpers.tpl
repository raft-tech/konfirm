{{/*
Expand the name of the chart.
*/}}
{{- define "konfirm.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "konfirm.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

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
Operator namespace
*/}}
{{- define "konfirm.managedNamespace" -}}
{{- default .Release.Namespace .Values.manager.namespace }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "konfirm.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- printf "%s-%s" (include "konfirm.fullname" .) "controller" }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Create the name of the manager config map
*/}}
{{- define "konfirm.managerConfigMapName" -}}
{{- $name := default (printf "%s-%s" (include "konfirm.fullname" .) "manager-config") .Values.manager.configMapName }}
{{- if contains .Release.Name $name }}
{{- $name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}

{{/*
Create the name of the leader role
*/}}
{{- define "konfirm.leaderRoleName" -}}
{{- $name := printf "%s-%s" (include "konfirm.fullname" .) "leader-role" }}
{{- $name | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create the name of the manager cluster role
*/}}
{{- define "konfirm.managerClusterRoleName" -}}
{{- $name := printf "%s-%s" (include "konfirm.fullname" .) "manager-role" }}
{{- $name | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create the name of the editor cluster role
*/}}
{{- define "konfirm.editorClusterRoleName" -}}
{{- $name := printf "%s-%s" (include "konfirm.fullname" .) "editor-role" }}
{{- $name | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create the name of the viewer cluster role
*/}}
{{- define "konfirm.viewerClusterRoleName" -}}
{{- $name := printf "%s-%s" (include "konfirm.fullname" .) "viewer-role" }}
{{- $name | trunc 63 | trimSuffix "-" }}
{{- end }}
