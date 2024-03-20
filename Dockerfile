FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY manager .
USER 2000:2000
ENTRYPOINT ["/manager"]
