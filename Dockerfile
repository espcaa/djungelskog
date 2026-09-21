FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/djungelskog ./cmd/djungelskog

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/djungelskog /usr/local/bin/djungelskog
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/djungelskog"]