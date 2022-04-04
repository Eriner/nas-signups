FROM golang:alpine as build

WORKDIR /build
RUN apk add -U upx git
COPY . .
RUN go mod download && CGO_ENABLED=0 go build -o nas-signups -trimpath -ldflags="-s -w" . && upx nas-signups

FROM scratch
COPY --from=build /build/nas-signups /nas-signups
ENTRYPOINT [ "/nas-signups" ]
