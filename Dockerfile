FROM golang:1.27.3 AS build
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -o /cachelab ./cmd/cachelab

FROM scratch
COPY --from=build /cachelab /cachelab
ENTRYPOINT ["/cachelab"]
