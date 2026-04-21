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
{{- $repo := default .Values.image.repository .Values.edge.image.repository -}}
{{- $tag := default .Values.image.tag .Values.edge.image.tag -}}
{{- printf "%s:%s" $repo $tag -}}
{{- end -}}

{{- define "ambience.edgeImagePullPolicy" -}}
{{- default .Values.image.pullPolicy .Values.edge.image.pullPolicy -}}
{{- end -}}

{{- define "ambience.authorityImage" -}}
{{- $repo := default .Values.image.repository .Values.authority.image.repository -}}
{{- $tag := default .Values.image.tag .Values.authority.image.tag -}}
{{- printf "%s:%s" $repo $tag -}}
{{- end -}}

{{- define "ambience.authorityImagePullPolicy" -}}
{{- default .Values.image.pullPolicy .Values.authority.image.pullPolicy -}}
{{- end -}}
