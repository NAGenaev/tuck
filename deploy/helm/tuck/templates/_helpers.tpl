{{/*
Expand the chart name.
*/}}
{{- define "tuck.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "tuck.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}

{{/*
Namespace to deploy into.
*/}}
{{- define "tuck.namespace" -}}
{{- default .Release.Namespace .Values.namespaceOverride }}
{{- end }}

{{/*
Common labels applied to all resources.
*/}}
{{- define "tuck.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}

{{/*
Selector labels for a component.
*/}}
{{- define "tuck.selectorLabels" -}}
app.kubernetes.io/name: {{ include "tuck.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/component: {{ .component }}
{{- end }}

{{/*
Server image reference.
*/}}
{{- define "tuck.serverImage" -}}
{{- $reg := .Values.global.imageRegistry -}}
{{- $repo := .Values.server.image.repository -}}
{{- $tag := .Values.server.image.tag | default .Chart.AppVersion -}}
{{- if $reg -}}
{{- printf "%s/%s:%s" $reg $repo $tag }}
{{- else -}}
{{- printf "%s:%s" $repo $tag }}
{{- end -}}
{{- end }}

{{/*
Operator image reference.
*/}}
{{- define "tuck.operatorImage" -}}
{{- $reg := .Values.global.imageRegistry -}}
{{- $repo := .Values.operator.image.repository -}}
{{- $tag := .Values.operator.image.tag | default .Chart.AppVersion -}}
{{- if $reg -}}
{{- printf "%s/%s:%s" $reg $repo $tag }}
{{- else -}}
{{- printf "%s:%s" $repo $tag }}
{{- end -}}
{{- end }}

{{/*
Injector image reference.
*/}}
{{- define "tuck.injectorImage" -}}
{{- $reg := .Values.global.imageRegistry -}}
{{- $repo := .Values.injector.image.repository -}}
{{- $tag := .Values.injector.image.tag | default .Chart.AppVersion -}}
{{- printf "%s/%s:%s" $reg $repo $tag }}
{{- end }}

{{/*
Agent image reference (used as default by the injector).
*/}}
{{- define "tuck.agentImage" -}}
{{- if .Values.injector.agentImage -}}
{{- .Values.injector.agentImage -}}
{{- else -}}
{{- $reg := .Values.global.imageRegistry -}}
{{- $tag := .Chart.AppVersion -}}
{{- printf "%s/tuck-agent:%s" $reg $tag }}
{{- end }}
{{- end }}

{{/*
Tuck server address for operator/injector.
*/}}
{{- define "tuck.serverAddr" -}}
{{- if .Values.operator.tuckAddr -}}
{{- .Values.operator.tuckAddr -}}
{{- else -}}
{{- $scheme := "http" -}}
{{- if .Values.server.tls.enabled }}{{- $scheme = "https" }}{{- end -}}
{{- printf "%s://%s-server.%s.svc:%d" $scheme (include "tuck.fullname" .) (include "tuck.namespace" .) (int .Values.server.service.port) }}
{{- end }}
{{- end }}
