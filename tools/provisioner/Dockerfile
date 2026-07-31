FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go vet ./... && go test ./... \
 && CGO_ENABLED=0 go build -o /out/provisioner ./cmd/provisioner \
 && CGO_ENABLED=0 go build -o /out/telcoctl   ./cmd/telcoctl

FROM alpine:3.20
RUN apk add --no-cache wget ca-certificates
COPY --from=build /out/provisioner /usr/local/bin/provisioner
COPY --from=build /out/telcoctl   /usr/local/bin/telcoctl
EXPOSE 8080
ENTRYPOINT ["provisioner"]
