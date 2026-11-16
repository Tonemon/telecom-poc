FROM golang:1.26-alpine AS build
WORKDIR /src
COPY . .
RUN go vet ./... && go test ./... \
 && CGO_ENABLED=0 go build -o /out/map ./cmd/map

FROM alpine:3.20
RUN apk add --no-cache wget ca-certificates
COPY --from=build /out/map /usr/local/bin/map
EXPOSE 8088
ENTRYPOINT ["map"]
