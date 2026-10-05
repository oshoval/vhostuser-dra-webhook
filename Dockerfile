FROM golang:1.24 AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags='-s -w' -o /vhostuser-dra-webhook .

FROM gcr.io/distroless/static:nonroot
COPY --from=build /vhostuser-dra-webhook /vhostuser-dra-webhook
USER 65532:65532
ENTRYPOINT ["/vhostuser-dra-webhook"]
