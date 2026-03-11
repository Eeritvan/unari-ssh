FROM golang:1.26-alpine3.23 AS build

WORKDIR /build
COPY . .

RUN adduser -D -H -g '' -u 10001 nonroot

RUN apk add --no-cache ca-certificates openssh-keygen && \
    apk add tzdata

RUN go mod download && \
    go mod verify && \
    GOOS=linux GOARCH=amd64 go build -ldflags "-w -s -extldflags '-static -Wl,--strip-all,--gc-sections'" -o server


FROM scratch

COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /etc/passwd /etc/passwd
COPY --from=build /build/server /server
COPY --from=build /usr/share/zoneinfo /usr/share/zoneinfo

USER nonroot

CMD ["/server"]
