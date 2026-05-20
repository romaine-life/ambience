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
