{{- define "ambience.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "ambience.labels" -}}
app.kubernetes.io/name: {{ include "ambience.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" }}
{{- end -}}

{{- define "ambience.selectorLabels" -}}
app: {{ include "ambience.name" . }}
{{- end -}}

{{- define "ambience.renderMode" -}}
{{- $mode := .Values.renderMode | default "normal" -}}
{{- if not (has $mode (list "normal" "warm" "hot")) -}}
{{- fail (printf "renderMode must be one of: normal, warm, hot; got %q" $mode) -}}
{{- end -}}
{{- $mode -}}
{{- end -}}

{{- define "ambience.isTestEnv" -}}
{{- $mode := include "ambience.renderMode" . -}}
{{- if or (eq $mode "warm") (eq $mode "hot") -}}true{{- else -}}false{{- end -}}
{{- end -}}

{{- define "ambience.renderWarm" -}}
{{- $mode := include "ambience.renderMode" . -}}
{{- if or (eq $mode "normal") (eq $mode "warm") -}}true{{- else -}}false{{- end -}}
{{- end -}}

{{- define "ambience.renderHot" -}}
{{- $mode := include "ambience.renderMode" . -}}
{{- if or (eq $mode "normal") (eq $mode "hot") -}}true{{- else -}}false{{- end -}}
{{- end -}}

{{- define "ambience.slotName" -}}
{{- if eq (include "ambience.isTestEnv" .) "true" -}}{{ required "testEnv.slotName is required when renderMode is warm or hot" .Values.testEnv.slotName }}{{- else -}}{{ .Release.Name }}{{- end -}}
{{- end -}}

{{- define "ambience.edgeSelectorLabels" -}}
app: {{ include "ambience.name" . }}
component: edge
{{- end -}}

{{- define "ambience.authoritySelectorLabels" -}}
app: {{ include "ambience.name" . }}
component: authority
{{- end -}}

{{- define "ambience.edgeImage" -}}
{{- $repo := default .Values.image.repository .Values.edge.image.repository -}}
{{- $tag := default .Values.image.tag .Values.edge.image.tag | toString -}}
{{- printf "%s:%s" $repo $tag -}}
{{- end -}}

{{- define "ambience.edgeImagePullPolicy" -}}
{{- default .Values.image.pullPolicy .Values.edge.image.pullPolicy -}}
{{- end -}}

{{- define "ambience.authorityImage" -}}
{{- $repo := default .Values.image.repository .Values.authority.image.repository -}}
{{- $tag := default .Values.image.tag .Values.authority.image.tag | toString -}}
{{- printf "%s:%s" $repo $tag -}}
{{- end -}}

{{- define "ambience.authorityImagePullPolicy" -}}
{{- default .Values.image.pullPolicy .Values.authority.image.pullPolicy -}}
{{- end -}}

{{- define "ambience.domainHost" -}}
{{- if eq (include "ambience.isTestEnv" .) "true" -}}{{ printf "%s.%s" (include "ambience.slotName" .) .Values.testEnv.recordBase }}{{- else -}}{{ .Values.domain.host }}{{- end -}}
{{- end -}}

{{- define "ambience.routeListenerSetName" -}}
{{- if eq (include "ambience.isTestEnv" .) "true" -}}{{ .Values.testEnv.wildcardListenerSetName }}{{- else -}}{{ default .Values.gateway.listenerSetName .Values.route.attachListenerSet.name }}{{- end -}}
{{- end -}}

{{- define "ambience.routeListenerSetNamespace" -}}
{{- if eq (include "ambience.isTestEnv" .) "true" -}}{{ .Values.testEnv.wildcardListenerSetNamespace }}{{- else -}}{{ default .Release.Namespace .Values.route.attachListenerSet.namespace }}{{- end -}}
{{- end -}}

{{- define "ambience.edgeReplicas" -}}
{{- if eq (include "ambience.isTestEnv" .) "true" -}}1{{- else -}}{{ .Values.edge.replicas }}{{- end -}}
{{- end -}}

{{- define "ambience.authorityReplicas" -}}
{{- if eq (include "ambience.isTestEnv" .) "true" -}}1{{- else -}}{{ .Values.authority.replicas }}{{- end -}}
{{- end -}}

{{/*
ambience.authorityHotSwap — whether the authority renders its image-deploy-lane
hot-swap supervisor + writer container. True only on a validation slot
(warm/hot) with hotSwapBackend enabled AND when the live-preview lane is OFF. The
live-preview lane runs the STABLE backend directly (no hot-swap), so the two
lanes are mutually exclusive. Gating on (not livePreview.enabled) keeps every
non-preview render byte-identical (livePreview.enabled defaults false) while
ensuring a preview lease never activates the image-deploy machinery — Glimmung's
generic provision sets a top-level hotSwapBackend.enabled=false that ambience's
nested authority.hotSwapBackend.enabled does not read, so the gate lives here.
*/}}
{{- define "ambience.authorityHotSwap" -}}
{{- if and (eq (include "ambience.isTestEnv" .) "true") .Values.authority.hotSwapBackend.enabled (not .Values.livePreview.enabled) -}}true{{- else -}}false{{- end -}}
{{- end -}}

{{/*
ambience.servedSelectorLabels — the pod selector the PUBLIC Service targets. With
live preview OFF this is the native edge tier (component: edge), unchanged. With
live preview ON the generic live-preview-edge is co-located in the authority pod
(it replaces the native edge), so the served selector flips to the authority
(component: authority).
*/}}
{{- define "ambience.servedSelectorLabels" -}}
{{- if .Values.livePreview.enabled -}}
{{- include "ambience.authoritySelectorLabels" . -}}
{{- else -}}
{{- include "ambience.edgeSelectorLabels" . -}}
{{- end -}}
{{- end -}}
