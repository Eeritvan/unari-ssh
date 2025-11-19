FROM golang:1.25.4-alpine3.22 AS build

WORKDIR /build
COPY . .

RUN adduser -D -H -g '' -u 10001 nonroot

RUN apk add --no-cache openssh-keygen && \
    mkdir -p /build/.ssh && \
    ssh-keygen -t ed25519 -f /build/.ssh/id_ed25519 -N "" && \
    chown -R nonroot:nonroot /build/.ssh

RUN go mod download && \
    go mod verify && \
    GOOS=linux GOARCH=amd64 go build -ldflags "-w -s -extldflags '-static -Wl,--strip-all,--gc-sections'" -o unari-ssh


FROM scratch

COPY --from=build /etc/passwd /etc/passwd
COPY --from=build /build/unari-ssh /unari-ssh
COPY --from=build /build/.ssh /.ssh

USER nonroot

CMD ["/unari-ssh"]
