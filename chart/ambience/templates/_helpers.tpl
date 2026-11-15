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

{{- define "ambience.edgeSelectorLabels" -}}
app: {{ include "ambience.name" . }}
component: edge
{{- end -}}

{{- define "ambience.authoritySelectorLabels" -}}
app: {{ include "ambience.name" . }}
component: authority
{{- end -}}

{{- define "ambience.edgeImage" -}}
{{- default .Values.image.reference .Values.edge.image.reference -}}
{{- end -}}

{{- define "ambience.edgeImagePullPolicy" -}}
{{- default .Values.image.pullPolicy .Values.edge.image.pullPolicy -}}
{{- end -}}

{{- define "ambience.authorityImage" -}}
{{- default .Values.image.reference .Values.authority.image.reference -}}
{{- end -}}

{{- define "ambience.authorityImagePullPolicy" -}}
{{- default .Values.image.pullPolicy .Values.authority.image.pullPolicy -}}
{{- end -}}

{{- define "ambience.domainHost" -}}
{{- .Values.domain.host -}}
{{- end -}}

{{- define "ambience.routeListenerSetName" -}}
{{- default .Values.gateway.listenerSetName .Values.route.attachListenerSet.name -}}
{{- end -}}

{{- define "ambience.routeListenerSetNamespace" -}}
{{- default .Release.Namespace .Values.route.attachListenerSet.namespace -}}
{{- end -}}
