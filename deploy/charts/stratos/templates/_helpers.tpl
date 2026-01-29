{{/*
Expand the name of the chart.
*/}}
{{- define "stratos.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "stratos.fullname" -}}
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
{{- define "stratos.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "stratos.labels" -}}
helm.sh/chart: {{ include "stratos.chart" . }}
{{ include "stratos.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "stratos.selectorLabels" -}}
app.kubernetes.io/name: {{ include "stratos.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: controller
{{- end }}

{{/*
Service account name
*/}}
{{- define "stratos.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "stratos.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Controller image
*/}}
{{- define "stratos.image" -}}
{{- printf "%s:%s" .Values.image.repository (default (printf "%s" .Chart.AppVersion) .Values.image.tag) }}
{{- end }}

{{/*
Cluster name - required value
*/}}
{{- define "stratos.clusterName" -}}
{{- required "cluster.name is required. Set it with --set cluster.name=<your-cluster-name>" .Values.cluster.name }}
{{- end }}

{{/*
Cluster API server endpoint - required value for AWS provider
*/}}
{{- define "stratos.clusterEndpoint" -}}
{{- if eq .Values.cloudProvider "aws" }}
{{- required "cluster.apiServerEndpoint is required for AWS provider" .Values.cluster.apiServerEndpoint }}
{{- else }}
{{- .Values.cluster.apiServerEndpoint }}
{{- end }}
{{- end }}

{{/*
Cluster CA - required value for AWS provider
*/}}
{{- define "stratos.clusterCA" -}}
{{- if eq .Values.cloudProvider "aws" }}
{{- required "cluster.certificateAuthority is required for AWS provider" .Values.cluster.certificateAuthority }}
{{- else }}
{{- .Values.cluster.certificateAuthority }}
{{- end }}
{{- end }}

{{/*
Cluster CIDR - required value for AWS provider
*/}}
{{- define "stratos.clusterCIDR" -}}
{{- if eq .Values.cloudProvider "aws" }}
{{- required "cluster.cidr is required for AWS provider" .Values.cluster.cidr }}
{{- else }}
{{- .Values.cluster.cidr }}
{{- end }}
{{- end }}
