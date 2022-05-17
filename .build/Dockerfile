ARG KUBEBUILDER_VERSION=v3.4.1
FROM golang:1.17 AS build
ARG KUBEBUILDER_VERSION
RUN curl -L -o kubebuilder "https://github.com/kubernetes-sigs/kubebuilder/releases/download/$KUBEBUILDER_VERSION/kubebuilder_linux_amd64" \
    && chmod +x kubebuilder \
    && mv kubebuilder /usr/bin/
WORKDIR /workspace