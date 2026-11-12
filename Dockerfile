FROM oven/bun:1.4.4 AS web
WORKDIR /src
COPY web/package.json web/bun.lock web/.npmrc ./
RUN bun install --frozen-lockfile
COPY web ./
RUN bun run build

FROM golang:1.27.4 AS server
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY api api
COPY cmd cmd
COPY internal internal
COPY --from=web /src/build internal/webui/dist
RUN CGO_ENABLED=0 go build -trimpath -o /kitsune-server ./cmd/kitsune-server

FROM scratch
COPY --from=server /kitsune-server /kitsune-server
ENV XDG_CONFIG_HOME=/config XDG_DATA_HOME=/data
USER 1000:1000
EXPOSE 4533
ENTRYPOINT ["/kitsune-server"]
CMD ["serve"]
