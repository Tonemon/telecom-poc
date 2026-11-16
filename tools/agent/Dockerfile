FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY . .
RUN go vet ./... && go test ./... \
 && CGO_ENABLED=0 go build -o /out/agent ./cmd/agent

FROM alpine:3.20
RUN apk add --no-cache wget ca-certificates
COPY --from=build /out/agent /usr/local/bin/agent
EXPOSE 8091
ENTRYPOINT ["agent"]
