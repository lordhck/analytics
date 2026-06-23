FROM golang:1.25-alpine AS build
ARG VERSION=0.1
WORKDIR /src
COPY server/ .
RUN go mod tidy && CGO_ENABLED=0 go build -trimpath \
	-ldflags="-s -w -X main.version=${VERSION}" -o /analytics .

FROM alpine:3.22
RUN apk add --no-cache tzdata ca-certificates su-exec \
	&& adduser -D -u 10001 xyz
WORKDIR /app
COPY --from=build /analytics /app/analytics
COPY docker/entrypoint.sh /entrypoint.sh
RUN mkdir -p /app/data && chown -R xyz:xyz /app && chmod +x /entrypoint.sh
EXPOSE 8080
VOLUME ["/app/data"]
ENTRYPOINT ["/entrypoint.sh"]
