FROM golang:1.25-alpine AS build
ARG VERSION=0.1
WORKDIR /src
COPY . .
RUN go mod tidy && CGO_ENABLED=0 go build -trimpath \
	-ldflags"-s -w -X main.version=${VERSION}" -o /analytics .

FROM alpine:3.22
RUN apk add --no-cache tzdata ca-certificates \
	&& adduser -D -u 10001 xyz
WORKDIR /app
COPY --from=build /analytics /app/analytics
RUN mkdir -p /app/data && chown -R xyz:xyz /app
USER xyz
EXPOSE 8080
VOLUME ["/app/data"]
ENTRYPOINT ["/app/analytics"]
