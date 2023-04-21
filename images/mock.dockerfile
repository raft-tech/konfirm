FROM scratch
COPY mock /mock
ENTRYPOINT ["/mock"]
CMD ["pass"]