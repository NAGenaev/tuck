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
CSI image reference.
*/}}
{{- define "tuck.csiImage" -}}
{{- $reg := .Values.global.imageRegistry -}}
{{- $repo := .Values.csi.image.repository -}}
{{- $tag := .Values.csi.image.tag | default .Chart.AppVersion -}}
{{- if $reg -}}
{{- printf "%s/%s:%s" $reg $repo $tag }}
{{- else -}}
{{- printf "%s:%s" $repo $tag }}
{{- end -}}
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

{{/*
Headless Service giving every server pod a stable DNS name (Raft peers).
*/}}
{{- define "tuck.serverHeadless" -}}
{{- printf "%s-server-headless" (include "tuck.fullname" .) -}}
{{- end }}

{{/*
Validate and gate Raft HA settings. Fails early on combinations that cannot work.
*/}}
{{- define "tuck.validateCluster" -}}
{{- if .Values.server.cluster.enabled }}
{{- if eq .Values.server.sealType "dev" }}
{{- fail "server.cluster.enabled does not work with sealType=dev: every pod would generate its own root key. Use sealType=transit (auto-unseal) or shamir." }}
{{- end }}
{{- if not .Values.server.persistence.enabled }}
{{- fail "server.cluster.enabled requires server.persistence.enabled=true (the Raft log must survive pod restarts)" }}
{{- end }}
{{- $n := int .Values.server.replicaCount }}
{{- if or (lt $n 3) (eq (mod $n 2) 0) }}
{{- fail (printf "server.cluster.enabled requires an odd server.replicaCount >= 3 (got %d): an even or single-node Raft group has no fault tolerance gain" $n) }}
{{- end }}
{{- end }}
{{- end }}
