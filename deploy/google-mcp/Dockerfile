FROM golang:1.27.1-alpine@sha256:4cb7ac979db5fcc41cae44b2227ba5ab8a51e8807f40d9ba4dee20a0ad960b5b AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY skills/ ./skills/
RUN CGO_ENABLED=0 go build -trimpath -o /out/gog-mcp ./cmd/gog-mcp \
 && CGO_ENABLED=0 go build -trimpath -o /out/gog ./cmd/gog

FROM alpine:3.23@sha256:85fe1e81d6758c208f3e1eed4338a1997e19d4be002d4dd32d3100c9a8c010a0
RUN apk add --no-cache ca-certificates tzdata util-linux \
 && addgroup -g 10001 gog \
 && adduser -D -H -u 10001 -G gog gog
COPY --from=build /out/gog-mcp /out/gog /usr/local/bin/
COPY --chmod=0555 deploy/google-mcp/entrypoint.sh /usr/local/bin/gog-mcp-entrypoint
USER 10001:10001
ENV XDG_CONFIG_HOME=/state GOG_KEYRING_BACKEND=file
ENTRYPOINT ["/usr/local/bin/gog-mcp-entrypoint"]
CMD ["gog-mcp"]
